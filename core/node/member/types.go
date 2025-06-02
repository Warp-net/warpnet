/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU General Public License for more details.

 You should have received a copy of the GNU General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package member

import (
	"github.com/Warp-net/warpnet/core/consensus"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/mdns"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/storage"
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
	UnsubscribeUserUpdate(userId string) (err error)
	Run(m pubsub.PubsubServerNodeConnector, clientNode pubsub.PubsubClientNodeStreamer)
	PublishOwnerUpdate(ownerId string, msg event.Message) (err error)
	Close() error
}

type UserFetcher interface {
	Get(userId string) (user domain.User, err error)
	Update(userId string, newUser domain.User) (domain.User, error)
	GetByNodeID(nodeID string) (user domain.User, err error)
}

type ConsensusProvider interface {
	Start(node consensus.NodeTransporter) (err error)
	LeaderID() warpnet.WarpPeerID
	CommitState(newState consensus.KVState) (_ *consensus.KVState, err error)
	Shutdown()
	AskUserValidation(user domain.User) error
	AskSelfHashValidation(hashes map[string]struct{}) error
	Stats() map[string]string
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
	ValidateUser(k, v string) error
	GetByNodeID(nodeID string) (user domain.User, err error)
	Get(userId string) (user domain.User, err error)
	List(network string, limit *uint64, cursor *string) ([]domain.User, string, error)
	Update(userId string, newUser domain.User) (updatedUser domain.User, err error)
	GetBatch(userIds ...string) (users []domain.User, err error)
	GetOtherNetworkUser(network, userId string) (user domain.User, err error)
}

type ClientNodeStreamer interface {
	ClientStream(nodeId string, path string, data any) (_ []byte, err error)
}

type ConsensusStorer interface {
	Reset() error
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
	NewTxn() (storage.WarpTransactioner, error)
	Get(key storage.DatabaseKey) ([]byte, error)
	GetExpiration(key storage.DatabaseKey) (uint64, error)
	GetSize(key storage.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *storage.WarpDB
	SetWithTTL(key storage.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key storage.DatabaseKey, value []byte) error
	Delete(key storage.DatabaseKey) error
	Path() string
	Stats() map[string]string
	IsFirstRun() bool
}
