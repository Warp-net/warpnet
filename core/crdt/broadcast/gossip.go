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

// Package broadcast carries CRDT deltas between replicas.
package broadcast

import (
	"context"
	"sync"

	"github.com/Warp-net/warpnet/core/metrics"
)

// GossipPubSuber is the pubsub this broadcaster rides.
type GossipPubSuber interface {
	PublishRaw(topicName string, data []byte) error
	SubscribeRaw(topicName string, h func([]byte) error) error
}

// Gossip adapts a gossip topic to the broadcaster a CRDT datastore expects.
// Each datastore needs a topic of its own: replicas of different stores on
// one topic would merge each other's heads.
type Gossip struct {
	ctx context.Context

	gossip   GossipPubSuber
	topic    string
	dataChan chan []byte

	mx     sync.Mutex
	closed bool // guarded by mx; once true, dataChan is closed and no more sends are allowed.
}

// NewGossip subscribes to topic and broadcasts on it.
func NewGossip(ctx context.Context, gossip GossipPubSuber, topic string) (*Gossip, error) {
	gb := &Gossip{
		gossip:   gossip,
		topic:    topic,
		dataChan: make(chan []byte, 100),
		ctx:      ctx,
	}
	err := gossip.SubscribeRaw(topic, func(data []byte) error {
		gb.Receive(data)
		return nil
	})
	return gb, err
}

func (gb *Gossip) Broadcast(_ context.Context, data []byte) error {
	return gb.gossip.PublishRaw(gb.topic, data)
}

// Next receives broadcasted data
func (gb *Gossip) Next(ctx context.Context) ([]byte, error) {
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

func (gb *Gossip) Receive(data []byte) {
	gb.mx.Lock()
	defer gb.mx.Unlock()

	if gb.closed {
		return
	}

	metrics.CRDTDeltasReceived.Inc()

	select {
	case <-gb.ctx.Done():
		return
	case gb.dataChan <- data:
		metrics.CRDTQueueDepth.Set(float64(len(gb.dataChan)))
		return
	default:
	}

	// The queue is full, so the oldest delta gives way to the newest. What is
	// evicted here is only recoverable through a rebroadcast round a minute
	// later, which is why the loss is worth a number of its own.
	select {
	case <-gb.dataChan:
		metrics.CRDTDeltasDropped.Inc()
	default:
	}
	select {
	case gb.dataChan <- data:
	default:
		metrics.CRDTDeltasDropped.Inc()
	}
	metrics.CRDTQueueDepth.Set(float64(len(gb.dataChan)))
}

func (gb *Gossip) close() {
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
