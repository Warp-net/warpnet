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
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"strings"
	"time"
)

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
	PublishModerationRequest(body []byte) (err error)
}

type TweetsStorer interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	List(string, *uint64, *string) ([]domain.Tweet, string, error)
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
	Delete(userID, tweetID string) error
	CreateWithTTL(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error)
}

type TimelineUpdater interface {
	AddTweetToTimeline(userId string, tweet domain.Tweet) error
}

func StreamNewTweetHandler(
	broadcaster TweetBroadcaster,
	authRepo OwnerTweetStorer,
	tweetRepo TweetsStorer,
	timelineRepo TimelineUpdater,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewTweetEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		owner := authRepo.GetOwner()

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
		if owner.UserId == ev.UserId {
			respTweetEvent := event.NewTweetEvent{
				CreatedAt: tweet.CreatedAt,
				Id:        tweet.Id,
				ParentId:  tweet.ParentId,
				RootId:    tweet.RootId,
				Text:      tweet.Text,
				UserId:    tweet.UserId,
				Username:  tweet.Username,
				ImageKey:  tweet.ImageKey,
			}
			bt, _ := json.Marshal(respTweetEvent)
			if err := broadcaster.PublishUpdateToFollowers(owner.UserId, event.PRIVATE_POST_TWEET, bt); err != nil {
				log.Errorf("broadcaster publish owner tweet update: %v", err)
			}

			moderationEvent := event.ModerationEvent{
				NodeID:   owner.NodeId,
				UserID:   owner.UserId,
				Type:     event.Tweet,
				ObjectID: &tweet.Id,
			}
			bt, _ = json.Marshal(moderationEvent)

			if err := broadcaster.PublishModerationRequest(bt); err != nil {
				log.Errorf("broadcaster publish tweet moderation request: %v", err)
			} else {
				log.Infof("tweet: %s moderation requested", tweet.Id)
			}

		}
		return tweet, nil
	}
}

func StreamGetTweetHandler(repo TweetsStorer) warpnet.WarpHandlerFunc {
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

		return repo.Get(ev.UserId, ev.TweetId)
	}
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
			}, err
		}

		tweetsRefreshBackground(repo, userRepo, ev, streamer)

		tweets, cursor, _ = repo.List(
			ev.UserId, ev.Limit, ev.Cursor,
		)

		return event.TweetsResponse{
			Cursor: cursor,
			Tweets: tweets,
			UserId: ev.UserId,
		}, err
	}
}

func tweetsRefreshBackground(
	repo TweetsStorer,
	userRepo TweetUserFetcher,
	ev event.GetAllTweetsEvent,
	streamer TweetStreamer,
) {
	if streamer.NodeInfo().OwnerId == ev.UserId {
		return
	}
	otherUser, err := userRepo.Get(ev.UserId)
	if err != nil {
		log.Errorf("get tweets handler: get user: %v", err)
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

	var possibleError event.ErrorResponse
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
		_, _ = repo.CreateWithTTL(tweet.UserId, tweet, time.Hour*24)
	}
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
	likeRepo LikeTweetStorer,
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

		if _, err := likeRepo.Unlike(ev.UserId, strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)); err != nil {
			log.Errorf("delete tweet: fail unliking tweet: %v", err)
		}

		if err := repo.Delete(ev.UserId, strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)); err != nil {
			return nil, err
		}

		owner := authRepo.GetOwner()
		if owner.UserId == ev.UserId {
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

func StreamGetTweetStatsHandler(
	likeRepo LikeTweetStorer,
	retweetRepo RetweetsTweetStorer,
	replyRepo RepliesTweetCounter, // TODO views
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

		if ev.UserId != streamer.NodeInfo().OwnerId {
			u, err := userRepo.Get(ev.UserId)
			if err != nil {
				return nil, err
			}

			statsResp, err := streamer.GenericStream(
				u.NodeId,
				event.PUBLIC_GET_TWEET_STATS,
				ev,
			)
			if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
				return nil, err
			}

			var possibleError event.ErrorResponse
			if _ = json.Unmarshal(statsResp, &possibleError); possibleError.Message != "" {
				return nil, fmt.Errorf("unmarshal other reply response: %s", possibleError.Message)
			}

			var stats event.TweetStatsResponse
			if err := json.Unmarshal(statsResp, &stats); err != nil {
				return nil, fmt.Errorf("fetching tweet stats response: %v", err)
			}
			return stats, nil
		}

		var (
			retweetsCount uint64
			likesCount    uint64
			repliesCount  uint64
			ctx, cancelF  = context.WithDeadline(context.Background(), time.Now().Add(time.Second*5))
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
		if err = g.Wait(); err != nil {
			log.Errorf("get tweet stats: %s %v", buf, err)
		}
		return event.TweetStatsResponse{
			TweetId:       ev.TweetId,
			ViewsCount:    0, // TODO
			RetweetsCount: retweetsCount,
			LikeCount:     likesCount,
			RepliesCount:  repliesCount,
		}, nil
	}
}

//g.Go(func() error {
//	retweeters, retweetersCursor, err = retweetRepo.Retweeters(tweetId, ev.Limit, ev.Cursor)
//	if errors.Is(err, database.ErrTweetNotFound) {
//		return nil
//	}
//	return err
//})
//g.Go(func() error {
//	likers, likersCursor, err = likeRepo.Likers(tweetId, ev.Limit, ev.Cursor)
//	if errors.Is(err, database.ErrLikesNotFound) {
//		return nil
//	}
//	return err
//})
