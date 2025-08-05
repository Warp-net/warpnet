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
	"github.com/Warp-net/warpnet/cmd/node/bootstrap/pubsub"
	"github.com/Warp-net/warpnet/core/consensus"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"io"
)

type DiscoveryHandler interface {
	DefaultDiscoveryHandler(peerInfo warpnet.WarpAddrInfo)
	Run(n discovery.DiscoveryInfoStorer) error
	Close()
}

type PubSubProvider interface {
	Run(m pubsub.PubsubServerNodeConnector)
	Close() error
	PublishValidationRequest(bt []byte) (err error)
	GetConsensusTopicSubscribers() []warpnet.WarpAddrInfo
	OwnerID() string
}

type DistributedHashTableCloser interface {
	Close()
}

type ProviderCloser interface {
	io.Closer
}

type ConsensusServicer interface {
	Start(streamer consensus.ConsensusStreamer) (err error)
	Close()
	Validate(ev event.ValidationEvent) error
	ValidationResult(ev event.ValidationResultEvent) error
}
