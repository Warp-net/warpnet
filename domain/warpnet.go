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
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type ID = string

var ErrTweetSignatureInvalid = errors.New("tweet signature invalid")

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

type ChatMessage struct {
	ChatId     string    `json:"chat_id"`
	CreatedAt  time.Time `json:"created_at"`
	Id         string    `json:"id"`
	ReceiverId string    `json:"receiver_id"`
	SenderId   string    `json:"sender_id"`
	Text       string    `json:"text"`

	// ImageKeys holds the video's still frame as its only entry when VideoKey
	// is set, the same convention tweets use with the first of their image keys.
	ImageKeys []string `json:"image_keys,omitempty"`
	VideoKey  *string  `json:"video_key,omitempty"`

	Status string `json:"status,omitempty"`
}

func (m ChatMessage) IsEmpty() bool {
	return m.ChatId == "" && m.Id == "" && m.SenderId == "" && m.ReceiverId == "" &&
		m.Text == "" && len(m.ImageKeys) == 0 && m.VideoKey == nil &&
		m.Status == "" && m.CreatedAt.IsZero()
}

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

type Bookmark struct {
	UserId      string    `json:"user_id"`
	TweetId     string    `json:"tweet_id"`
	OwnerUserId string    `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type Reaction struct {
	TweetId string `json:"tweet_id"`
	UserId  string `json:"user_id"`
	Emoji   string `json:"emoji,omitempty"`
}

type ReactedTweet struct {
	UserId      string    `json:"user_id"`
	TweetId     string    `json:"tweet_id"`
	OwnerUserId string    `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type Owner struct {
	CreatedAt       time.Time `json:"created_at"`
	NodeId          string    `json:"node_id"`
	UserId          string    `json:"user_id"`
	RedundantUserID string    `json:"id"`
	Username        string    `json:"username"`
}

const RetweetPrefix = "RT:"

type Tweet struct {
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	Id           string     `json:"id"`
	ParentId     *string    `json:"parent_id,omitempty"`
	ParentUserId *string    `json:"parent_user_id,omitempty"`

	// RetweetedBy retweeted by user id
	RetweetedBy   *string          `json:"retweeted_by,omitempty"`
	RootId        string           `json:"root_id"`
	Text          string           `json:"text"`
	UserId        string           `json:"user_id"`
	Username      string           `json:"username"`
	ImageKeys     []string         `json:"image_keys,omitempty"`
	VideoKey      *string          `json:"video_key,omitempty"`
	Network       string           `json:"network"`
	Moderation    *TweetModeration `json:"moderation,omitempty"`
	Pinned        bool             `json:"pinned,omitempty"`
	QuotedTweetId *string          `json:"quoted_tweet_id,omitempty"`
	QuotedUserId  *string          `json:"quoted_user_id,omitempty"`
	Poll          *Poll            `json:"poll,omitempty"`
	// Signature is base64(ed25519) over signingBytes, produced with the
	// author's node key. A tweet reaches a follower through gossip, whose
	// loopback stream reports the local node as the peer, so authorship can
	// only be established from the payload itself.
	Signature string `json:"signature,omitempty"`
}

// signingBytes returns the canonical bytes the tweet signature covers, in the
// same length-prefixed form as ModerationVerdictEvent so signer and verifier
// agree byte-for-byte even across versions that add unrelated fields.
func (t Tweet) signingBytes() []byte {
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	parts := []string{
		t.Id,
		t.UserId,
		t.Username,
		t.Text,
		t.RootId,
		deref(t.ParentId),
		deref(t.ParentUserId),
		deref(t.QuotedTweetId),
		deref(t.QuotedUserId),
		deref(t.VideoKey),
		strconv.FormatInt(t.CreatedAt.UnixNano(), 10),
	}
	parts = append(parts, t.ImageKeys...)
	if t.Poll != nil {
		parts = append(parts, strconv.FormatInt(t.Poll.ExpiresAt.UnixNano(), 10))
		parts = append(parts, t.Poll.Options...)
	}

	buf := make([]byte, 0, 256)
	for _, p := range parts {
		buf = append(buf, strconv.Itoa(len(p))...)
		buf = append(buf, ':')
		buf = append(buf, p...)
	}
	return buf
}

// Signed returns a signed copy of the tweet. It returns the tweet rather than
// mutating one in place so a caller cannot ship an unsigned tweet by dropping
// the result on the floor.
func (t Tweet) Signed(privKey ed25519.PrivateKey) Tweet {
	if len(privKey) == 0 {
		return t
	}
	t.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privKey, t.signingBytes()))
	return t
}

// Verify checks the tweet signature against pubKey, the mirror of Signed.
func (t Tweet) Verify(pubKey ed25519.PublicKey) error {
	if len(pubKey) != ed25519.PublicKeySize {
		return ErrTweetSignatureInvalid
	}
	sig, err := base64.StdEncoding.DecodeString(t.Signature)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pubKey, t.signingBytes(), sig) {
		return ErrTweetSignatureInvalid
	}
	return nil
}

func (t *Tweet) IsReply() bool {
	return t.ParentId != nil && *t.ParentId != ""
}

func (t *Tweet) IsModerated() bool {
	return t.Moderation != nil
}

type Poll struct {
	Options   []string  `json:"options"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsClosed reports whether the poll has stopped accepting votes.
func (p *Poll) IsClosed() bool {
	return p != nil && !p.ExpiresAt.IsZero() && time.Now().After(p.ExpiresAt)
}

type ModelType string

const LLAMAGuard3 ModelType = "LlamaGuard3"

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
	BackgroundImageKey string            `json:"background_image_key"`
	Bio                string            `json:"bio"`
	Birthdate          string            `json:"birthdate"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          *time.Time        `json:"updated_at,omitempty"`
	FollowingsCount    int64             `json:"followings_count"`
	FollowersCount     int64             `json:"followers_count"`
	Id                 string            `json:"id"`
	IsOffline          bool              `json:"isOffline"`
	LastSeen           *time.Time        `json:"last_seen,omitempty"`
	NodeId             string            `json:"node_id"`
	Network            string            `json:"network"`
	RoundTripTime      int64             `json:"rtt"`
	TweetsCount        int64             `json:"tweets_count"`
	Username           string            `json:"username"`
	Website            *string           `json:"website,omitempty"`
	Moderation         *UserModeration   `json:"moderation"`
	Metadata           map[string]string `json:"metadata"`
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
	NotificationReactionType   NotificationType = "reaction"
	NotificationMentionType    NotificationType = "mention"
	NotificationReplyType      NotificationType = "reply"
	NotificationMessageType    NotificationType = "message"
	NotificationNewUserType    NotificationType = "new_user"
)

type Notification struct {
	Type        NotificationType `json:"type"`
	Id          string           `json:"id"`
	Text        string           `json:"text"`
	RecepientId string           `json:"user_id"`
	ActorId     string           `json:"actor_id,omitempty"`
	TweetId     string           `json:"tweet_id,omitempty"`
	IsRead      bool             `json:"is_read"`
	CreatedAt   time.Time        `json:"created_at"`
}

type NotificationSettings struct {
	EmailEnabled bool   `json:"email_enabled"`
	Recipient    string `json:"recipient"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`
	// Types is the per-notification-type email toggle. A type absent from
	// the map (or false) means "do not email for this type".
	Types map[NotificationType]bool `json:"types"`
}

type GatewaySettings struct {
	NodeID string `json:"node_id"`
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

type (
	Base64Image string
	ImageKey    string
	Base64Video string
	VideoKey    string
)

type Alias struct {
	ID         ID        `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	NodeId     string    `json:"node_id"`
	Token      string    `json:"token"`
	Platform   string    `json:"platform"`
	LastActive time.Time `json:"last_active"`
}
