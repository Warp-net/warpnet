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
)

// GossipPublisher interface for publishing to Gossip
type GossipPubSuber interface {
	PublishRaw(topicName string, data []byte) error
	SubscribeRaw(topicName string, h func([]byte) error) error
}

// GossipBroadcaster adapts Gossip to CRDT Broadcaster interface
type GossipBroadcaster struct {
	ctx context.Context

	gossip   GossipPubSuber
	topic    string
	dataChan chan []byte

	mx     sync.Mutex
	closed bool // guarded by mx; once true, dataChan is closed and no more sends are allowed.
}

const statsTopic = "/warpnet/stats/1.0.0"

// NewGossipBroadcaster creates a new Gossip-based broadcaster for CRDT
func NewGossipBroadcaster(ctx context.Context, gossip GossipPubSuber) (*GossipBroadcaster, error) {
	gb := &GossipBroadcaster{
		gossip:   gossip,
		topic:    statsTopic,
		dataChan: make(chan []byte, 100),
		ctx:      ctx,
	}
	err := gossip.SubscribeRaw(statsTopic, func(data []byte) error {
		gb.Receive(data)
		return nil
	})
	return gb, err
}

// Broadcast sends data via Gossip.
//
// gb.gossip and gb.topic are set once at construction and never
// mutated, so this method intentionally does NOT take gb.mx —
// otherwise a slow PublishRaw (network I/O) would block close() and
// Receive() through the same lock, defeating the deadlock fix in
// Receive().
func (gb *GossipBroadcaster) Broadcast(_ context.Context, data []byte) error {
	return gb.gossip.PublishRaw(gb.topic, data)
}

// Next receives broadcasted data
func (gb *GossipBroadcaster) Next(ctx context.Context) ([]byte, error) {
	select {
	case data := <-gb.dataChan:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-gb.ctx.Done():
		gb.close()
		return nil, gb.ctx.Err()
	}
}

// Receive is called by Gossip subscription handler to deliver data.
//
// All channel operations are non-blocking. If the buffer is full, the
// oldest pending message is dropped and the new one is enqueued. The
// previous implementation did `<-gb.dataChan` unconditionally inside the
// `default` branch, which could deadlock under the held mutex when a
// concurrent Next() drained the channel between the select decision
// and the receive.
//
// The `closed` flag and `close()` taking the same mutex prevent a
// "send on closed channel" panic when Next() shuts the broadcaster
// down concurrently with an in-flight Receive.
func (gb *GossipBroadcaster) Receive(data []byte) {
	gb.mx.Lock()
	defer gb.mx.Unlock()

	if gb.closed {
		return
	}

	select {
	case <-gb.ctx.Done():
		return
	case gb.dataChan <- data:
		return
	default:
	}

	select {
	case <-gb.dataChan:
	default:
	}
	select {
	case gb.dataChan <- data:
	default:
	}
}

func (gb *GossipBroadcaster) close() {
	if gb == nil {
		return
	}
	gb.mx.Lock()
	defer gb.mx.Unlock()
	if gb.closed {
		return
	}
	gb.closed = true
	close(gb.dataChan)
}
