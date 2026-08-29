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
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type ReactionTweetsStorer interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	List(string, *uint64, *string) ([]domain.Tweet, string, error)
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
	Delete(userID, tweetID string) error
}

type ReactedUserFetcher interface {
	GetBatch(userIds ...string) (users []domain.User, err error)
	Get(userId string) (users domain.User, err error)
}

type ReactionStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type ReactionsStorer interface {
	React(tweetId, userId, emoji string, isTransitive bool) (reactionsNum uint64, err error)
	Unreact(tweetId, userId string, isTransitive bool) (reactionsNum uint64, err error)
	ReactionsCount(tweetId string) (reactionsNum uint64, err error)
	Reactors(tweetId string, limit *uint64, cursor *string) (reactors []string, cur string, err error)
	Reactions(tweetId string) (map[string]uint64, error)
	SetReacted(userId, tweetId, ownerUserId string) error
	RemoveReacted(userId, tweetId string) error
}

type ReactionNotifier interface {
	Add(not domain.Notification) error
}

func StreamReactionHandler(
	repo ReactionsStorer,
	userRepo ReactedUserFetcher,
	notifyRepo ReactionNotifier,
	streamer ReactionStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.ReactionEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.OwnerId == "" {
			return nil, warpnet.WarpError("react: empty owner id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("react: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("react: empty tweet id")
		}

		reactor, _ := userRepo.Get(ev.OwnerId)
		if err := warpnet.VerifyAuthorship(s, reactor.NodeId); err != nil {
			return nil, err
		}

		tweetId := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		ownNodeInfo := streamer.NodeInfo()

		emoji, err := normalizeReaction(ev.Emoji)
		if err != nil {
			return nil, err
		}

		num, err := repo.React(tweetId, ev.OwnerId, emoji, ev.OwnerId == ownNodeInfo.OwnerId) // store my reaction
		if err != nil {
			log.Errorf("reaction handler failed: %v", err)
			return nil, err
		}

		if err := repo.SetReacted(ev.OwnerId, tweetId, ev.UserId); err != nil {
			log.Warnf("reaction handler: reacted index: %v", err)
		}

		resp := event.ReactionsCountResponse{Count: num, Reactions: getReactionsWithDefault(repo, tweetId)}

		isOwnTweetReaction := ev.OwnerId == ev.UserId
		if isOwnTweetReaction { // own tweet reaction
			return resp, nil
		}

		isSomeoneReactedToMe := ev.OwnerId != ownNodeInfo.OwnerId
		if isSomeoneReactedToMe { // reactions exchange finished
			notifyUsername := reactor.Username
			if err := notifyRepo.Add(domain.Notification{
				Type:        domain.NotificationReactionType,
				Text:        notifyUsername + " reacted your tweet",
				RecepientId: ev.UserId,
				ActorId:     ev.OwnerId,
				TweetId:     tweetId,
			}); err != nil {
				log.Errorf("reaction handler: adding notification: %v", err)
			}
			return resp, nil
		}

		reactedUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return resp, nil
		}
		if err != nil {
			return nil, err
		}

		if reactedUser.NodeId == ownNodeInfo.ID.String() {
			return resp, nil
		}

		reactionDataResp, err := streamer.GenericStream(
			reactedUser.NodeId,
			event.PUBLIC_POST_REACT,
			event.ReactionEvent{
				TweetId: ev.TweetId,
				OwnerId: ev.OwnerId,
				UserId:  ev.UserId,
				Emoji:   ev.Emoji,
			},
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return resp, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(reactionDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other reaction error response: %s", possibleError.Message)
		}

		return resp, nil
	}
}

const (
	defaultReaction  = "❤️"
	maxReactionRunes = 8
)

func normalizeReaction(emoji string) (string, error) {
	if emoji == "" {
		return defaultReaction, nil
	}
	if !utf8.ValidString(emoji) {
		return "", warpnet.WarpError("reaction: not a valid utf-8 string")
	}
	if utf8.RuneCountInString(emoji) > maxReactionRunes {
		return "", warpnet.WarpError("reaction: too long")
	}
	for _, r := range emoji {
		if r == '/' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", warpnet.WarpError("reaction: forbidden character")
		}
	}
	return emoji, nil
}

func StreamUnreactionHandler(repo ReactionsStorer, userRepo ReactedUserFetcher, streamer ReactionStreamer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnreactionEvent
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

		reactor, _ := userRepo.Get(ev.OwnerId)
		if err := warpnet.VerifyAuthorship(s, reactor.NodeId); err != nil {
			return nil, err
		}

		tweetId := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		ownNodeInfo := streamer.NodeInfo()
		// Mirror the reaction path: only the unreactor's own node adjusts the
		// network-wide (CRDT) counter.
		num, err := repo.Unreact(tweetId, ev.OwnerId, ev.OwnerId == ownNodeInfo.OwnerId)
		if err != nil {
			log.Errorf("unreaction handler failed: %v", err)
			return nil, err
		}
		if err := repo.RemoveReacted(ev.OwnerId, tweetId); err != nil {
			log.Warnf("unreaction handler: reacted index: %v", err)
		}
		resp := event.ReactionsCountResponse{Count: num, Reactions: getReactionsWithDefault(repo, tweetId)}

		isOwnTweetUnreaction := ev.OwnerId == ev.UserId
		if isOwnTweetUnreaction { // own tweet reaction
			return resp, nil
		}

		isSomeoneUnreactedToMe := ev.OwnerId != ownNodeInfo.OwnerId
		if isSomeoneUnreactedToMe { // reactions exchange finished
			return resp, nil
		}

		unreactedUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return resp, nil
		}
		if err != nil {
			return nil, err
		}

		if unreactedUser.NodeId == ownNodeInfo.ID.String() {
			return resp, nil
		}

		unreactionDataResp, err := streamer.GenericStream(
			unreactedUser.NodeId,
			event.PUBLIC_POST_UNREACT,
			event.UnreactionEvent{
				TweetId: ev.TweetId,
				UserId:  ev.UserId,
				OwnerId: ev.OwnerId,
			},
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return resp, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(unreactionDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other unreaction error response: %s", possibleError.Message)
		}

		return resp, nil
	}
}

type ReactedTweetsLister interface {
	Reacted(userId string, limit *uint64, cursor *string) ([]domain.ReactedTweet, string, error)
}

// StreamGetReactionsHandler returns one page of the local user's "tweets I
// reacted to" index, newest first. Same reference-only wire shape as bookmarks:
// clients hydrate each tweet via PUBLIC_GET_TWEET using OwnerUserId.
func StreamGetReactionsHandler(repo ReactedTweetsLister) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetReactionsEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("reactions: empty user id")
		}

		reacted, cur, err := repo.Reacted(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		items := make([]event.BookmarkItem, 0, len(reacted))
		for _, lt := range reacted {
			items = append(items, event.BookmarkItem{
				UserId:      lt.UserId,
				TweetId:     lt.TweetId,
				OwnerUserId: lt.OwnerUserId,
				CreatedAt:   lt.CreatedAt,
			})
		}
		return event.GetReactionsResponse{Items: items, Cursor: cur}, nil
	}
}

// getReactionsWithDefault is a best-effort per-emoji tally for a
// react/unreact response: the count itself already succeeded, so a failing
// lookup must not fail the request — it falls back to an empty map, which
// marshals away under the field's omitempty just like a nil one.
func getReactionsWithDefault(repo ReactionsStorer, tweetId string) map[string]uint64 {
	reactions, err := repo.Reactions(tweetId)
	if err != nil {
		log.Warnf("reaction handler: reactions breakdown: %v", err)
		return map[string]uint64{}
	}
	return reactions
}
