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

package moderator

import (
	"context"
	"github.com/Warp-net/warpnet/core/consensus"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
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
	Run(m pubsub.PubsubServerNodeConnector)
	Close() error
	SubscribeModerationTopic() error
}

type DistributedHashTableCloser interface {
	Close()
}

type DistributedStorer interface {
	GetStream(ctx context.Context, id string) (io.ReadCloser, error)
	PutStream(ctx context.Context, reader io.ReadCloser) (id string, _ error)
	Close() error
}

type ProviderCloser interface {
	io.Closer
}

type Streamer interface {
	Send(peerAddr warpnet.WarpAddrInfo, r stream.WarpRoute, data []byte) ([]byte, error)
}

type ConsensusServicer interface {
	Start(streamer consensus.ConsensusStreamer) (err error)
	Close()
	AskValidation(data event.ValidationEvent) error
	Validate(ev event.ValidationEvent) error
	ValidationResult(ev event.ValidationResultEvent) error
}

type Moderator interface {
	Moderate(content string) (bool, string, error)
	Close()
}
