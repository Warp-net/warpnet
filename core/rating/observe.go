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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// Some behaviour is an offence only in numbers: one reconnection is
// ordinary, a burst of them is not.
const (
	flapWindow    = time.Minute
	flapThreshold = 4

	writeFloodWindow    = 5 * time.Minute
	writeFloodThreshold = 20

	discoveryWindow    = 10 * time.Minute
	discoveryThreshold = 32

	burstPeers = 1024
)

// burst counts one observation per peer inside a sliding window.
type burst struct {
	mu        sync.Mutex
	counts    *expirable.LRU[string, int]
	threshold int
}

func newBurst(window time.Duration, threshold int) *burst {
	return &burst{
		counts:    expirable.NewLRU[string, int](burstPeers, nil, window),
		threshold: threshold,
	}
}

// reached reports the count hitting the threshold, once per window.
func (b *burst) reached(peerID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	count, _ := b.counts.Get(peerID)
	count++
	b.counts.Add(peerID, count)
	return count == b.threshold
}

// Listen charges the peers the modules' fan-outs name. It returns at
// once; each listener runs until its channel closes or the engine does,
// and Close waits for them.
func (e *Engine) Listen(sources ...<-chan warpnet.PeerEvent) {
	if e == nil || e.ctx.Err() != nil {
		return
	}
	for _, events := range sources {
		if events == nil {
			continue
		}
		e.listeners.Go(func() { e.listen(events) })
	}
}

func (e *Engine) listen(events <-chan warpnet.PeerEvent) {
	for {
		select {
		case <-e.ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			e.observe(ev)
		}
	}
}

// observe turns one observation into the offences it is worth. A type
// this node does not charge for is not an error: it is a plain fact, or
// an axis its role cannot witness.
func (e *Engine) observe(ev warpnet.PeerEvent) {
	peerID := warpnet.FromStringToPeerID(ev.PeerID)
	if peerID == "" {
		return
	}

	switch ev.Type {
	case warpnet.PeerConnected:
		if e.flaps.reached(ev.PeerID) {
			e.record(peerID, KindConnectionFlap)
		}
	case warpnet.PeerDiscovered:
		if e.discoveries.reached(ev.PeerID) {
			e.record(peerID, KindDiscoveryFlood)
		}
	case warpnet.PeerRateLimited:
		e.record(peerID, KindRateLimitHit)
		if !stream.WarpRoute(ev.Route).IsGet() && e.writes.reached(ev.PeerID) {
			e.record(peerID, KindWriteFlood)
		}
	default:
		if kind, ok := ParseKind(string(ev.Type)); ok {
			e.record(peerID, kind)
		}
	}
}
