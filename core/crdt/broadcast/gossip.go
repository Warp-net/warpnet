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
	"sync/atomic"
	"time"

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

	lastNext atomic.Int64 // unix nanos of the last Next return, for the consumer accounting
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
	// The gap since the last return is what the datastore spent on the previous
	// broadcast instead of reading the queue.
	if last := gb.lastNext.Load(); last != 0 {
		metrics.CRDTConsumerBusySeconds.WithLabelValues(gb.topic).Add(time.Since(time.Unix(0, last)).Seconds())
	}
	waitStart := time.Now()

	select {
	case data := <-gb.dataChan:
		metrics.CRDTConsumerWaitSeconds.WithLabelValues(gb.topic).Add(time.Since(waitStart).Seconds())
		metrics.CRDTConsumed.WithLabelValues(gb.topic).Inc()
		gb.lastNext.Store(time.Now().UnixNano())
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
	metrics.CRDTReceivedByTopic.WithLabelValues(gb.topic).Inc()

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
	//
	// Measured on a stand predating #502 this fired on two deltas in three, and
	// was read as the queue being the next thing to fix. It was the two stores
	// fighting over shared bitswap protocol IDs: with #502 in, 25 nodes at K=20
	// evict nothing and the consumer spends microseconds per broadcast.
	select {
	case <-gb.dataChan:
		metrics.CRDTDeltasDropped.Inc()
		metrics.CRDTDroppedByTopic.WithLabelValues(gb.topic).Inc()
	default:
	}
	select {
	case gb.dataChan <- data:
	default:
		metrics.CRDTDeltasDropped.Inc()
		metrics.CRDTDroppedByTopic.WithLabelValues(gb.topic).Inc()
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
