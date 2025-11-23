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

package domain

import (
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
)

type ID = string

// AuthNodeInfo defines model for AuthNodeInfo.
type AuthNodeInfo struct {
	Identity Identity         `json:"identity"`
	NodeInfo warpnet.NodeInfo `json:"node_info"`
}

// Chat defines model for Chat.
type Chat struct {
	CreatedAt   time.Time `json:"created_at"`
	Id          string    `json:"id"`
	OtherUserId string    `json:"other_user_id"`
	OwnerId     string    `json:"owner_id"`
	LastMessage string    `json:"last_message"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ChatMessage defines model for ChatMessage.
type ChatMessage struct {
	ChatId     string    `json:"chat_id"`
	CreatedAt  time.Time `json:"created_at"`
	Id         string    `json:"id"`
	ReceiverId string    `json:"receiver_id"`
	SenderId   string    `json:"sender_id"`
	Text       string    `json:"text"`
	Status     string    `json:"status,omitempty"`
}

// Error defines model for Error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return e.Message
}

// Identity defines model for Identity.
type Identity struct {
	Owner Owner  `json:"owner"`
	Token string `json:"token"`
}

// Like defines model for Like.
type Like struct {
	TweetId string `json:"tweet_id"`
	UserId  string `json:"user_id"`
}

// Owner defines model for Owner.
type Owner struct {
	CreatedAt       time.Time `json:"created_at"`
	NodeId          string    `json:"node_id"`
	UserId          string    `json:"user_id"`
	RedundantUserID string    `json:"id"`
	Username        string    `json:"username"`
}

// ReplyNode defines model for ReplyNode.
type ReplyNode struct {
	Children []ReplyNode `json:"children"`
	Reply    Tweet       `json:"reply"`
}

const RetweetPrefix = "RT:"

// Tweet defines model for Tweet.
type Tweet struct {
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Id        string     `json:"id"`
	ParentId  *string    `json:"parent_id,omitempty"`

	// RetweetedBy retweeted by user id
	RetweetedBy *string          `json:"retweeted_by,omitempty"`
	RootId      string           `json:"root_id"`
	Text        string           `json:"text"`
	UserId      string           `json:"user_id"`
	Username    string           `json:"username"`
	ImageKey    string           `json:"image_key"`
	Network     string           `json:"network"`
	Moderation  *TweetModeration `json:"moderation,omitempty"`
}

func (t *Tweet) IsModerated() bool {
	return t.Moderation != nil
}

type ModelType string

const LLAMA2 ModelType = "llama2"

type TweetModeration struct {
	ModeratorID ID               `json:"moderator_id"`
	Model       ModelType        `json:"model"`
	IsOk        ModerationResult `json:"is_ok"`
	Reason      *string          `json:"reason"`
	TimeAt      time.Time        `json:"time_at"`
}

// User defines model for User.
type User struct {
	// Avatar mime type + "," + base64
	AvatarKey string `json:"avatar_key,omitempty"`

	// BackgroundImage mime type + "," + base64
	BackgroundImageKey string            `json:"background_image_key"`
	Bio                string            `json:"bio"`
	Birthdate          string            `json:"birthdate"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          *time.Time        `json:"updated_at,omitempty"`
	FollowingsCount    int64             `json:"followings_count"`
	FollowersCount     int64             `json:"followers_count"`
	Id                 string            `json:"id"`
	IsOffline          bool              `json:"isOffline"`
	NodeId             string            `json:"node_id"`
	Network            string            `json:"network"`
	RoundTripTime      int64             `json:"rtt"`
	TweetsCount        int64             `json:"tweets_count"`
	Username           string            `json:"username"`
	Website            *string           `json:"website,omitempty"`
	Moderation         *UserModeration   `json:"moderation"`
	Metadata           map[string]string `json:"metadata"`
}

type UserModeration struct {
	IsModerated bool      `json:"is_moderated"`
	Model       ModelType `json:"model"`
	IsOk        bool      `json:"is_ok"`
	Reason      *string   `json:"reason"`
	Strikes     uint8     `json:"strikes"`
	TimeAt      time.Time `json:"time_at"`
}

type NotificationType string

func (n NotificationType) String() string {
	return string(n)
}

const (
	NotificationModerationType NotificationType = "moderation"
	NotificationRetweetType    NotificationType = "retweet"
	NotificationFollowType     NotificationType = "follow"
	NotificationLikeType       NotificationType = "like"
	NotificationMentionType    NotificationType = "mention"
	NotificationReplyType      NotificationType = "reply"
)

type Notification struct {
	Type      NotificationType `json:"type"`
	Id        string           `json:"id"`
	Text      string           `json:"text"`
	UserId    string           `json:"user_id"`
	IsRead    bool             `json:"is_read"`
	CreatedAt time.Time        `json:"created_at"`
}

type ModerationResult bool

const (
	OK   ModerationResult = true
	FAIL ModerationResult = false
)

type ModerationObjectType int

const (
	ModerationUserType ModerationObjectType = iota
	ModerationTweetType
	ModerationReplyType
	ModerationImageType
)

func (t ModerationObjectType) String() string {
	switch t {
	case ModerationUserType:
		return "user description"
	case ModerationTweetType:
		return "tweet text"
	case ModerationReplyType:
		return "reply text"
	case ModerationImageType:
		return "image content"
	default:
		return "unknown"
	}

}
