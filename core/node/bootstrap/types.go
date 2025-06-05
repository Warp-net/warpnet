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

package bootstrap

import (
	"github.com/Warp-net/warpnet/core/consensus"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"io"
)

type DiscoveryHandler interface {
	HandlePeerFound(pi warpnet.WarpAddrInfo)
	Run(n discovery.DiscoveryInfoStorer) error
	Close()
}

type PubSubProvider interface {
	Run(m pubsub.PubsubServerNodeConnector, clientNode pubsub.PubsubClientNodeStreamer)
	PublishOwnerUpdate(ownerId string, msg event.Message) (err error)
	Close() error
}

type ConsensusProvider interface {
	Start(node consensus.NodeTransporter) (err error)
	LeaderID() warpnet.WarpPeerID
	CommitState(newState consensus.KVState) (_ *consensus.KVState, err error)
	Shutdown()
	AskSelfHashValidation(selfHashHex string) error
}

type DistributedHashTableCloser interface {
	Close()
}

type ProviderCloser interface {
	io.Closer
}

type ClientNodeStreamer interface {
	ClientStream(nodeId string, path string, data any) (_ []byte, err error)
}
