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

package node

import (
	"context"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/member/pubsub"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/mdns"
	corePubsub "github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/peer"
)

type DiscoveryHandler interface {
	DiscoveryHandlerStream(pi warpnet.WarpAddrInfo)
	Run(n discovery.DiscoveryInfoStorer) error
	Close()
}

type MDNSStarterCloser interface {
	Start(n mdns.NodeConnector)
	Close()
}

type PubSubProvider interface {
	SubscribeUserUpdate(userId string) (err error)
	UnsubscribeUserUpdate(userId string) (err error)
	Run(m pubsub.PubsubServerNodeConnector)
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
	PublishReport(ev event.ReportEvent) error
	Close() error
	Gossip() *corePubsub.Gossip
}

type UserFetcher interface {
	Get(userId string) (user domain.User, err error)
	Update(userId string, newUser domain.User) (domain.User, error)
	GetByNodeID(nodeID string) (user domain.User, err error)
}

type DistributedHashTableCloser interface {
	FindProvidersAsync(ctx context.Context, key cid.Cid, count int) (ch <-chan peer.AddrInfo)
	BootstrapNodes() []warpnet.WarpAddrInfo
	Close()
}

type NodeProvider interface {
	datastore.Datastore
	BlocklistExponential(peerId string) error
	BlocklistPermanent(peerId string) error
	BlocklistRemove(peerId string) error
}

type StatsProvider interface {
	datastore.Datastore
}

type AuthProvider interface {
	GetOwner() domain.Owner
	SessionToken() string
}

type UserProvider interface {
	Create(user domain.User) (domain.User, error)
	GetByNodeID(nodeID string) (user domain.User, err error)
	Get(userId string) (user domain.User, err error)
	List(limit *uint64, cursor *string) ([]domain.User, string, error)
	Search(query string, limit *uint64, cursor *string) ([]domain.User, string, error)
	Update(userId string, newUser domain.User) (updatedUser domain.User, err error)
	GetBatch(userIds ...string) (users []domain.User, err error)
	CreateWithTTL(user domain.User, ttl time.Duration) (domain.User, error)
	WhoToFollow(limit *uint64, cursor *string) ([]domain.User, string, error)
}

type DeviceProvider interface {
	SetDevice(ownerNodeId string, device domain.Device) error
	GetDevices(ownerNodeId string) (devices []domain.Device, err error)
}

type ClientNodeStreamer interface {
	ClientStream(nodeId string, path string, data any) (_ []byte, err error)
	IsRunning() bool
}

type FollowStorer interface {
	GetFollowersCount(userId string) (uint64, error)
	GetFollowingsCount(userId string) (uint64, error)
	Follow(fromUserId, toUserId string) error
	Unfollow(fromUserId, toUserId string) error
	GetFollowers(userId string, limit *uint64, cursor *string) ([]domain.ID, string, error)
	GetFollowings(userId string, limit *uint64, cursor *string) ([]domain.ID, string, error)
	IsFollowing(ownerId, otherUserId string) bool
	IsFollower(ownerId, otherUserId string) bool
	AddFollowRequest(targetUserId, followerId string) error
	RemoveFollowRequest(targetUserId, followerId string) error
	ListFollowRequests(targetUserId string, limit *uint64, cursor *string) ([]domain.ID, string, error)
}

type Storer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	NewReadTxn() (local_store.WarpTransactioner, error)
	Get(key local_store.DatabaseKey) ([]byte, error)
	GetExpiration(key local_store.DatabaseKey) (uint64, error)
	GetSize(key local_store.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local_store.WarpDB
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local_store.DatabaseKey, value []byte) error
	Delete(key local_store.DatabaseKey) error
	Path() string
	Stats() map[string]string
	IsFirstRun() bool
}

type StatsStorer interface {
	Increment(key ds.Key) error
	Decrement(key ds.Key) error
	GetAggregatedStat(key ds.Key) (uint64, error)
	Close() error
}

// Collaborator interfaces for the repos memberRepos bundles: the node wires
// concrete repos in, handlers and tests can wire anything that fits.
type BlocksProvider interface {
	Block(blockerId string, blockeeId string) error
	List(blockerId string, limit *uint64, cursor *string) ([]string, string, error)
	Unblock(blockerId string, blockeeId string) error
}

type BookmarkProvider interface {
	Bookmark(userId string, tweetId string, ownerUserId string) error
	List(userId string, limit *uint64, cursor *string) ([]domain.Bookmark, string, error)
	Unbookmark(userId string, tweetId string) error
}

type ChatProvider interface {
	CreateChat(chatId *string, ownerId string, otherUserId string) (domain.Chat, error)
	CreateMessage(msg domain.ChatMessage) (domain.ChatMessage, error)
	DeleteChat(chatId string) error
	DeleteMessage(chatId string, id string) error
	GetChat(chatId string) (chat domain.Chat, err error)
	GetMessage(chatId string, id string) (domain.ChatMessage, error)
	GetUserChats(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error)
	ListMessages(chatId string, limit *uint64, cursor *string) ([]domain.ChatMessage, string, error)
}

type FilterProvider interface {
	AddKeyword(userId string, filterId string, kw domain.FilterKeyword) (domain.FilterKeyword, error)
	Create(userId string, f domain.Filter) (domain.Filter, error)
	Delete(userId string, filterId string) error
	DeleteKeyword(userId string, keywordId string) error
	Get(userId string, filterId string) (domain.Filter, error)
	List(userId string, limit *uint64, cursor *string) ([]domain.Filter, string, error)
	Update(userId string, f domain.Filter) (domain.Filter, error)
	UpdateKeyword(userId string, kw domain.FilterKeyword) (domain.FilterKeyword, error)
}

type LikesProvider interface {
	Like(tweetId string, userId string, isTransitive bool) (likesNum uint64, err error)
	Liked(userId string, limit *uint64, cursor *string) ([]domain.LikedTweet, string, error)
	Likers(tweetId string, limit *uint64, cursor *string) (likers []string, cur string, err error)
	LikesCount(tweetId string) (likesNum uint64, err error)
	RemoveLiked(userId string, tweetId string) error
	SetLiked(userId string, tweetId string, ownerUserId string) error
	Unlike(tweetId string, userId string, isTransitive bool) (likesNum uint64, err error)
}

type MediaProvider interface {
	GetImage(userId string, key string) (database.Base64Image, error)
	GetImageMeta(userId string, key string) (database.MediaMeta, error)
	GetVideo(userId string, key string) (database.Base64Video, error)
	SetForeignImageWithTTL(userId string, key string, img database.Base64Image) error
	SetForeignVideoWithTTL(userId string, key string, video database.Base64Video) error
	SetImage(userId string, img database.Base64Image) (_ database.ImageKey, err error)
	SetImageMeta(userId string, key string, meta database.MediaMeta) error
	SetVideo(userId string, video database.Base64Video) (_ database.VideoKey, err error)
}

type MutesProvider interface {
	List(muterId string, limit *uint64, cursor *string) ([]string, string, error)
	Mute(muterId string, muteeId string) error
	Unmute(muterId string, muteeId string) error
}

type NotificationProvider interface {
	Get(userId string, notificationId string) (domain.Notification, error)
	List(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error)
	MarkAllRead(userId string) error
	MarkRead(userId string, notificationId string) error
	ReverseList(userId string, cursor *string, limit *uint64) ([]domain.Notification, string, error)
	UnreadCount(userId string) (uint64, error)
}

type PollProvider interface {
	Results(tweetId string, optionsNum int) (votes []uint64, err error)
	Vote(tweetId string, userId string, option int, isTransitive bool) error
	Voted(tweetId string, userId string) (option int, ok bool, err error)
}

type SettingsProvider interface {
	GetGatewaySettings(userId string) (domain.GatewaySettings, error)
	GetNotificationSettings(userId string) (domain.NotificationSettings, error)
	SetGatewaySettings(userId string, s domain.GatewaySettings) error
	SetNotificationSettings(userId string, s domain.NotificationSettings) error
}

type SubscriptionProvider interface {
	Subscribe(selfId string, targetUserId string) error
	Unsubscribe(selfId string, targetUserId string) error
}

type TimelineProvider interface {
	AddTweetToTimeline(userId string, tweet domain.Tweet) error
	DeleteTweetFromTimeline(userID string, tweetID string) error
	GetTimeline(string, *uint64, *string) ([]domain.Tweet, string, error)
}

type TweetsProvider interface {
	AddReply(reply domain.Tweet, isTransitive bool) (domain.Tweet, error)
	AppendEdit(edit domain.TweetEdit) (domain.TweetEdit, error)
	Blocklist(tweetId string) error
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
	CreateWithTTL(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error)
	Delete(userID string, tweetID string) error
	DeleteReply(parentID string, replyID string, isTransitive bool) (domain.Tweet, error)
	Get(userID string, tweetID string) (tweet domain.Tweet, err error)
	GetReplies(parentID string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
	GetReply(parentID string, replyID string) (domain.Tweet, error)
	GetViewsCount(tweetId string) (uint64, error)
	IsBlocklisted(tweetId string) bool
	List(string, *uint64, *string) ([]domain.Tweet, string, error)
	NewRetweet(tweet domain.Tweet, isTransitive bool) (_ domain.Tweet, err error)
	Pin(userId string, tweetId string) (domain.Tweet, error)
	RecordView(tweetId string, viewerId string) (uint64, error)
	RepliesCount(tweetId string) (likesNum uint64, err error)
	Retweeters(tweetId string, limit *uint64, cursor *string) (_ []string, cur string, err error)
	RetweetsCount(tweetId string) (uint64, error)
	TweetsCount(userId string) (uint64, error)
	UnRetweet(retweetedByUserID string, tweetId string, isTransitive bool) error
	Unpin(userId string, tweetId string) (domain.Tweet, error)
	Update(tweet domain.Tweet) error
}
