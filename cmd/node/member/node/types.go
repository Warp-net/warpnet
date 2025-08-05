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
	"github.com/Warp-net/warpnet/core/consensus"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/mdns"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"io"
	"time"
)

type DiscoveryHandler interface {
	HandlePeerFound(pi warpnet.WarpAddrInfo)
	Run(n discovery.DiscoveryInfoStorer) error
	Close()
}

type MDNSStarterCloser interface {
	Start(n mdns.NodeConnector)
	Close()
}

type PubSubProvider interface {
	SubscribeUserUpdate(userId string) (err error)
	PublishModerationRequest(bt []byte) (err error)
	UnsubscribeUserUpdate(userId string) (err error)
	Run(m pubsub.PubsubServerNodeConnector)
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
	Close() error
}

type UserFetcher interface {
	Get(userId string) (user domain.User, err error)
	Update(userId string, newUser domain.User) (domain.User, error)
	GetByNodeID(nodeID string) (user domain.User, err error)
}

type DistributedHashTableCloser interface {
	Close()
}

type NodeProvider interface {
	io.Closer
	GetSelfHashes() (map[string]struct{}, error)
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
	Update(userId string, newUser domain.User) (updatedUser domain.User, err error)
	GetBatch(userIds ...string) (users []domain.User, err error)
	CreateWithTTL(user domain.User, ttl time.Duration) (domain.User, error)
	WhoToFollow(limit *uint64, cursor *string) ([]domain.User, string, error)
}

type ClientNodeStreamer interface {
	ClientStream(nodeId string, path string, data any) (_ []byte, err error)
	IsRunning() bool
}

type FollowStorer interface {
	GetFollowersCount(userId string) (uint64, error)
	GetFolloweesCount(userId string) (uint64, error)
	Follow(fromUserId, toUserId string, event domain.Following) error
	Unfollow(fromUserId, toUserId string) error
	GetFollowers(userId string, limit *uint64, cursor *string) ([]domain.Following, string, error)
	GetFollowees(userId string, limit *uint64, cursor *string) ([]domain.Following, string, error)
}

type Storer interface {
	NewTxn() (local.WarpTransactioner, error)
	Get(key local.DatabaseKey) ([]byte, error)
	GetExpiration(key local.DatabaseKey) (uint64, error)
	GetSize(key local.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local.WarpDB
	SetWithTTL(key local.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local.DatabaseKey, value []byte) error
	Delete(key local.DatabaseKey) error
	Path() string
	Stats() map[string]string
	IsFirstRun() bool
}

type ConsensusServicer interface {
	Start(streamer consensus.ConsensusStreamer) (err error)
	Close()
	AskValidation(data event.ValidationEvent)
	Validate(ev event.ValidationEvent) error
	ValidationResult(ev event.ValidationResultEvent) error
}

type PseudoStreamer interface {
	ID() warpnet.WarpPeerID
	IsMastodonID(id warpnet.WarpPeerID) bool
	Addrs() []warpnet.WarpAddress
	Route(r stream.WarpRoute, data any) (_ []byte, err error)
}
