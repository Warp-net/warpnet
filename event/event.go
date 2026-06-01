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
	"time"

	"github.com/Warp-net/warpnet/domain"
	json "github.com/json-iterator/go"
)

const (
	// Accepted is the verbatim JSON body the middleware writes when a
	// handler signals plain success. Plain string (not a named type) so
	// the middleware's response type switch hits the `case string` arm
	// and writes the bytes as-is; with a named type it would fall through
	// to the JSON encoder and be re-emitted as a quoted string literal.
	Accepted            string = `{"code":0,"message":"Accepted"}`
	InternalRoutePrefix string = "/internal"
	EndCursor           string = "end"
)

// ChatCreatedResponse defines model for ChatCreatedResponse.
type ChatCreatedResponse = domain.Chat

type GetChatResponse = domain.Chat

// ChatMessageResponse defines model for ChatMessageResponse.
type ChatMessageResponse = domain.ChatMessage

// ChatMessagesResponse defines model for ChatMessagesResponse.
type ChatMessagesResponse struct {
	ChatId   domain.ID            `json:"chat_id"`
	Cursor   string               `json:"cursor"`
	Messages []domain.ChatMessage `json:"messages"`
}

// ChatsResponse defines model for ChatsResponse.
type ChatsResponse struct {
	Chats  []domain.Chat `json:"chats"`
	Cursor string        `json:"cursor"`
	UserId domain.ID     `json:"user_id"`
}

// DeleteChatEvent defines model for DeleteChatEvent.
type DeleteChatEvent struct {
	ChatId domain.ID `json:"chat_id"`
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
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e ResponseError) Error() string {
	return e.Message
}

// FollowingsResponse defines model for FollowingsResponse.
type FollowingsResponse struct {
	Cursor     string      `json:"cursor"`
	Followings []domain.ID `json:"followings"`
	FollowerId domain.ID   `json:"follower_id"`
}

// ImportTwitterArchiveEvent defines model for ImportTwitterArchiveEvent.
//
// Exactly one source is set: ArchivePath (desktop member node — the node
// reads the .zip straight off local disk) or ArchiveData (business browser
// dashboard — base64 of the uploaded .zip, optionally data-URL prefixed).
type ImportTwitterArchiveEvent struct {
	ArchivePath string `json:"archive_path,omitempty"`
	ArchiveData string `json:"archive_data,omitempty"`
}

// ImportTwitterArchiveResponse defines model for ImportTwitterArchiveResponse.
type ImportTwitterArchiveResponse struct {
	ImportedTweets int `json:"imported_tweets"`
	ImportedImages int `json:"imported_images"`
	SkippedTweets  int `json:"skipped_tweets"`
}

// ImportTweetEvent defines model for ImportTweetEvent.
//
// One pre-parsed original tweet streamed from the business browser dashboard.
// The browser unzips and filters the X archive client-side (dropping retweets,
// replies, GIFs and videos) and streams only the kept tweets, so the node
// never buffers the whole archive. Images are raw base64 of the still photos
// (at most four), without a data-URL prefix.
type ImportTweetEvent struct {
	Id        string   `json:"id"`
	Text      string   `json:"text"`
	CreatedAt string   `json:"created_at"`
	Images    []string `json:"images,omitempty"`
}

type IsFollowingResponse struct {
	IsFollowing bool `json:"is_following"`
}

type IsFollowerResponse struct {
	IsFollower bool `json:"is_follower"`
}

type FollowersResponse struct {
	Cursor      string      `json:"cursor"`
	FollowingId string      `json:"following_id"`
	Followers   []domain.ID `json:"followers"`
}

// GetAllChatsEvent defines model for GetAllChatsEvent.
type GetAllChatsEvent struct {
	Cursor *string   `json:"cursor,omitempty"`
	Limit  *uint64   `json:"limit,omitempty"`
	UserId domain.ID `json:"user_id"`
}

// GetAllMessagesEvent defines model for GetAllMessagesEvent.
type GetAllMessagesEvent struct {
	OwnerId domain.ID `json:"owner_id"`
	ChatId  domain.ID `json:"chat_id"`
	Cursor  *string   `json:"cursor,omitempty"`
	Limit   *uint64   `json:"limit,omitempty"`
}

// GetAllRepliesEvent defines model for GetAllRepliesEvent.
//
// ParentId is the parent TWEET id (not a user id) — it selects which
// subtree of replies inside RootId to return. Empty means "top-level
// replies of the thread"; the handler treats that as ParentId = RootId.
// RootId is the root tweet of the thread.
type GetAllRepliesEvent struct {
	Cursor   *string   `json:"cursor,omitempty"`
	Limit    *uint64   `json:"limit,omitempty"`
	ParentId domain.ID `json:"parent_id"`
	RootId   domain.ID `json:"root_id"`
}

// GetAllTweetsEvent defines model for GetAllTweetsEvent.
type GetAllTweetsEvent struct {
	Cursor *string   `json:"cursor,omitempty"`
	Limit  *uint64   `json:"limit,omitempty"`
	UserId domain.ID `json:"user_id"`
}

// GetAllUsersEvent defines model for GetAllUsersEvent.
type GetAllUsersEvent struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`

	// UserId default owner
	UserId domain.ID `json:"user_id"`
}

// GetChatEvent defines model for GetChatEvent.
type GetChatEvent struct {
	ChatId domain.ID `json:"chat_id"`
}

// GetFollowingsEvent defines model for GetFollowingsEvent.
type GetFollowingsEvent = GetFollowersEvent

// GetFollowersEvent defines model for GetFollowersEvent.
type GetFollowersEvent struct {
	Cursor *string   `json:"cursor,omitempty"`
	Limit  *uint64   `json:"limit,omitempty"`
	UserId domain.ID `json:"user_id"`
}

type GetIsFollowingEvent struct {
	UserId domain.ID `json:"user_id"`
}

type GetIsFollowerEvent = GetIsFollowingEvent

// GetLikersResponse defines model for GetLikersResponse.
type GetLikersResponse = UsersResponse

// GetLikesCountEvent defines model for GetLikesCountEvent.
type GetLikesCountEvent struct {
	TweetId domain.ID `json:"tweet_id"`
}

// GetMessageEvent defines model for GetMessageEvent.
type GetMessageEvent struct {
	ChatId domain.ID `json:"chat_id"`
	Id     domain.ID `json:"id"`
}

type GetTweetStatsEvent struct {
	TweetId domain.ID `json:"tweet_id"`
	UserId  domain.ID `json:"user_id"`
}

// GetReTweetsCountEvent defines model for GetReTweetsCountEvent.
type GetReTweetsCountEvent = GetLikesCountEvent

// GetReplyEvent defines model for GetReplyEvent.
type GetReplyEvent struct {
	ReplyId domain.ID `json:"reply_id"`
	RootId  domain.ID `json:"root_id"`
	UserId  domain.ID `json:"user_id"`
}

// GetRetweetersResponse defines model for GetRetweetersResponse.
type GetRetweetersResponse = UsersResponse

// GetTimelineEvent defines model for GetTimelineEvent.
type GetTimelineEvent = GetAllTweetsEvent

// GetTweetEvent defines model for GetTweetEvent.
type GetTweetEvent struct {
	TweetId domain.ID `json:"tweet_id"`
	UserId  domain.ID `json:"user_id"`
}

// GetUserEvent defines model for GetUserEvent.
type GetUserEvent struct {
	UserId domain.ID `json:"user_id"`
}

// LikeEvent defines model for LikeEvent.
type LikeEvent struct {
	TweetId domain.ID `json:"tweet_id"`
	UserId  domain.ID `json:"user_id"`
	OwnerId domain.ID `json:"owner_id"`
}

// LikesCountResponse defines model for LikesCountResponse.
type LikesCountResponse struct {
	Count uint64 `json:"count"`
}

// LoginEvent defines model for LoginEvent.
type LoginEvent struct {
	Password string `json:"password"` //nolint:gosec
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
	Body        json.RawMessage `json:"body"`
	MessageId   domain.ID       `json:"message_id"`
	NodeId      domain.ID       `json:"node_id"`
	Destination string          `json:"path"` // TODO change to 'destination'
	Timestamp   time.Time       `json:"timestamp"`
	Version     string          `json:"version"`
	Signature   string          `json:"signature"`
}

// MessageBody defines model for Message.Body.
type MessageBody any

// NewChatEvent defines model for NewChatEvent.
type NewChatEvent struct {
	ChatId      *domain.ID `json:"chat_id,omitempty"`
	OtherUserId domain.ID  `json:"other_user_id"`
	OwnerId     domain.ID  `json:"owner_id"`
}

// NewFollowEvent defines model for NewFollowEvent.
type NewFollowEvent = struct {
	FollowerId  domain.ID `json:"follower_id"`
	FollowingId domain.ID `json:"following_id"`
}

// NewMessageEvent defines model for NewMessageEvent.
type NewMessageEvent = domain.ChatMessage

// NewMessageResponse defines model for NewMessageResponse.
type NewMessageResponse = domain.ChatMessage

// NewReplyEvent defines model for NewReplyEvent.
//
// ParentId is the parent TWEET id this reply is attached to (nil/empty
// means the reply hangs directly off RootId). ParentUserId is the user
// id of the parent tweet's author — that's the routing key the server
// uses to forward the request to the right node when the parent tweet
// lives on a remote peer.
type NewReplyEvent struct {
	CreatedAt    time.Time  `json:"created_at"`
	Id           domain.ID  `json:"id"`
	ParentId     *domain.ID `json:"parent_id,omitempty"`
	ParentUserId domain.ID  `json:"parent_user_id"`
	RootId       domain.ID  `json:"root_id"`
	Text         string     `json:"text"`
	UserId       domain.ID  `json:"user_id"`
	Username     string     `json:"username"`
}

// NewReplyResponse defines model for NewReplyResponse.
type NewReplyResponse = domain.Tweet

// NewRetweetEvent defines model for NewRetweetEvent.
type NewRetweetEvent = domain.Tweet

// NewTweetEvent defines model for NewTweetEvent.
type NewTweetEvent = domain.Tweet

// NewUnfollowEvent defines model for NewUnfollowEvent.
type NewUnfollowEvent = NewFollowEvent

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
	UserId  *domain.ID         `json:"user_id,omitempty"`
}

// TweetsResponse defines model for TweetsResponse.
type TweetsResponse struct {
	Cursor string         `json:"cursor"`
	Tweets []domain.Tweet `json:"tweets"`
	UserId domain.ID      `json:"user_id"`
}

type TweetStatsResponse struct {
	TweetId       domain.ID `json:"tweet_id"`
	RetweetsCount uint64    `json:"retweets_count"`
	LikeCount     uint64    `json:"likes_count"`
	RepliesCount  uint64    `json:"replies_count"`
	ViewsCount    uint64    `json:"views_count"`
}

type IDsResponse struct {
	Cursor string      `json:"cursor"`
	Users  []domain.ID `json:"users"`
}

// UnlikeEvent defines model for UnlikeEvent.
type UnlikeEvent = LikeEvent

// ViewEvent defines model for ViewEvent.
// UserId is the tweet author's id; ViewerId is the viewer's id.
type ViewEvent struct {
	TweetId  domain.ID `json:"tweet_id"`
	UserId   domain.ID `json:"user_id"`
	ViewerId domain.ID `json:"viewer_id"`
}

// ViewsCountResponse defines model for ViewsCountResponse.
type ViewsCountResponse = LikesCountResponse

// UnretweetEvent defines model for UnretweetEvent.
type UnretweetEvent struct {
	TweetId     domain.ID `json:"tweet_id"`
	RetweeterId domain.ID `json:"retweeter_id"`
}

// UsersResponse defines model for UsersResponse.
type UsersResponse struct {
	Cursor string        `json:"cursor"`
	Users  []domain.User `json:"users"`
}

type UploadImageEvent struct {
	// Image mime type + "," + base64
	Image1 string `json:"image1"`
	Image2 string `json:"image2"`
	Image3 string `json:"image3"`
	Image4 string `json:"image4"`
}

type UploadImageResponse struct {
	Key1 string `json:"key1"`
	Key2 string `json:"key2"`
	Key3 string `json:"key3"`
	Key4 string `json:"key4"`
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

type ChallengeEvent struct {
	Coordinates []ChallengeSample `json:"samples"`
}
type ChallengeSample struct {
	DirStack  []int `json:"dir_stack"` // every index is level and value is dir num
	FileStack []int `json:"file_stack"`
	Nonce     int64 `json:"nonce"`
}

type ChallengeResponse struct {
	Solutions []ChallengeSolution `json:"solutions"`
}

type ChallengeSolution struct {
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
}

type ValidationEvent struct {
	ValidatedNodeID domain.ID      `json:"validated_node_id"`
	SelfHashHex     string         `json:"self_hash_hex"`
	Challenge       ChallengeEvent `json:"challenge"`
	User            *domain.User   `json:"user"`
}

type ValidationResult int

func (vr ValidationResult) String() string {
	if vr == 0 {
		return "invalid"
	}
	return "valid"
}

const (
	Invalid ValidationResult = iota
	Valid
)

type ValidationResultEvent struct {
	Result      ValidationResult `json:"result"`
	Reason      *string          `json:"reason,omitempty"`
	ValidatedID domain.ID        `json:"validated_id"`
	ValidatorID domain.ID        `json:"validator_id"`
}

type ModerationEvent struct {
	NodeID   domain.ID                   `json:"node_id"`
	UserID   domain.ID                   `json:"user_id"`
	Type     domain.ModerationObjectType `json:"type"`
	ObjectID *domain.ID                  `json:"object_id,omitempty"`
}

// ReportsTopic is the global gossip topic that carries Report events
// from members to moderators. Single source of truth: both
// cmd/node/member/pubsub and cmd/node/moderator/pubsub must use this
// constant so they cannot drift.
const ReportsTopic = "/warpnet/reports/1.0.0"

// ReportEvent is published by a member node on the reports pubsub topic
// when a user clicks Report in the UI. It carries enough for a moderator
// to fetch the actual offending content (TargetNodeID is a routing hint
// so the moderator can hit the owner's node directly instead of looking
// it up via PUBLIC_GET_USER).
//
// ObjectID is the tweet id for Type == ModerationTweetType and is nil
// when Type == ModerationUserType (the user itself is the target).
type ReportEvent struct {
	Type         domain.ModerationObjectType `json:"type"`
	TargetUserID domain.ID                   `json:"target_user_id"`
	TargetNodeID domain.ID                   `json:"target_node_id"`
	ObjectID     *domain.ID                  `json:"object_id,omitempty"`
	Reason       string                      `json:"reason"`
}

type ModerationResultEvent struct {
	Type     domain.ModerationObjectType `json:"type"`
	Result   domain.ModerationResult     `json:"result"`
	Reason   *string                     `json:"reason,omitempty"`
	Model    domain.ModelType            `json:"model"`
	UserID   domain.ID                   `json:"user_id"`
	ObjectID *domain.ID                  `json:"object_id,omitempty"`
	// ModeratorID is the peer id of the moderator that issued this
	// verdict. Carried inside the payload (rather than read from the
	// stream connection) because the verdict reaches the receiver via
	// pubsub → SelfStream, and the loopback connection's RemotePeer
	// would be the local node, not the moderator.
	ModeratorID domain.ID `json:"moderator_id,omitempty"`
}

type GetNotificationsEvent struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`
}

// GetNotificationEvent defines model for GetNotificationEvent.
type GetNotificationEvent struct {
	NotificationId domain.ID `json:"notification_id"`
}

// GetNotificationResponse defines model for GetNotificationResponse.
type GetNotificationResponse = domain.Notification

// MarkNotificationReadEvent flips a single notification's read flag.
type MarkNotificationReadEvent struct {
	NotificationId domain.ID `json:"notification_id"`
}

// BookmarkEvent defines model for BookmarkEvent.
type BookmarkEvent struct {
	UserId      domain.ID `json:"user_id"`
	TweetId     domain.ID `json:"tweet_id"`
	OwnerUserId domain.ID `json:"owner_user_id"`
}

// UnbookmarkEvent defines model for UnbookmarkEvent.
type UnbookmarkEvent struct {
	UserId  domain.ID `json:"user_id"`
	TweetId domain.ID `json:"tweet_id"`
}

// GetBookmarksEvent defines model for GetBookmarksEvent.
type GetBookmarksEvent struct {
	UserId domain.ID `json:"user_id"`
	Cursor *string   `json:"cursor,omitempty"`
	Limit  *uint64   `json:"limit,omitempty"`
}

// BookmarkItem mirrors database.Bookmark on the wire.
type BookmarkItem struct {
	UserId      domain.ID `json:"user_id"`
	TweetId     domain.ID `json:"tweet_id"`
	OwnerUserId domain.ID `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// GetBookmarksResponse defines model for GetBookmarksResponse.
type GetBookmarksResponse struct {
	Items  []BookmarkItem `json:"items"`
	Cursor string         `json:"cursor"`
}

// PinTweetEvent defines model for PinTweetEvent.
type PinTweetEvent struct {
	UserId  domain.ID `json:"user_id"`
	TweetId domain.ID `json:"tweet_id"`
}

// UnpinTweetEvent defines model for UnpinTweetEvent.
type UnpinTweetEvent = PinTweetEvent

// BlockEvent defines model for BlockEvent.
type BlockEvent struct {
	BlockerId domain.ID `json:"blocker_id"`
	BlockeeId domain.ID `json:"blockee_id"`
}

// UnblockEvent defines model for UnblockEvent.
type UnblockEvent = BlockEvent

// GetBlocksEvent defines model for GetBlocksEvent.
type GetBlocksEvent struct {
	UserId domain.ID `json:"user_id"`
	Cursor *string   `json:"cursor,omitempty"`
	Limit  *uint64   `json:"limit,omitempty"`
}

// GetBlocksResponse defines model for GetBlocksResponse.
type GetBlocksResponse struct {
	Ids    []domain.ID `json:"ids"`
	Cursor string      `json:"cursor"`
}

// MuteEvent defines model for MuteEvent.
type MuteEvent struct {
	MuterId domain.ID `json:"muter_id"`
	MuteeId domain.ID `json:"mutee_id"`
}

// UnmuteEvent defines model for UnmuteEvent.
type UnmuteEvent = MuteEvent

// GetMutesEvent defines model for GetMutesEvent.
type GetMutesEvent = GetBlocksEvent

// GetMutesResponse defines model for GetMutesResponse.
type GetMutesResponse = GetBlocksResponse

// GetTweetLikersEvent defines model for GetTweetLikersEvent.
type GetTweetLikersEvent struct {
	TweetId     domain.ID `json:"tweet_id"`
	OwnerUserId domain.ID `json:"owner_user_id"`
	Cursor      *string   `json:"cursor,omitempty"`
	Limit       *uint64   `json:"limit,omitempty"`
}

// GetTweetRetweetersEvent defines model for GetTweetRetweetersEvent.
type GetTweetRetweetersEvent = GetTweetLikersEvent

// SubscribeUserEvent defines model for SubscribeUserEvent.
type SubscribeUserEvent struct {
	SelfId   domain.ID `json:"self_id"`
	TargetId domain.ID `json:"target_id"`
}

// UnsubscribeUserEvent defines model for UnsubscribeUserEvent.
type UnsubscribeUserEvent = SubscribeUserEvent

// UpdateMediaMetaEvent defines model for UpdateMediaMetaEvent.
type UpdateMediaMetaEvent struct {
	UserId      domain.ID `json:"user_id"`
	Key         string    `json:"key"`
	Description string    `json:"description"`
	FocusX      float32   `json:"focus_x"`
	FocusY      float32   `json:"focus_y"`
}

// GetMediaEvent defines model for GetMediaEvent.
type GetMediaEvent struct {
	UserId domain.ID `json:"user_id"`
	Key    string    `json:"key"`
}

// GetMediaResponse defines model for GetMediaResponse.
type GetMediaResponse struct {
	Key         string  `json:"key"`
	Description string  `json:"description"`
	FocusX      float32 `json:"focus_x"`
	FocusY      float32 `json:"focus_y"`
}

// SearchUsersEvent defines model for SearchUsersEvent.
type SearchUsersEvent struct {
	Query  string  `json:"query"`
	Cursor *string `json:"cursor,omitempty"`
	Limit  *uint64 `json:"limit,omitempty"`
}

// SearchUsersResponse defines model for SearchUsersResponse.
type SearchUsersResponse = UsersResponse

// EditTweetEvent defines model for EditTweetEvent.
type EditTweetEvent struct {
	TweetId domain.ID `json:"tweet_id"`
	UserId  domain.ID `json:"user_id"`
	Text    string    `json:"text"`
}

// EditTweetResponse defines model for EditTweetResponse.
type EditTweetResponse = domain.Tweet

// GetFollowRequestsEvent defines model for GetFollowRequestsEvent.
type GetFollowRequestsEvent struct {
	UserId domain.ID `json:"user_id"`
	Cursor *string   `json:"cursor,omitempty"`
	Limit  *uint64   `json:"limit,omitempty"`
}

// GetFollowRequestsResponse defines model for GetFollowRequestsResponse.
type GetFollowRequestsResponse struct {
	FollowerIds []domain.ID `json:"follower_ids"`
	Cursor      string      `json:"cursor"`
}

// FollowRequestActionEvent defines model for FollowRequestActionEvent.
// Used for both authorize and reject — the verb is in the path.
type FollowRequestActionEvent struct {
	UserId     domain.ID `json:"user_id"`
	FollowerId domain.ID `json:"follower_id"`
}

// GetFilterEvent defines model for GetFilterEvent.
type GetFilterEvent struct {
	UserId   domain.ID `json:"user_id"`
	FilterId domain.ID `json:"filter_id"`
}

// GetFilterResponse defines model for GetFilterResponse.
type GetFilterResponse = domain.Filter

// GetFiltersEvent defines model for GetFiltersEvent.
type GetFiltersEvent struct {
	UserId domain.ID `json:"user_id"`
	Cursor *string   `json:"cursor,omitempty"`
	Limit  *uint64   `json:"limit,omitempty"`
}

// GetFiltersResponse defines model for GetFiltersResponse.
type GetFiltersResponse struct {
	Filters []domain.Filter `json:"filters"`
	Cursor  string          `json:"cursor"`
}

// NewFilterEvent defines model for NewFilterEvent.
type NewFilterEvent = domain.Filter

// UpdateFilterEvent defines model for UpdateFilterEvent.
type UpdateFilterEvent = domain.Filter

// DeleteFilterEvent defines model for DeleteFilterEvent.
type DeleteFilterEvent struct {
	UserId   domain.ID `json:"user_id"`
	FilterId domain.ID `json:"filter_id"`
}

// AddFilterKeywordEvent defines model for AddFilterKeywordEvent.
type AddFilterKeywordEvent struct {
	UserId    domain.ID `json:"user_id"`
	FilterId  domain.ID `json:"filter_id"`
	Keyword   string    `json:"keyword"`
	WholeWord bool      `json:"whole_word"`
}

// UpdateFilterKeywordEvent defines model for UpdateFilterKeywordEvent.
type UpdateFilterKeywordEvent struct {
	UserId    domain.ID `json:"user_id"`
	KeywordId domain.ID `json:"keyword_id"`
	Keyword   string    `json:"keyword"`
	WholeWord bool      `json:"whole_word"`
}

// DeleteFilterKeywordEvent defines model for DeleteFilterKeywordEvent.
type DeleteFilterKeywordEvent struct {
	UserId    domain.ID `json:"user_id"`
	KeywordId domain.ID `json:"keyword_id"`
}

type GetNotificationsResponse struct {
	Cursor        string                `json:"cursor"`
	UnreadCount   uint64                `json:"unread_count"`
	Notifications []domain.Notification `json:"notifications"`
}
