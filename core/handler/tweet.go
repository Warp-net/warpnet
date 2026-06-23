/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const tweetCharLimit = 280

type TweetUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

type TweetStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type OwnerTweetStorer interface {
	GetOwner() domain.Owner
}

type TweetBroadcaster interface {
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
}

type TweetsStorer interface {
	TweetsCount(userId string) (uint64, error)
	GetViewsCount(tweetId string) (uint64, error)
	IsBlocklisted(tweetId string) bool
	Blocklist(tweetId string) error
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	List(string, *uint64, *string) ([]domain.Tweet, string, error)
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
	Delete(userID, tweetID string) error
	UnRetweet(retweetedByUserID, tweetId string) error
	Retweeters(tweetId string, limit *uint64, cursor *string) (_ []string, cur string, err error)
	CreateWithTTL(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error)
	Update(tweet domain.Tweet) error
	Pin(userId, tweetId string) (domain.Tweet, error)
	Unpin(userId, tweetId string) (domain.Tweet, error)
	AppendEdit(edit domain.TweetEdit) (domain.TweetEdit, error)
	AddReply(reply domain.Tweet) (domain.Tweet, error)
	GetReply(rootID, replyID string) (domain.Tweet, error)
	DeleteReply(rootID, replyID string) (domain.Tweet, error)
	GetReplies(rootId, parentId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
}

type TimelineUpdater interface {
	AddTweetToTimeline(userId string, tweet domain.Tweet) error
	DeleteTweetFromTimeline(userID, tweetID string) error
}

// TweetFollowChecker reports whether ownerId follows authorId. The new-tweet
// handler uses it to keep unsolicited tweets out of the local timeline: a
// tweet is only accepted when the owner authored it or follows its author.
type TweetFollowChecker interface {
	IsFollowing(ownerId, authorId string) bool
}

func StreamNewTweetHandler(
	broadcaster TweetBroadcaster,
	authRepo OwnerTweetStorer,
	tweetRepo TweetsStorer,
	timelineRepo TimelineUpdater,
	followRepo TweetFollowChecker,
	userRepo TweetUserFetcher,
	notifyRepo ModerationNotifier,
	streamer TweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewTweetEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		// check any incoming tweets
		if ev.Moderation != nil && !ev.Moderation.IsOk {
			return nil, tweetRepo.Blocklist(ev.Id)
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}
		if ev.Text == "" {
			return nil, warpnet.WarpError("empty tweet text")
		}
		if len(ev.Text) > tweetCharLimit {
			return nil, warpnet.WarpError("tweet text is too long")
		}

		// A reply is a tweet with a parent: it lives inside its thread, not
		// in the timeline, so it never reaches the follower-broadcast path.
		if ev.IsReply() {
			return handleNewReply(ev, tweetRepo, userRepo, notifyRepo, streamer)
		}

		owner := authRepo.GetOwner()

		// A tweet only belongs in this node's timeline if the owner wrote
		// it (local authoring) or follows its author (delivered through the
		// owner's own gossip subscription). Any other inbound NewTweetEvent
		// is an unsolicited direct push from a peer we don't follow — drop
		// it so it can't be injected into the Home feed. Ack so the sender
		// doesn't treat it as a transport failure and retry.
		isMyOwnTweet := owner.UserId == ev.UserId
		if !isMyOwnTweet && (followRepo == nil || !followRepo.IsFollowing(owner.UserId, ev.UserId)) {
			return event.Accepted, nil
		}

		tweet, err := tweetRepo.Create(ev.UserId, ev)
		if err != nil {
			return nil, err
		}

		if tweet.Id == "" {
			return tweet, warpnet.WarpError("tweet handler: empty tweet id")
		}
		if err = timelineRepo.AddTweetToTimeline(owner.UserId, tweet); err != nil {
			log.Infof("fail adding tweet to timeline: %v", err)
		}

		if isMyOwnTweet { // publish to friends timelines
			respTweetEvent := event.NewTweetEvent{
				CreatedAt: tweet.CreatedAt,
				Id:        tweet.Id,
				ParentId:  tweet.ParentId,
				RootId:    tweet.RootId,
				Text:      tweet.Text,
				UserId:    tweet.UserId,
				Username:  tweet.Username,
				ImageKeys: tweet.ImageKeys,
			}
			bt, _ := json.Marshal(respTweetEvent)
			if err := broadcaster.PublishUpdateToFollowers(owner.UserId, event.PRIVATE_POST_TWEET, bt); err != nil {
				log.Errorf("broadcaster publish owner tweet update: %v", err)
			}
		}
		return tweet, nil
	}
}

// handleNewReply stores a reply in its thread, notifies the parent tweet's
// author when it lives here, and forwards the reply to the parent author's
// node otherwise so the thread stays consistent across peers.
func handleNewReply(
	ev domain.Tweet,
	replyRepo TweetsStorer,
	userRepo TweetUserFetcher,
	notifyRepo ModerationNotifier,
	streamer TweetStreamer,
) (any, error) {
	rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
	parentId := strings.TrimPrefix(*ev.ParentId, domain.RetweetPrefix)
	id := strings.TrimPrefix(ev.Id, domain.RetweetPrefix)

	reply, err := replyRepo.AddReply(domain.Tweet{
		CreatedAt:    ev.CreatedAt,
		Id:           id,
		ParentId:     &parentId,
		ParentUserId: ev.ParentUserId,
		RootId:       rootId,
		Text:         ev.Text,
		UserId:       ev.UserId,
		Username:     ev.Username,
		ImageKeys:    ev.ImageKeys,
	})
	if err != nil {
		log.Errorf("reply handler failed: %v", err)
		return nil, err
	}

	if ev.ParentUserId == nil || *ev.ParentUserId == "" {
		return reply, nil
	}
	parentUserId := *ev.ParentUserId

	parentUser, err := userRepo.Get(parentUserId)
	if errors.Is(err, database.ErrUserNotFound) {
		return reply, nil
	}
	if err != nil {
		return nil, err
	}

	ownNodeInfo := streamer.NodeInfo()
	isOwnTweetReply := parentUserId == ownNodeInfo.OwnerId
	if isOwnTweetReply {
		if ev.UserId != ownNodeInfo.OwnerId {
			if err := notifyRepo.Add(domain.Notification{
				Type:   domain.NotificationReplyType,
				Text:   ev.Username + " replied to your tweet",
				UserId: parentUserId,
			}); err != nil {
				log.Errorf("reply handler: adding notification: %v", err)
			}
		}
		return reply, nil
	}
	if ownNodeInfo.ID.String() == parentUser.NodeId {
		return reply, nil
	}

	// Forward the normalized/stored reply (prefix-stripped ids) so peers
	// build identical thread keys.
	replyDataResp, err := streamer.GenericStream(
		parentUser.NodeId,
		event.PRIVATE_POST_TWEET,
		domain.Tweet{
			CreatedAt:    reply.CreatedAt,
			Id:           reply.Id,
			ParentId:     reply.ParentId,
			ParentUserId: reply.ParentUserId,
			RootId:       reply.RootId,
			Text:         reply.Text,
			UserId:       reply.UserId,
			Username:     reply.Username,
			ImageKeys:    reply.ImageKeys,
		},
	)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return reply, nil
	}
	if err != nil {
		return nil, err
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(replyDataResp, &possibleError); possibleError.Message != "" {
		log.Errorf("unmarshal other reply error response: %s", possibleError.Message)
	}

	return reply, nil
}

func StreamGetTweetHandler(
	repo TweetsStorer,
	authRepo OwnerTweetStorer,
	userRepo TweetUserFetcher,
	streamer TweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetTweetEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("empty tweet id")
		}
		if repo.IsBlocklisted(ev.TweetId) {
			return nil, warpnet.WarpError("tweet is moderated")
		}

		owner := authRepo.GetOwner()

		isMyOwnTweet := ev.UserId == owner.UserId
		if isMyOwnTweet {
			return localTweet(repo, ev)
		}

		otherUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return localTweet(repo, ev)
		}
		if err != nil {
			return nil, err
		}

		getTweetResp, err := streamer.GenericStream(
			otherUser.NodeId,
			event.PUBLIC_GET_TWEET,
			ev,
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return localTweet(repo, ev)
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(getTweetResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other get tweet error response: %s", possibleError.Message)
			return localTweet(repo, ev)
		}

		var tweet domain.Tweet
		if err = json.Unmarshal(getTweetResp, &tweet); err != nil {
			return localTweet(repo, ev)
		}

		return tweet, nil
	}
}

// localTweet resolves a tweet from local storage: a reply (RootId set and
// distinct from TweetId) is read from the thread index, anything else from
// the timeline keyspace.
func localTweet(repo TweetsStorer, ev event.GetTweetEvent) (domain.Tweet, error) {
	if ev.RootId != "" && ev.RootId != ev.TweetId {
		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
		id := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		if t, err := repo.GetReply(rootId, id); err == nil {
			return t, nil
		}
	}
	return repo.Get(ev.UserId, ev.TweetId)
}

func StreamGetTweetsHandler(
	repo TweetsStorer,
	userRepo TweetUserFetcher,
	streamer TweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllTweetsEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		// Thread context (RootId or ParentId) means "get the replies under a
		// tweet": replies are tweets with a parent, served from the thread
		// index. A plain timeline/profile request carries only UserId.
		if ev.RootId != "" || ev.ParentId != "" {
			return getThreadReplies(repo, userRepo, streamer, ev)
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		tweets, cursor, err := repo.List(
			ev.UserId, ev.Limit, ev.Cursor,
		)
		if err != nil {
			return nil, err
		}

		if len(tweets) != 0 {
			go tweetsRefreshBackground(repo, userRepo, ev, streamer)

			return event.TweetsResponse{
				Cursor: cursor,
				Tweets: tweets,
				UserId: ev.UserId,
			}, nil
		}

		tweetsRefreshBackground(repo, userRepo, ev, streamer)

		tweets, cursor, _ = repo.List(
			ev.UserId, ev.Limit, ev.Cursor,
		)

		return event.TweetsResponse{
			Cursor: cursor,
			Tweets: tweets,
			UserId: ev.UserId,
		}, nil
	}
}

func tweetsRefreshBackground(
	repo TweetsStorer,
	userRepo TweetUserFetcher,
	ev event.GetAllTweetsEvent,
	streamer TweetStreamer,
) {
	ownNodeInfo := streamer.NodeInfo()
	if ownNodeInfo.OwnerId == ev.UserId {
		return
	}
	otherUser, err := userRepo.Get(ev.UserId)
	if errors.Is(err, database.ErrUserNotFound) {
		return
	}
	if err != nil {
		log.Errorf("get tweets handler: get user: %v", err)
		return
	}
	if ownNodeInfo.ID.String() == otherUser.NodeId {
		return
	}

	tweetsDataResp, err := streamer.GenericStream(
		otherUser.NodeId,
		event.PUBLIC_GET_TWEETS,
		ev,
	)
	if err != nil {
		log.Errorf("get tweets handler: stream: %v", err)
		return
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(tweetsDataResp, &possibleError); possibleError.Message != "" {
		log.Errorf("get tweets handler: unmarshal other tweets error response: %s", possibleError.Message)
		return
	}

	var tweetsResp event.TweetsResponse
	if err := json.Unmarshal(tweetsDataResp, &tweetsResp); err != nil {
		log.Errorf("get tweets handler: unmarshalresponse: %s", tweetsDataResp)
		return
	}

	for _, tweet := range tweetsResp.Tweets {
		if repo.IsBlocklisted(tweet.Id) {
			continue
		}
		_, _ = repo.CreateWithTTL(tweet.UserId, tweet, time.Hour*24*30) //nolint:mnd
	}
}

// getThreadReplies serves the direct replies to a tweet — a flat list of
// tweets whose ParentId is that tweet, within its RootId thread. It is one
// level of the tree; clients walk deeper by re-querying with each reply as
// the parent. With no local replies it forwards to the root author's home
// node so threads on remote/bridged tweets still resolve.
func getThreadReplies(
	repo TweetsStorer,
	userRepo TweetUserFetcher,
	streamer TweetStreamer,
	ev event.GetAllTweetsEvent,
) (any, error) {
	// ParentId is the tweet whose replies we want; RootId locates its thread.
	// For a top-level tweet the two coincide, so either field implies the
	// other — derive the missing one rather than misroute the request.
	parentId := ev.ParentId
	if parentId == "" {
		parentId = ev.RootId
	}
	rootId := ev.RootId
	if rootId == "" {
		rootId = parentId
	}

	rootId = strings.TrimPrefix(rootId, domain.RetweetPrefix)
	parentId = strings.TrimPrefix(parentId, domain.RetweetPrefix)

	replies, cursor, err := repo.GetReplies(rootId, parentId, ev.Limit, ev.Cursor)
	if err != nil {
		return nil, err
	}
	if len(replies) == 0 {
		if resp, ok := forwardThreadReplies(userRepo, streamer, ev); ok {
			return resp, nil
		}
	}
	return event.TweetsResponse{
		Cursor: cursor,
		Tweets: replies,
		UserId: parentId,
	}, nil
}

// forwardThreadReplies asks the root tweet author's home node for the thread's
// replies when that node is not this one. ok=false means handle locally.
func forwardThreadReplies(userRepo TweetUserFetcher, streamer TweetStreamer, ev event.GetAllTweetsEvent) (event.TweetsResponse, bool) {
	if ev.RootUserId == "" {
		return event.TweetsResponse{}, false
	}
	author, err := userRepo.Get(ev.RootUserId)
	if err != nil || author.NodeId == "" || author.NodeId == streamer.NodeInfo().ID.String() {
		return event.TweetsResponse{}, false
	}
	data, err := streamer.GenericStream(author.NodeId, event.PUBLIC_GET_TWEETS, ev)
	if err != nil {
		log.Errorf("get replies: forward to %s: %v", author.NodeId, err)
		return event.TweetsResponse{}, false
	}
	var resp event.TweetsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return event.TweetsResponse{}, false
	}
	return resp, true
}

type LikeTweetStorer interface {
	Like(tweetId, userId string) (likesNum uint64, err error)
	Unlike(tweetId, userId string) (likesNum uint64, err error)
	LikesCount(tweetId string) (likesNum uint64, err error)
	Likers(tweetId string, limit *uint64, cursor *string) (likers []string, cur string, err error)
}

func StreamDeleteTweetHandler(
	broadcaster TweetBroadcaster,
	authRepo OwnerTweetStorer,
	repo TweetsStorer,
	timelineRepo TimelineUpdater,
	likeRepo LikeTweetStorer,
	userRepo TweetUserFetcher,
	streamer TweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteTweetEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("empty tweet id")
		}

		// A reply carries its thread RootId: delete it from the thread index
		// and forward the deletion to the parent author's node.
		if ev.RootId != "" && ev.RootId != ev.TweetId {
			return deleteReply(ev, repo, userRepo, streamer)
		}

		if _, err := likeRepo.Unlike(ev.UserId, strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)); err != nil {
			log.Errorf("delete tweet: unliking tweet: %v", err)
		}

		t, _ := repo.Get(ev.UserId, domain.RetweetPrefix+ev.TweetId)
		if t.RetweetedBy != nil {
			_ = repo.UnRetweet(*t.RetweetedBy, t.Id)
		}

		if err := repo.Delete(ev.UserId, strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)); err != nil {
			return nil, err
		}
		if err := timelineRepo.DeleteTweetFromTimeline(ev.UserId, ev.TweetId); err != nil {
			log.Errorf("delete tweet: timeline delete: %v", err)
		}

		owner := authRepo.GetOwner()
		isMyOwnTweet := owner.UserId == ev.UserId
		if isMyOwnTweet {
			respTweetEvent := event.DeleteTweetEvent{
				UserId:  ev.UserId,
				TweetId: ev.TweetId,
			}
			bt, _ := json.Marshal(respTweetEvent)
			if err := broadcaster.PublishUpdateToFollowers(owner.UserId, event.PRIVATE_DELETE_TWEET, bt); err != nil {
				log.Infoln("broadcaster publish owner tweet update:", err)
			}
		}

		return event.Accepted, nil
	}
}

// deleteReply removes a reply from its thread and propagates the deletion to
// the parent author's node so the thread stays consistent across peers.
func deleteReply(
	ev event.DeleteTweetEvent,
	repo TweetsStorer,
	userRepo TweetUserFetcher,
	streamer TweetStreamer,
) (any, error) {
	rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
	id := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)

	reply, err := repo.DeleteReply(rootId, id)
	if err != nil {
		log.Errorf("delete reply handler failed: %v", err)
		return nil, err
	}

	if reply.ParentUserId == nil || *reply.ParentUserId == "" {
		return event.Accepted, nil
	}

	ownNodeInfo := streamer.NodeInfo()
	parentUser, err := userRepo.Get(*reply.ParentUserId)
	if errors.Is(err, database.ErrUserNotFound) {
		return event.Accepted, nil
	}
	if err != nil {
		return nil, err
	}
	if ownNodeInfo.ID.String() == parentUser.NodeId {
		return event.Accepted, nil
	}

	// Forward normalized ids so the remote node deletes under the same key.
	resp, err := streamer.GenericStream(
		parentUser.NodeId,
		event.PRIVATE_DELETE_TWEET,
		event.DeleteTweetEvent{TweetId: id, RootId: rootId, UserId: ev.UserId},
	)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return event.Accepted, nil
	}
	if err != nil {
		return nil, err
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		log.Errorf("unmarshal other delete reply error response: %s", possibleError.Message)
	}

	return event.Accepted, nil
}

type RetweetsTweetStorer interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	NewRetweet(tweet domain.Tweet) (_ domain.Tweet, err error)
	UnRetweet(retweetedByUserID, tweetId string) error
	RetweetsCount(tweetId string) (uint64, error)
	Retweeters(tweetId string, limit *uint64, cursor *string) (_ []string, cur string, err error)
}

type RepliesTweetCounter interface {
	RepliesCount(tweetId string) (likesNum uint64, err error)
}

// CRDTLikesCounter provides CRDT-based likes counting
type CRDTLikesCounter interface {
	GetLikesCount(tweetID string) (uint64, error)
}

// CRDTRetweetsCounter provides CRDT-based retweets counting
type CRDTRetweetsCounter interface {
	GetRetweetsCount(tweetID string) (uint64, error)
}

// CRDTRepliesCounter provides CRDT-based replies counting
type CRDTRepliesCounter interface {
	GetRepliesCount(tweetID string) (uint64, error)
}

func StreamGetTweetStatsHandler(
	tweetRepo TweetsStorer,
	likeRepo LikeTweetStorer,
	retweetRepo RetweetsTweetStorer,
	replyRepo RepliesTweetCounter,
	userRepo TweetUserFetcher,
	streamer TweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetTweetStatsEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("empty tweet id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		ownNodeInfo := streamer.NodeInfo()

		isMyOwnTweet := ev.UserId == ownNodeInfo.OwnerId
		if !isMyOwnTweet { //nolint:nestif
			u, err := userRepo.Get(ev.UserId)
			if errors.Is(err, database.ErrUserNotFound) {
				return event.TweetStatsResponse{TweetId: ev.TweetId}, nil
			}
			if err != nil {
				return nil, err
			}

			if ownNodeInfo.ID.String() == u.NodeId {
				return event.TweetStatsResponse{TweetId: ev.TweetId}, nil
			}

			statsResp, err := streamer.GenericStream(
				u.NodeId,
				event.PUBLIC_GET_TWEET_STATS,
				ev,
			)
			if errors.Is(err, warpnet.ErrNodeIsOffline) {
				return event.TweetStatsResponse{TweetId: ev.TweetId}, nil
			}
			if err != nil {
				log.Errorf("stream other stats handler failed: %v", err)
				return nil, err
			}

			var possibleError event.ResponseError
			if _ = json.Unmarshal(statsResp, &possibleError); possibleError.Message != "" {
				return nil, fmt.Errorf("unmarshal other reply response: %w", possibleError)
			}

			var stats event.TweetStatsResponse
			if err := json.Unmarshal(statsResp, &stats); err != nil {
				return nil, fmt.Errorf("fetching tweet stats response: %w", err)
			}

			return stats, nil
		}

		var (
			retweetsCount uint64
			likesCount    uint64
			repliesCount  uint64
			viewsCount    uint64
			ctx, cancelF  = context.WithDeadline(context.Background(), time.Now().Add(time.Second*5)) //nolint:mnd
			g, _          = errgroup.WithContext(ctx)
			tweetId       = strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		)
		defer cancelF()

		g.Go(func() (retweetsErr error) {
			retweetsCount, retweetsErr = retweetRepo.RetweetsCount(tweetId)
			if errors.Is(retweetsErr, database.ErrTweetNotFound) {
				return nil
			}
			return retweetsErr
		})
		g.Go(func() (likesErr error) {
			likesCount, likesErr = likeRepo.LikesCount(tweetId)
			if errors.Is(likesErr, database.ErrLikesNotFound) {
				return nil
			}
			return likesErr
		})
		g.Go(func() (repliesErr error) {
			repliesCount, repliesErr = replyRepo.RepliesCount(tweetId)
			if errors.Is(repliesErr, database.ErrReplyNotFound) {
				return nil
			}
			return repliesErr
		})
		g.Go(func() (viewsErr error) {
			viewsCount, viewsErr = tweetRepo.GetViewsCount(tweetId)
			if errors.Is(viewsErr, database.ErrViewsNotFound) {
				return nil
			}
			return viewsErr
		})
		if err = g.Wait(); err != nil {
			log.Errorf("get tweet stats: %s %v", buf, err)
		}
		return event.TweetStatsResponse{
			TweetId:       ev.TweetId,
			ViewsCount:    viewsCount,
			RetweetsCount: retweetsCount,
			LikeCount:     likesCount,
			RepliesCount:  repliesCount,
		}, nil
	}
}

func StreamPinTweetHandler(repo TweetsStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		return setPinnedFromEvent(buf, repo, true)
	}
}

func StreamUnpinTweetHandler(repo TweetsStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		return setPinnedFromEvent(buf, repo, false)
	}
}

func StreamEditTweetHandler(repo TweetsStorer, timelineRepo TimelineUpdater) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.EditTweetEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("edit tweet: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("edit tweet: empty tweet id")
		}
		if ev.Text == "" {
			return nil, warpnet.WarpError("edit tweet: empty text")
		}

		existing, err := repo.Get(ev.UserId, ev.TweetId)
		if err != nil {
			return nil, err
		}
		if existing.UserId != ev.UserId {
			return nil, warpnet.WarpError("edit tweet: only the author can edit their own tweet")
		}
		if existing.Text == ev.Text {
			// No-op edit — return the existing tweet without recording a revision.
			return event.EditTweetResponse(existing), nil
		}

		// Append the *previous* text as a revision so the user can see what
		// the tweet looked like before this edit.
		if _, err := repo.AppendEdit(domain.TweetEdit{
			OriginalTweetId: existing.Id,
			UserId:          existing.UserId,
			Text:            existing.Text,
		}); err != nil {
			return nil, err
		}

		updated := existing
		updated.Text = ev.Text
		if err := repo.Update(updated); err != nil {
			return nil, err
		}
		// Re-fetch so the response carries the storage-canonical UpdatedAt.
		out, err := repo.Get(existing.UserId, existing.Id)
		if err != nil {
			return nil, err
		}

		// Refresh the author's own timeline copy. The timeline stores
		// a snapshot of the tweet at the time it was added, so without
		// this refresh the Home feed would keep serving the pre-edit
		// text until the timeline entry is naturally rewritten.
		if timelineRepo != nil {
			if err := timelineRepo.AddTweetToTimeline(out.UserId, out); err != nil {
				log.Warnf("edit tweet: timeline refresh failed: %v", err)
			}
		}

		// Cancel every plain retweet of this tweet — they were
		// retweeting the *original* content, and on an edit that
		// consent shouldn't carry over. Quote-style retweets live as
		// their own tweets and aren't enumerated here; the reader
		// detects an edited source via the comparison of source
		// UpdatedAt and the quote's CreatedAt and surfaces the source
		// as "unavailable".
		cancelRetweetsForEditedTweet(repo, existing.Id)

		return event.EditTweetResponse(out), nil
	}
}

// cancelRetweetsForEditedTweet iterates the retweeters index for
// tweetId on the local node and unretweets each one. Failures are
// logged and swallowed — a stuck retweet is not worth failing the
// edit response over. Cross-node propagation of the cancellation is
// out of scope.
func cancelRetweetsForEditedTweet(repo TweetsStorer, tweetId string) {
	var cursor string
	limit := uint64(100)
	for {
		retweeters, cur, err := repo.Retweeters(tweetId, &limit, &cursor)
		if err != nil {
			log.Warnf("edit tweet: list retweeters of %s: %v", tweetId, err)
			return
		}
		for _, retweeterId := range retweeters {
			if err := repo.UnRetweet(retweeterId, tweetId); err != nil {
				log.Warnf("edit tweet: cancel retweet by %s of %s: %v", retweeterId, tweetId, err)
			}
		}
		if cur == "" || uint64(len(retweeters)) < limit {
			return
		}
		cursor = cur
	}
}

// setPinnedFromEvent decodes the pin/unpin payload, enforces author-only
// pinning, and delegates the write.
func setPinnedFromEvent(buf []byte, repo TweetsStorer, pin bool) (any, error) {
	op := "unpin"
	if pin {
		op = "pin"
	}
	var ev event.PinTweetEvent
	if err := json.Unmarshal(buf, &ev); err != nil {
		return nil, err
	}
	if ev.UserId == "" {
		return nil, warpnet.WarpError(op + ": empty user id")
	}
	if ev.TweetId == "" {
		return nil, warpnet.WarpError(op + ": empty tweet id")
	}
	tw, err := repo.Get(ev.UserId, ev.TweetId)
	if err != nil {
		return nil, err
	}
	if tw.UserId != ev.UserId {
		return nil, warpnet.WarpError(op + ": only the author can " + op + " their own tweet")
	}
	if pin {
		return repo.Pin(ev.UserId, ev.TweetId)
	}
	return repo.Unpin(ev.UserId, ev.TweetId)
}
