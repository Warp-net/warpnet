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

package main

// Wire DTOs copied from warpnet's domain/event packages so the gateway depends
// only on the protocol's JSON shapes, not on the warpnet Go packages. These are
// minimal subsets — JSON tags match the node exactly; unread fields are omitted.

import (
	"encoding/json"
	"time"
)

// Warpnet public-route protocol IDs (each is also the libp2p protocol string).
const (
	routeGetUser       = "/public/get/user/0.0.0"
	routeGetTweet      = "/public/get/tweet/0.0.0"
	routeGetTweets     = "/public/get/tweets/0.0.0"
	routeGetFollowers  = "/public/get/followers/0.0.0"
	routeGetFollowings = "/public/get/followings/0.0.0"
	routeGetImage      = "/public/get/image/0.0.0"
	routePostFollow    = "/public/post/follow/0.0.0"
	routePostUnfollow  = "/public/post/unfollow/0.0.0"
	routePostLike      = "/public/post/like/0.0.0"
	routePostUnlike    = "/public/post/unlike/0.0.0"
	routePostRetweet   = "/public/post/retweet/0.0.0"
	routePostUnretweet = "/public/post/unretweet/0.0.0"
	routePostReply     = "/public/post/reply/0.0.0"
)

// message is the signed stream envelope (warpnet's event.Message).
type message struct {
	Body        json.RawMessage `json:"body"`
	MessageId   string          `json:"message_id"`
	NodeId      string          `json:"node_id"`
	Destination string          `json:"path"`
	Timestamp   time.Time       `json:"timestamp"`
	Version     string          `json:"version"`
	Signature   string          `json:"signature"`
}

// tweet is the subset of domain.Tweet the gateway reads and writes. The retweet
// route also takes a Tweet, so this doubles as the new-retweet payload.
type tweet struct {
	CreatedAt   time.Time `json:"created_at"`
	Id          string    `json:"id"`
	ParentId    *string   `json:"parent_id,omitempty"`
	RetweetedBy *string   `json:"retweeted_by,omitempty"`
	RootId      string    `json:"root_id"`
	Text        string    `json:"text"`
	UserId      string    `json:"user_id"`
	ImageKeys   []string  `json:"image_keys,omitempty"`
}

// user is the subset of domain.User the gateway renders into an actor.
type user struct {
	Id       string `json:"id"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
}

type getUserEvent struct {
	UserId string `json:"user_id"`
}

type getTweetEvent struct {
	TweetId string `json:"tweet_id"`
	UserId  string `json:"user_id"`
}

type getAllTweetsEvent struct {
	UserId string `json:"user_id"`
}

type tweetsResponse struct {
	Tweets []tweet `json:"tweets"`
}

// getFollowersEvent is also used for the followings route (same shape).
type getFollowersEvent struct {
	UserId string `json:"user_id"`
}

type followersResponse struct {
	Followers []string `json:"followers"`
}

type followingsResponse struct {
	Followings []string `json:"followings"`
}

// newFollowEvent is also the unfollow payload (same shape).
type newFollowEvent struct {
	FollowerId  string `json:"follower_id"`
	FollowingId string `json:"following_id"`
}

// likeEvent is also the unlike payload (same shape).
type likeEvent struct {
	TweetId string `json:"tweet_id"`
	UserId  string `json:"user_id"`
	OwnerId string `json:"owner_id"`
}

type unretweetEvent struct {
	TweetId     string `json:"tweet_id"`
	RetweeterId string `json:"retweeter_id"`
}

type newReplyEvent struct {
	CreatedAt    time.Time `json:"created_at"`
	Id           string    `json:"id"`
	ParentId     *string   `json:"parent_id,omitempty"`
	ParentUserId string    `json:"parent_user_id"`
	RootId       string    `json:"root_id"`
	Text         string    `json:"text"`
	UserId       string    `json:"user_id"`
	Username     string    `json:"username"`
}

type getImageEvent struct {
	UserId string `json:"user_id"`
	Key    string `json:"key"`
}

type getImageResponse struct {
	File string `json:"file"`
}
