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

package mastodon

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	stripper "github.com/grokify/html-strip-tags-go"
	"github.com/mattn/go-mastodon"
)

const (
	mastodonServer = "https://mastodon.social"
	// pk, _ = security.GenerateKeyFromSeed([]byte(mastodonServer))
	// mastodonPseudoPeerID, _ := warpnet.IDFromPublicKey(pk.Public().(ed25519.PublicKey))
	mastodonPseudoPeerID = "12D3KooWDfpE8bR2iBjEMMe7gTVwEiahF9duFxETLHq3N6en9hsG"
	mastodonPseudoMaddr  = "/dns4/mastodon.social/tcp/443"

	// read only proxy account
	clientID     = "GMhQuzhygPDmyNW5RN6p2vMOLokLUkt86TPyObJwE7E"
	clientSecret = "qGDlFgu--O4j9fy4ZIs5ov_nl_Oq32-rWdWdxHjN2hg"
	accessToken  = "1FP-aJ5pPbhMdoaGLuVNDSOT2HeO7BWPciK8ST4_a8o"

	defaultLimit          = 20
	defaultMastodonUserID = "13179"
	website               = "https://github.com/Warp-net/warpnet"

	MastodonNetwork = "mastodon"
)

type warpnetMastodonPseudoNode struct {
	ctx                    context.Context
	pseudoPeerID           warpnet.WarpPeerID
	nodeInfo               warpnet.NodeInfo
	bridge                 *mastodon.Client
	proxyUser, defaultUser domain.User
}

func NewWarpnetMastodonPseudoNode(
	ctx context.Context,
	version *semver.Version,
) (*warpnetMastodonPseudoNode, error) {
	pseudoPeerID := warpnet.FromStringToPeerID(mastodonPseudoPeerID)

	config := &mastodon.Config{
		Server:       mastodonServer,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AccessToken:  accessToken,
	}

	c := mastodon.NewClient(config)

	acct, err := c.GetAccountCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("mastodon: failed to get current user: %w", err)
	}

	n := &warpnetMastodonPseudoNode{
		ctx:          ctx,
		pseudoPeerID: pseudoPeerID,
		bridge:       c,
		proxyUser: domain.User{
			AvatarKey:          acct.AvatarStatic,
			BackgroundImageKey: acct.HeaderStatic,
			Bio:                stripper.StripTags(acct.Note),
			CreatedAt:          acct.CreatedAt,
			FolloweesCount:     uint64(acct.FollowingCount),
			FollowersCount:     uint64(acct.FollowersCount),
			Id:                 string(acct.ID),
			NodeId:             pseudoPeerID.String(),
			TweetsCount:        uint64(acct.StatusesCount),
			Username:           acct.DisplayName,
			Website:            func(s string) *string { return &s }(website),
			Network:            MastodonNetwork,
		},
		nodeInfo: warpnet.NodeInfo{
			OwnerId:        string(acct.ID),
			ID:             mastodonPseudoPeerID,
			Version:        version,
			Addresses:      []string{mastodonPseudoMaddr},
			StartTime:      time.Now(),
			RelayState:     "off",
			BootstrapPeers: nil,
			Reachability:   warpnet.ReachabilityPublic,
		},
	}

	n.defaultUser, err = n.getUserHandler(defaultMastodonUserID)
	return n, err
}

func (m *warpnetMastodonPseudoNode) ID() warpnet.WarpPeerID {
	if m == nil {
		return ""
	}
	return m.pseudoPeerID
}

func (m *warpnetMastodonPseudoNode) IsMastodonID(id warpnet.WarpPeerID) bool {
	if m == nil {
		return false
	}
	return m.pseudoPeerID == id
}

func (m *warpnetMastodonPseudoNode) WarpnetUser() domain.User {
	if m == nil {
		return domain.User{}
	}
	return m.proxyUser
}

func (m *warpnetMastodonPseudoNode) DefaultUser() domain.User {
	if m == nil {
		return domain.User{}
	}
	return m.defaultUser
}

func (m *warpnetMastodonPseudoNode) Addrs() []warpnet.WarpAddress {
	if m == nil {
		return []warpnet.WarpAddress{}
	}
	pseudoMaddr, err := warpnet.NewMultiaddr(m.nodeInfo.Addresses[0])
	if err != nil {
		log.Errorf("pseudo mastodon: failed to parse address %s", err)
		return nil
	}
	return []warpnet.WarpAddress{pseudoMaddr}
}

func (m *warpnetMastodonPseudoNode) Route(r stream.WarpRoute, payload any) (_ []byte, err error) {
	var (
		getOneEvent event.GetTweetEvent
		getAllEvent event.GetAllTweetsEvent
		getImage    event.GetImageEvent
		resp        interface{}
	)
	var data []byte
	if payload != nil {
		var ok bool
		data, ok = payload.([]byte)
		if !ok {
			data, err = json.Marshal(payload)
			if err != nil {
				return nil, err
			}
		}
	}

	switch r.String() {
	case event.PUBLIC_GET_INFO:
		resp = m.getInfoHandler()
	case event.PUBLIC_GET_USER:
		_ = json.Unmarshal(data, &getOneEvent)
		resp, err = m.getUserHandler(getOneEvent.UserId)
	case event.PUBLIC_GET_USERS:
		_ = json.Unmarshal(data, &getAllEvent)
		resp, err = m.getUsersHandler(getAllEvent.UserId, getAllEvent.Cursor)
	case event.PUBLIC_GET_TWEETS:
		_ = json.Unmarshal(data, &getAllEvent)
		resp, err = m.getTweetsHandler(getAllEvent.UserId, getAllEvent.Cursor)
	case event.PUBLIC_GET_TWEET:
		_ = json.Unmarshal(data, &getOneEvent)
		resp, err = m.getTweetHandler(getOneEvent.TweetId)
	case event.PUBLIC_GET_TWEET_STATS:
		_ = json.Unmarshal(data, &getOneEvent)
		resp, err = m.getTweetStatsHandler(getOneEvent.TweetId)
	case event.PUBLIC_GET_REPLIES:
		_ = json.Unmarshal(data, &getOneEvent)
		resp, err = m.getRepliesHandler(getOneEvent.TweetId)
	case event.PUBLIC_GET_FOLLOWERS:
		_ = json.Unmarshal(data, &getAllEvent)
		resp, err = m.getFollowersHandler(getAllEvent.UserId, getAllEvent.Cursor)
	case event.PUBLIC_GET_FOLLOWEES:
		_ = json.Unmarshal(data, &getAllEvent)
		resp, err = m.getFolloweesHandler(getAllEvent.UserId, getAllEvent.Cursor)
	case event.PUBLIC_GET_IMAGE:
		_ = json.Unmarshal(data, &getImage)
		resp, err = m.getImageHandler(getImage.Key)
	default:
		return nil, warpnet.WarpError("unknown route")
	}
	if err != nil {
		return nil, fmt.Errorf("mastodon: failed to handle request, route: %s, message: %w", r.String(), err)
	}

	return json.Marshal(resp)
}

func (m *warpnetMastodonPseudoNode) getInfoHandler() warpnet.NodeInfo {
	return m.nodeInfo
}

func (m *warpnetMastodonPseudoNode) getUserHandler(userId string) (domain.User, error) {
	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(userId))

	acct, err := m.bridge.GetAccount(m.ctx, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("masotodon: bridge: get account: %w", err)
	}

	var birthdate, site string
	for _, f := range acct.Fields {
		if f.Name == "birthdate" {
			birthdate = f.Value
		}
		if f.Name == "website" {
			site = f.Value
		}
	}

	warpnetUser := domain.User{
		AvatarKey:          acct.AvatarStatic,
		BackgroundImageKey: acct.HeaderStatic,
		Bio:                stripper.StripTags(acct.Note),
		Birthdate:          birthdate,
		CreatedAt:          acct.CreatedAt,
		FolloweesCount:     uint64(acct.FollowingCount),
		FollowersCount:     uint64(acct.FollowersCount),
		Id:                 string(acct.ID),
		IsOffline:          false,
		NodeId:             m.pseudoPeerID.String(),
		Latency:            math.MaxInt64, // TODO
		TweetsCount:        uint64(acct.StatusesCount),
		Username:           acct.DisplayName,
		Website:            &site,
		Network:            MastodonNetwork,
	}
	return warpnetUser, nil
}

func (m *warpnetMastodonPseudoNode) getUsersHandler(userId string, cursor *string) (event.UsersResponse, error) {
	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(userId))

	pagination := &mastodon.Pagination{
		Limit: defaultLimit,
	}
	if cursor != nil {
		var cursorId mastodon.ID
		_ = cursorId.UnmarshalJSON([]byte(*cursor))
		pagination.SinceID = cursorId
	}
	defaultUsers := event.UsersResponse{
		Users: []domain.User{m.defaultUser, m.proxyUser},
	}

	followers, err := m.bridge.GetAccountFollowers(m.ctx, id, pagination)
	if err != nil {
		return defaultUsers, err
	}
	if len(followers) == 0 {
		return defaultUsers, nil
	}

	resp := event.UsersResponse{
		Users:  make([]domain.User, 0, len(followers)),
		Cursor: string(followers[len(followers)-1].ID),
	}
	resp.Users = append(resp.Users, defaultUsers.Users...)

	for _, acct := range followers {
		if acct == nil {
			continue
		}
		if acct.StatusesCount == 0 && acct.AvatarStatic == "" { // TODO
			continue
		}
		var birthdate, site string

		for _, f := range acct.Fields {
			if f.Name == "birthdate" {
				birthdate = f.Value
			}
			if f.Name == "website" {
				site = f.Value
			}
		}

		u := domain.User{
			AvatarKey:          acct.AvatarStatic,
			BackgroundImageKey: acct.HeaderStatic,
			Bio:                stripper.StripTags(acct.Note),
			Birthdate:          birthdate,
			CreatedAt:          acct.CreatedAt,
			FolloweesCount:     uint64(acct.FollowingCount),
			FollowersCount:     uint64(acct.FollowersCount),
			Id:                 string(acct.ID),
			IsOffline:          false,
			NodeId:             m.pseudoPeerID.String(),
			Latency:            math.MaxInt64, // TODO
			TweetsCount:        uint64(acct.StatusesCount),
			Username:           acct.DisplayName,
			Website:            &site,
			Network:            MastodonNetwork,
		}
		resp.Users = append(resp.Users, u)
	}

	return resp, nil
}

func (m *warpnetMastodonPseudoNode) getTweetsHandler(userId string, cursor *string) (event.TweetsResponse, error) {
	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(userId))

	pagination := &mastodon.Pagination{
		Limit: defaultLimit,
	}
	if cursor != nil {
		var cursorId mastodon.ID
		_ = cursorId.UnmarshalJSON([]byte(*cursor))
		pagination.SinceID = cursorId
	}

	toots, err := m.bridge.GetAccountStatuses(m.ctx, id, pagination)
	if err != nil {
		return event.TweetsResponse{}, err
	}
	if len(toots) == 0 {
		return event.TweetsResponse{}, nil
	}

	resp := event.TweetsResponse{
		Cursor: string(toots[len(toots)-1].ID),
		UserId: userId,
		Tweets: make([]domain.Tweet, 0, len(toots)),
	}

	for _, toot := range toots {
		if toot == nil {
			continue
		}

		media := toot.MediaAttachments
		imageKey := ""
		if len(media) > 0 && media[0].Type == "image" {
			imageKey = media[0].URL // TODO all images!
		}

		var (
			retweetedBy *string
			parentId    string
			tweetId     = string(toot.ID)
			content     = stripper.StripTags(toot.Content)
			username    = toot.Account.DisplayName
			tootUserId  = string(toot.Account.ID)
		)
		if pid, ok := toot.InReplyToID.(string); ok {
			parentId = pid
		}

		originalTweet := toot.Reblog
		if originalTweet != nil {
			retweetedBy = func(s string) *string { return &s }(string(toot.Account.ID))
			tweetId = string(originalTweet.Account.ID)
			if pid, ok := originalTweet.InReplyToID.(string); ok {
				parentId = pid
			}
			content = stripper.StripTags(originalTweet.Content)
			username = originalTweet.Account.DisplayName
			tootUserId = string(originalTweet.Account.ID)
		}

		resp.Tweets = append(resp.Tweets, domain.Tweet{
			CreatedAt:   toot.CreatedAt,
			Id:          tweetId,
			ParentId:    &parentId,
			RetweetedBy: retweetedBy,
			RootId:      parentId,
			Text:        content,
			UserId:      tootUserId,
			Username:    username,
			ImageKey:    imageKey,
			Network:     MastodonNetwork,
		})
	}

	return resp, nil
}

func (m *warpnetMastodonPseudoNode) getTweetHandler(tweetId string) (domain.Tweet, error) {
	tweetId = strings.TrimPrefix(tweetId, domain.RetweetPrefix)

	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(tweetId))
	status, err := m.bridge.GetStatus(m.ctx, id)
	if err != nil {
		return domain.Tweet{}, err
	}

	media := status.MediaAttachments
	imageKey := ""
	if len(media) > 0 && media[0].Type == "image" {
		imageKey = media[0].URL
	}

	var (
		retweetedBy *string
		parentId    string
		statusId    = string(status.ID)
		content     = stripper.StripTags(status.Content)
		username    = status.Account.DisplayName
		userId      = string(status.Account.ID)
	)
	if pid, ok := status.InReplyToID.(string); ok {
		parentId = pid
	}

	originalTweet := status.Reblog
	if originalTweet != nil {
		retweetedBy = func(s string) *string { return &s }(string(status.Account.ID))
		statusId = string(originalTweet.Account.ID)
		if pid, ok := originalTweet.InReplyToID.(string); ok {
			parentId = pid
		}
		content = stripper.StripTags(originalTweet.Content)
		username = originalTweet.Account.DisplayName
		userId = string(originalTweet.Account.ID)
	}

	tweet := domain.Tweet{
		CreatedAt:   status.CreatedAt,
		Id:          statusId,
		ParentId:    &parentId,
		RetweetedBy: retweetedBy,
		RootId:      parentId,
		Text:        content,
		UserId:      userId,
		Username:    username,
		ImageKey:    imageKey,
		Network:     MastodonNetwork,
	}
	return tweet, nil
}

func (m *warpnetMastodonPseudoNode) getTweetStatsHandler(tweetId string) (event.TweetStatsResponse, error) {
	tweetId = strings.TrimPrefix(tweetId, domain.RetweetPrefix)

	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(tweetId))

	status, err := m.bridge.GetStatus(m.ctx, id)
	if err != nil {
		var apiErr *mastodon.APIError
		if errors.As(err, &apiErr) {
			if apiErr.StatusCode == http.StatusNotFound {
				return event.TweetStatsResponse{}, nil
			}
		}
		return event.TweetStatsResponse{}, err
	}

	stats := event.TweetStatsResponse{
		TweetId:       event.ID(status.ID),
		RetweetsCount: uint64(status.ReblogsCount),
		LikeCount:     uint64(status.FavouritesCount),
		RepliesCount:  uint64(status.RepliesCount),
		ViewsCount:    0,
	}
	return stats, nil
}

func (m *warpnetMastodonPseudoNode) getRepliesHandler(tweetId string) (event.RepliesResponse, error) {
	tweetId = strings.TrimPrefix(tweetId, domain.RetweetPrefix)

	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(tweetId))
	replies, err := m.bridge.GetStatusContext(m.ctx, id)
	if err != nil {
		return event.RepliesResponse{}, err
	}

	resp := event.RepliesResponse{
		Cursor:  "",
		Replies: make([]domain.ReplyNode, 0, len(replies.Descendants)),
		UserId:  nil,
	}

	for _, status := range replies.Descendants {
		if status == nil {
			continue
		}

		media := status.MediaAttachments
		imageKey := ""
		if len(media) > 0 && media[0].Type == "image" { // TODO
			imageKey = media[0].URL
		}

		var (
			retweetedBy *string
			statusId    = string(status.ID)
		)
		if status.Reblog != nil {
			retweetedBy = func(s string) *string { return &s }(string(status.Reblog.Account.ID))
			statusId = string(status.ID)
		}

		parentId := ""
		if pid, ok := status.InReplyToID.(string); ok {
			parentId = pid
		}

		tweet := domain.Tweet{
			CreatedAt:   status.CreatedAt,
			Id:          statusId,
			ParentId:    &parentId,
			RetweetedBy: retweetedBy,
			RootId:      parentId,
			Text:        stripper.StripTags(status.Content),
			UserId:      string(status.Account.ID),
			Username:    status.Account.DisplayName,
			ImageKey:    imageKey,
			Network:     MastodonNetwork,
		}
		resp.Replies = append(resp.Replies, domain.ReplyNode{Reply: tweet})
	}
	return resp, nil
}

func (m *warpnetMastodonPseudoNode) getFollowersHandler(userId string, cursor *string) (event.FollowersResponse, error) {
	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(userId))

	pagination := &mastodon.Pagination{
		Limit: defaultLimit,
	}
	if cursor != nil {
		var cursorId mastodon.ID
		_ = cursorId.UnmarshalJSON([]byte(*cursor))
		pagination.SinceID = cursorId
	}

	followers, err := m.bridge.GetAccountFollowers(m.ctx, id, pagination)
	if err != nil {
		return event.FollowersResponse{}, err
	}
	if len(followers) == 0 {
		return event.FollowersResponse{}, nil
	}

	resp := event.FollowersResponse{
		Followee:  userId,
		Followers: make([]domain.Following, 0, len(followers)),
		Cursor:    string(followers[len(followers)-1].ID),
	}

	for _, follower := range followers {
		if follower == nil {
			continue
		}
		resp.Followers = append(resp.Followers, domain.Following{
			Followee: userId,
			Follower: string(follower.ID),
		})
	}

	return resp, nil
}

func (m *warpnetMastodonPseudoNode) getFolloweesHandler(userId string, cursor *string) (event.FolloweesResponse, error) {
	var id mastodon.ID
	_ = id.UnmarshalJSON([]byte(userId))

	pagination := &mastodon.Pagination{
		Limit:   defaultLimit,
		SinceID: "",
	}
	if cursor != nil {
		var cursorId mastodon.ID
		_ = cursorId.UnmarshalJSON([]byte(*cursor))
		pagination.SinceID = cursorId
	}

	followees, err := m.bridge.GetAccountFollowing(m.ctx, id, pagination)
	if err != nil {
		return event.FolloweesResponse{}, err
	}
	if len(followees) == 0 {
		return event.FolloweesResponse{}, nil
	}

	resp := event.FolloweesResponse{
		Follower:  userId,
		Followees: make([]domain.Following, 0, len(followees)),
		Cursor:    string(followees[len(followees)-1].ID),
	}

	for _, followee := range followees {
		if followee == nil {
			continue
		}
		resp.Followees = append(resp.Followees, domain.Following{
			Followee: string(followee.ID),
			Follower: userId,
		})
	}

	return resp, nil
}

func (m *warpnetMastodonPseudoNode) getImageHandler(url string) (event.GetImageResponse, error) {
	resp, err := m.bridge.Get(url)
	if err != nil {
		return event.GetImageResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bt, err := io.ReadAll(resp.Body)
	if err != nil {
		return event.GetImageResponse{}, err
	}

	prefix := ""
	contentType := resp.Header.Get("content-type")
	switch contentType {
	case "image/jpeg":
		prefix = "data:image/jpeg;base64,"
	case "image/png":
		prefix = "data:image/png;base64,"
	case "image/gif":
		prefix = "data:image/gif;base64,"
	case "image/webp":
		prefix = "data:image/webp;base64,"
	default:
		return event.GetImageResponse{File: string(bt)}, errors.New("unknown image type")
	}

	encoded := base64.StdEncoding.EncodeToString(bt)

	return event.GetImageResponse{File: prefix + encoded}, nil
}
