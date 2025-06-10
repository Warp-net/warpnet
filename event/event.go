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

package event

import (
	"github.com/Warp-net/warpnet/domain"
	json "github.com/json-iterator/go"
	"time"
)

const (
	Accepted acceptedResponse = `{"code":0,"message":"Accepted"}`
)

type ID = string

type acceptedResponse string

// ChatCreatedResponse defines model for ChatCreatedResponse.
type ChatCreatedResponse = domain.Chat

type GetChatResponse = domain.Chat

// ChatMessageResponse defines model for ChatMessageResponse.
type ChatMessageResponse = domain.ChatMessage

// ChatMessagesResponse defines model for ChatMessagesResponse.
type ChatMessagesResponse struct {
	ChatId   string               `json:"chat_id"`
	Cursor   string               `json:"cursor"`
	Messages []domain.ChatMessage `json:"messages"`
}

// ChatsResponse defines model for ChatsResponse.
type ChatsResponse struct {
	Chats  []domain.Chat `json:"chats"`
	Cursor string        `json:"cursor"`
	UserId string        `json:"user_id"`
}

// DeleteChatEvent defines model for DeleteChatEvent.
type DeleteChatEvent struct {
	ChatId string `json:"chat_id"`
}

// DeleteMessageEvent defines model for DeleteMessageEvent.
type DeleteMessageEvent = GetMessageEvent

// DeleteReplyEvent defines model for DeleteReplyEvent.
type DeleteReplyEvent = GetReplyEvent

// DeleteTweetEvent defines model for DeleteTweetEvent.
type DeleteTweetEvent = GetTweetEvent

// ErrorEvent defines model for ErrorEvent.
type ErrorEvent struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse defines model for ErrorResponse.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e ErrorResponse) Error() string {
	return e.Message
}

// FolloweesResponse defines model for FolloweesResponse.
type FolloweesResponse struct {
	Cursor    string             `json:"cursor"`
	Followees []domain.Following `json:"followees"`
	Follower  string             `json:"follower"`
}

// FollowersResponse defines model for FollowersResponse.
type FollowersResponse struct {
	Cursor    string             `json:"cursor"`
	Followee  string             `json:"followee"`
	Followers []domain.Following `json:"followers"`
}

// GetAllChatsEvent defines model for GetAllChatsEvent.
type GetAllChatsEvent struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`
	UserId string  `json:"user_id"`
}

// GetAllMessagesEvent defines model for GetAllMessagesEvent.
type GetAllMessagesEvent struct {
	ChatId string  `json:"chat_id"`
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`
}

// GetAllRepliesEvent defines model for GetAllRepliesEvent.
type GetAllRepliesEvent struct {
	Cursor   *string `json:"cursor,omitempty"`
	Limit    *uint64 `json:"limit,omitempty"`
	ParentId string  `json:"parent_id"`
	RootId   string  `json:"root_id"`
}

// GetAllTweetsEvent defines model for GetAllTweetsEvent.
type GetAllTweetsEvent struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`
	UserId string  `json:"user_id"`
}

// GetAllUsersEvent defines model for GetAllUsersEvent.
type GetAllUsersEvent struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`

	// UserId default owner
	UserId string `json:"user_id"`
}

// GetChatEvent defines model for GetChatEvent.
type GetChatEvent struct {
	ChatId string `json:"chat_id"`
}

// GetFolloweesEvent defines model for GetFolloweesEvent.
type GetFolloweesEvent = GetFollowersEvent

// GetFollowersEvent defines model for GetFollowersEvent.
type GetFollowersEvent struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`
	UserId string  `json:"user_id"`
}

// GetLikersEvent defines model for GetLikersEvent.
type GetReactorsEvent struct {
	Cursor  *string `json:"cursor,omitempty"`
	Limit   *uint64 `json:"limit,omitempty"`
	TweetId string  `json:"tweet_id"`
}

// GetLikersResponse defines model for GetLikersResponse.
type GetLikersResponse = UsersResponse

// GetLikesCountEvent defines model for GetLikesCountEvent.
type GetLikesCountEvent struct {
	TweetId string `json:"tweet_id"`
}

// GetMessageEvent defines model for GetMessageEvent.
type GetMessageEvent struct {
	ChatId string `json:"chat_id"`
	Id     string `json:"id"`
}

type GetTweetStatsEvent struct {
	TweetId string `json:"tweet_id"`
	UserId  string `json:"user_id"`
}

// GetReTweetsCountEvent defines model for GetReTweetsCountEvent.
type GetReTweetsCountEvent = GetLikesCountEvent

// GetReplyEvent defines model for GetReplyEvent.
type GetReplyEvent struct {
	ReplyId string `json:"reply_id"`
	RootId  string `json:"root_id"`
	UserId  string `json:"user_id"`
}

// GetRetweetersResponse defines model for GetRetweetersResponse.
type GetRetweetersResponse = UsersResponse

// GetTimelineEvent defines model for GetTimelineEvent.
type GetTimelineEvent = GetAllTweetsEvent

// GetTweetEvent defines model for GetTweetEvent.
type GetTweetEvent struct {
	TweetId string `json:"tweet_id"`
	UserId  string `json:"user_id"`
}

// GetUserEvent defines model for GetUserEvent.
type GetUserEvent struct {
	UserId string `json:"user_id"`
}

// LikeEvent defines model for LikeEvent.
type LikeEvent struct {
	TweetId string `json:"tweet_id"`
	UserId  string `json:"user_id"`
	OwnerId string `json:"owner_id"`
}

// LikesCountResponse defines model for LikesCountResponse.
type LikesCountResponse struct {
	Count uint64 `json:"count"`
}

// LoginEvent defines model for LoginEvent.
type LoginEvent struct {
	Password string `json:"password"`
	Username string `json:"username"`
}

// LoginResponse defines model for LoginResponse.
type LoginResponse = domain.AuthNodeInfo

// LogoutEvent defines model for LogoutEvent.
type LogoutEvent struct {
	Token string `json:"token"`
}

// Message defines model for Message.
type Message struct {
	Body      *json.RawMessage `json:"body,omitempty"`
	MessageId string           `json:"message_id"`
	NodeId    string           `json:"node_id"`
	Path      string           `json:"path"`
	Timestamp time.Time        `json:"timestamp,omitempty"`
	Version   string           `json:"version"`
}

// MessageBody defines model for Message.Body.
type MessageBody any

// NewChatEvent defines model for NewChatEvent.
type NewChatEvent struct {
	ChatId      *string `json:"chat_id,omitempty"`
	OtherUserId string  `json:"other_user_id"`
	OwnerId     string  `json:"owner_id"`
}

// NewFollowEvent defines model for NewFollowEvent.
type NewFollowEvent = domain.Following

// NewMessageEvent defines model for NewMessageEvent.
type NewMessageEvent = domain.ChatMessage

// NewMessageResponse defines model for NewMessageResponse.
type NewMessageResponse = domain.ChatMessage

// NewReplyEvent defines model for NewReplyEvent.
type NewReplyEvent struct {
	CreatedAt    time.Time `json:"created_at"`
	Id           string    `json:"id"`
	ParentId     *string   `json:"parent_id,omitempty"`
	ParentUserId string    `json:"parent_user_id"`
	RootId       string    `json:"root_id"`
	Text         string    `json:"text"`
	UserId       string    `json:"user_id"`
	Username     string    `json:"username"`
}

// NewReplyResponse defines model for NewReplyResponse.
type NewReplyResponse = domain.Tweet

// NewRetweetEvent defines model for NewRetweetEvent.
type NewRetweetEvent = domain.Tweet

// NewTweetEvent defines model for NewTweetEvent.
type NewTweetEvent = domain.Tweet

// NewUnfollowEvent defines model for NewUnfollowEvent.
type NewUnfollowEvent = domain.Following

// NewUserEvent defines model for NewUserEvent.
type NewUserEvent = domain.User

// Owner defines model for Owner.
type Owner = domain.Owner

// ReTweetsCountResponse defines model for ReTweetsCountResponse.
type ReTweetsCountResponse = LikesCountResponse

// RepliesResponse defines model for RepliesTreeResponse.
type RepliesResponse struct {
	Cursor  string             `json:"cursor"`
	Replies []domain.ReplyNode `json:"replies"`
	UserId  *string            `json:"user_id,omitempty"`
}

// TweetsResponse defines model for TweetsResponse.
type TweetsResponse struct {
	Cursor string         `json:"cursor"`
	Tweets []domain.Tweet `json:"tweets"`
	UserId string         `json:"user_id"`
}

type TweetStatsResponse struct {
	TweetId       ID     `json:"tweet_id"`
	RetweetsCount uint64 `json:"retweets_count"`
	LikeCount     uint64 `json:"likes_count"`
	RepliesCount  uint64 `json:"replies_count"`
	ViewsCount    uint64 `json:"views_count"`
}

type IDsResponse struct {
	Cursor string `json:"cursor"`
	Users  []ID   `json:"users"`
}

// UnlikeEvent defines model for UnlikeEvent.
type UnlikeEvent = LikeEvent

// UnretweetEvent defines model for UnretweetEvent.
type UnretweetEvent struct {
	TweetId     string `json:"tweet_id"`
	RetweeterId string `json:"retweeter_id"`
}

// UsersResponse defines model for UsersResponse.
type UsersResponse struct {
	Cursor string        `json:"cursor"`
	Users  []domain.User `json:"users"`
}

type UploadImageEvent struct {
	// Image mime type + "," + base64
	File string `json:"file"`
}

type UploadImageResponse struct {
	Key string `json:"key"`
}

type GetImageEvent struct {
	UserId string `json:"user_id"`
	// Image mime type + "," + base64
	Key string `json:"key"`
}

type GetImageResponse struct {
	// Image mime type + "," + base64
	File string `json:"file"`
}

type GetChallengeEvent struct {
	DirStack  []int `json:"dir_stack"` // every index is level and value is dir num
	FileStack []int `json:"file_stack"`
	Nonce     int64 `json:"nonce"`
}

type GetChallengeResponse struct {
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
}

type ValidationEvent struct {
	ValidatedNodeID string       `json:"validated_node_id"`
	SelfHashHex     string       `json:"self_hash_hex"`
	User            *domain.User `json:"user"`
}

type ValidationResult int

const (
	Invalid ValidationResult = 0
	Valid   ValidationResult = 1
)

type ValidationEventResponse struct {
	Result      ValidationResult `json:"result"`
	Reason      *string          `json:"reason"`
	ValidatedID string           `json:"validated_id"`
	ValidatorID string           `json:"validator_id"`
}
