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

package crdt

import (
	"context"
	"sync"

	"github.com/Warp-net/warpnet/event"
)

// GossipPublisher interface for publishing to Gossip
type GossipPublisher interface {
	Publish(msg event.Message, topics ...string) error
}

// GossipBroadcaster adapts Gossip to CRDT Broadcaster interface
type GossipBroadcaster struct {
	gossip    GossipPublisher
	topic     string
	dataChan  chan []byte
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
}

// NewGossipBroadcaster creates a new Gossip-based broadcaster for CRDT
func NewGossipBroadcaster(ctx context.Context, gossip GossipPublisher, topic string) *GossipBroadcaster {
	ctx, cancel := context.WithCancel(ctx)
	return &GossipBroadcaster{
		gossip:   gossip,
		topic:    topic,
		dataChan: make(chan []byte, 100),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Broadcast sends data via Gossip
func (gb *GossipBroadcaster) Broadcast(ctx context.Context, data []byte) error {
	msg := event.Message{
		Body: data,
	}
	return gb.gossip.Publish(msg, gb.topic)
}

// Next receives broadcasted data
func (gb *GossipBroadcaster) Next(ctx context.Context) ([]byte, error) {
	select {
	case data := <-gb.dataChan:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-gb.ctx.Done():
		return nil, gb.ctx.Err()
	}
}

// Receive is called by Gossip subscription handler to deliver data
func (gb *GossipBroadcaster) Receive(data []byte) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	
	select {
	case gb.dataChan <- data:
	case <-gb.ctx.Done():
		return
	default:
		// Channel full, drop oldest
	}
}

// Close stops the broadcaster
func (gb *GossipBroadcaster) Close() {
	gb.cancel()
	close(gb.dataChan)
}
