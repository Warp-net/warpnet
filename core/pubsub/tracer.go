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

package pubsub

import (
	"github.com/Warp-net/warpnet/core/metrics"
	"github.com/Warp-net/warpnet/core/warpnet"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// metricsTracer counts the drops gossipsub otherwise performs silently.
// UndeliverableMessage is the one that matters most: it fires exactly when the
// consumer of Subscribe is not reading fast enough, which is the listener.
//
// Tracers are invoked synchronously by pubsub, so every method here stays a
// counter bump and nothing else.
type metricsTracer struct{}

func (metricsTracer) UndeliverableMessage(*pubsub.Message) { metrics.PubSubUndeliverable.Inc() }
func (metricsTracer) DropRPC(*pubsub.RPC, warpnet.WarpPeerID) {
	metrics.PubSubDropRPC.Inc()
}
func (metricsTracer) DuplicateMessage(*pubsub.Message) { metrics.PubSubDuplicate.Inc() }
func (metricsTracer) ThrottlePeer(warpnet.WarpPeerID)   { metrics.PubSubThrottled.Inc() }
func (metricsTracer) DeliverMessage(*pubsub.Message)    { metrics.PubSubDelivered.Inc() }

func (metricsTracer) OnNewOutboundStream(warpnet.WarpPeerID, warpnet.WarpProtocolID) {}
func (metricsTracer) OnClosedOutboundStream(warpnet.WarpPeerID)                      {}
func (metricsTracer) Join(string)                                                    {}
func (metricsTracer) Leave(string)                                                   {}
func (metricsTracer) Graft(warpnet.WarpPeerID, string)                               {}
func (metricsTracer) Prune(warpnet.WarpPeerID, string)                               {}
func (metricsTracer) ValidateMessage(*pubsub.Message)                                {}
func (metricsTracer) RejectMessage(*pubsub.Message, string)                          {}
func (metricsTracer) RecvRPC(*pubsub.RPC)                                            {}
func (metricsTracer) SendRPC(*pubsub.RPC, warpnet.WarpPeerID)                        {}
