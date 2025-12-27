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

	gossip GossipPubSuber
	topic  string
	dataChan chan []byte

	once     sync.Once
	mx       sync.Mutex
}

const statsTopic = "/warpnet/stats/1.0.0"

// NewGossipBroadcaster creates a new Gossip-based broadcaster for CRDT
func NewGossipBroadcaster(ctx context.Context, gossip GossipPubSuber) (*GossipBroadcaster, error) {
	gb := &GossipBroadcaster{
		gossip:   gossip,
		topic:    statsTopic,
		dataChan: make(chan []byte, 100),
		ctx:      ctx,
		once:     sync.Once{},
		mx:       sync.Mutex{},
	}
	err := gossip.SubscribeRaw(statsTopic, func(data []byte) error {
		gb.Receive(data)
		return nil
	})
	return gb, err
}

// Broadcast sends data via Gossip
func (gb *GossipBroadcaster) Broadcast(_ context.Context, data []byte) error {
	gb.mx.Lock()
	defer gb.mx.Unlock()

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

// Receive is called by Gossip subscription handler to deliver data
func (gb *GossipBroadcaster) Receive(data []byte) {
	gb.mx.Lock()
	defer gb.mx.Unlock()

	select {
	case gb.dataChan <- data:
	case <-gb.ctx.Done():
		gb.close()
		return
	default:
		<-gb.dataChan
		gb.dataChan <- data
	}
}

func (gb *GossipBroadcaster) close() {
	if gb == nil {
		return
	}
	gb.once.Do(func() {
		close(gb.dataChan)
	})
}
