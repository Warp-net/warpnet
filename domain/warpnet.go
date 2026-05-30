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
	"github.com/Warp-net/warpnet/core/warpnet"
	"time"

	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type ID = string

// QRByteModeCapacity is the maximum payload (bytes) that fits in a QR code at
// version 40 with error correction level 'L' in byte mode. The desktop UI
// renders the AuthNodeInfo envelope as a pairing QR; JSON payloads larger
// than this cannot be encoded and the QR modal renders blank.
const QRByteModeCapacity = 2953

// AuthNodeInfo defines model for AuthNodeInfo.
type AuthNodeInfo struct {
	UserId         string   `json:"user_id"`
	Token          string   `json:"token"`
	PSK            string   `json:"psk"`
	ID             string   `json:"node_id"`
	Addresses      []string `json:"addresses"`
	BootstrapPeers []string `json:"bootstrap_peers"`
	Network        string   `json:"network,omitempty"`
}

// LogSize logs the JSON-encoded size of the AuthNodeInfo and warns when it
// exceeds QRByteModeCapacity, surfacing pairing-QR overflow in node logs
// before users hit a blank QR modal.
func (a AuthNodeInfo) LogSize() {
	data, err := json.Marshal(a)
	if err != nil {
		log.Warnf("auth node info: marshal for size check: %v", err)
		return
	}
	size := len(data)
	log.Infof("auth node info size: %d bytes", size)
	if size > QRByteModeCapacity {
		log.Warnf(
			"auth node info size (%d bytes) exceeds QR byte-mode capacity (%d bytes); pairing QR generation will fail",
			size, QRByteModeCapacity,
		)
	}
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
	PSK   string `json:"psk"`
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

type Device struct {
	ID         ID                 `json:"id"`
	CreatedAt  time.Time          `json:"created_at"`
	NodeId     warpnet.WarpPeerID `json:"node_id"`
	Token      string             `json:"token"`
	Platform   string             `json:"platform"`
	LastActive time.Time          `json:"last_active"`
}

// ReplyNode defines model for ReplyNode.
type ReplyNode struct {
	Children []ReplyNode `json:"children"`
	Reply    Tweet       `json:"reply"`
}

const RetweetPrefix = "RT:"

// Tweet defines model for Tweet.
//
// ParentId is the parent TWEET id (not a user id) for replies; nil for
// top-level tweets and for replies that hang directly off RootId.
type Tweet struct {
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Id        string     `json:"id"`
	ParentId  *string    `json:"parent_id,omitempty"`

	// RetweetedBy retweeted by user id
	RetweetedBy   *string          `json:"retweeted_by,omitempty"`
	RootId        string           `json:"root_id"`
	Text          string           `json:"text"`
	UserId        string           `json:"user_id"`
	Username      string           `json:"username"`
	ImageKeys     []string         `json:"image_keys,omitempty"`
	Network       string           `json:"network"`
	Moderation    *TweetModeration `json:"moderation,omitempty"`
	Pinned        bool             `json:"pinned,omitempty"`
	QuotedTweetId *string          `json:"quoted_tweet_id,omitempty"`
	QuotedUserId  *string          `json:"quoted_user_id,omitempty"`
}

func (t *Tweet) IsModerated() bool {
	return t.Moderation != nil
}

type ModelType string

const LLAMA2 ModelType = "llama2"

// TweetEdit is an immutable revision row. Tweets are mutated in-place
// (Tweet.Text rewritten) and a TweetEdit is appended for each edit so
// the client can show "edited at X" history. EditedAt = the moment the
// edit was committed; the original tweet's CreatedAt stays untouched.
type TweetEdit struct {
	Id              string    `json:"id"`
	OriginalTweetId string    `json:"original_tweet_id"`
	UserId          string    `json:"user_id"`
	Text            string    `json:"text"`
	EditedAt        time.Time `json:"edited_at"`
}

type TweetModeration struct {
	ModeratorID ID               `json:"moderator_id"`
	Model       ModelType        `json:"model"`
	IsOk        ModerationResult `json:"is_ok"`
	Reason      *string          `json:"reason"`
	TimeAt      time.Time        `json:"time_at"`
}

// Filter is a per-user keyword/regex filter. Filters apply at timeline-read
// time; they're never replicated to peers. Keywords are stored as an
// embedded slice (Mastodon models them as a sub-resource with their own
// ids — we keep the same shape on the wire but the storage is one record
// per filter).
// FilterContext is where a content filter applies. Closed enum — only
// these values are accepted on the wire. Note: there is no "account"
// context in Warpnet — Warpnet has users and nodes, not accounts.
// Warpnet has no "public" context either — every tweet is public by
// default, so a filter on a "public" timeline would be redundant.
type FilterContext string

const (
	FilterContextHome          FilterContext = "home"
	FilterContextNotifications FilterContext = "notifications"
	FilterContextThread        FilterContext = "thread"
)

// FilterAction is what happens to a tweet that matches a filter.
type FilterAction string

const (
	FilterActionWarn FilterAction = "warn"
	FilterActionHide FilterAction = "hide"
)

type Filter struct {
	Id        string          `json:"id"`
	UserId    string          `json:"user_id"`
	Title     string          `json:"title"`
	Context   []FilterContext `json:"context"`
	Action    FilterAction    `json:"action"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	Keywords  []FilterKeyword `json:"keywords"`
}

// FilterKeyword is a single match rule on a filter.
type FilterKeyword struct {
	Id        string `json:"id"`
	Keyword   string `json:"keyword"`
	WholeWord bool   `json:"whole_word"`
}

// User defines model for User.
type User struct {
	// Avatar mime type + "," + base64
	AvatarKey string `json:"avatar_key,omitempty"`

	// BackgroundImage mime type + "," + base64
	BackgroundImageKey string     `json:"background_image_key"`
	Bio                string     `json:"bio"`
	Birthdate          string     `json:"birthdate"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
	FollowingsCount    int64      `json:"followings_count"`
	FollowersCount     int64      `json:"followers_count"`
	Id                 string     `json:"id"`
	IsOffline          bool       `json:"isOffline"`
	NodeId             string     `json:"node_id"`
	Network            string     `json:"network"`
	// Role mirrors NodeInfo.Role: "" for a regular user, "business" for a
	// business account. Stamped from the node's NodeInfo when the user is
	// cached (discovery) so clients can badge business accounts.
	Role          string            `json:"role,omitempty"`
	RoundTripTime int64             `json:"rtt"`
	TweetsCount   int64             `json:"tweets_count"`
	Username      string            `json:"username"`
	Website       *string           `json:"website,omitempty"`
	Moderation    *UserModeration   `json:"moderation"`
	Metadata      map[string]string `json:"metadata"`
	// Locked is the "manually-approve followers" flag. When true, an
	// inbound follow lands in the follow-request queue instead of being
	// accepted automatically.
	Locked bool `json:"locked,omitempty"`
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
