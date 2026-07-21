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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package stream

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const outboxTriggerBuffer = 256

type OutboxStore interface {
	Enqueue(destNodeId, route string, payload []byte) (event.Message, error)
	ListByNode(destNodeId string) ([]event.Message, error)
	Delete(destNodeId, messageId string) error
	ListNodes() ([]string, error)
}

type Sender interface {
	Send(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte) ([]byte, error)
}

type Outbox struct {
	ctx     context.Context
	store   OutboxStore
	sf      singleflight.Group
	trigger chan string
	stop    chan struct{}
	stopped sync.Once

	mu      sync.RWMutex
	sender  Sender
	pending map[string]struct{}
}

func NewOutbox(ctx context.Context, store OutboxStore) *Outbox {
	o := &Outbox{
		ctx:     ctx,
		store:   store,
		trigger: make(chan string, outboxTriggerBuffer),
		stop:    make(chan struct{}),
		pending: make(map[string]struct{}),
	}
	go o.run()
	return o
}

func (o *Outbox) Run(sender Sender) {
	o.mu.Lock()
	o.sender = sender
	o.mu.Unlock()

	nodes, err := o.store.ListNodes()
	if err != nil {
		log.Errorf("outbox: list queued nodes: %v", err)
		return
	}
	for _, nodeId := range nodes {
		select {
		case o.trigger <- nodeId:
		default:
		}
	}
}

func (o *Outbox) getSender() Sender {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.sender
}

func (o *Outbox) Enqueue(nodeIdStr string, route WarpRoute, payload []byte) {
	if o == nil || nodeIdStr == "" || route.IsGet() {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	queued, err := o.store.ListByNode(nodeIdStr)
	if err != nil {
		log.Warnf("outbox: enqueue: list queued for %s: %v", nodeIdStr, err)
		return
	}
	for _, msg := range queued {
		if msg.Destination == string(route) && bytes.Equal(msg.Body, payload) {
			return
		}
	}

	if _, err := o.store.Enqueue(nodeIdStr, string(route), payload); err != nil {
		log.Warnf("outbox: enqueue for %s: %v", nodeIdStr, err)
		return
	}
	o.pending[nodeIdStr] = struct{}{}
}

func (o *Outbox) NotifyOnline(nodeId string) {
	if nodeId == "" {
		return
	}
	select {
	case o.trigger <- nodeId:
	default:
	}
}

func (o *Outbox) Close() {
	o.stopped.Do(func() { close(o.stop) })
}

func (o *Outbox) run() {
	for {
		select {
		case <-o.ctx.Done():
			return
		case <-o.stop:
			return
		case nodeId := <-o.trigger:
			if !o.hasQueued(nodeId) {
				continue
			}
			go o.flushNode(nodeId)
		}
	}
}

func (o *Outbox) hasQueued(nodeId string) bool {
	o.mu.RLock()
	_, ok := o.pending[nodeId]
	o.mu.RUnlock()
	if ok {
		return true
	}

	messages, err := o.store.ListByNode(nodeId)
	if err != nil || len(messages) == 0 {
		return false
	}
	o.mu.Lock()
	o.pending[nodeId] = struct{}{}
	o.mu.Unlock()
	return true
}

func (o *Outbox) flushNode(nodeId string) {
	if nodeId == "" {
		return
	}
	_, _, _ = o.sf.Do(nodeId, func() (any, error) {
		o.flush(nodeId)
		return nil, nil
	})
}

func (o *Outbox) flush(nodeId string) {
	sender := o.getSender()
	if sender == nil {
		return
	}
	peerID := warpnet.FromStringToPeerID(nodeId)
	if peerID == "" {
		log.Errorf("outbox: malformed queued node id: %s", nodeId)
		return
	}

	messages, err := o.store.ListByNode(nodeId)
	if err != nil {
		log.Errorf("outbox: list queued for %s: %v", nodeId, err)
		return
	}

	for _, msg := range messages {
		select {
		case <-o.stop:
			return
		default:
		}
		if o.ctx.Err() != nil {
			return
		}

		_, err := sender.Send(warpnet.WarpAddrInfo{ID: peerID}, WarpRoute(msg.Destination), msg.Body)
		switch {
		case err == nil, errors.Is(err, ErrResponseRead):
			if delErr := o.store.Delete(nodeId, msg.MessageId); delErr != nil {
				log.Errorf("outbox: delete delivered %s/%s: %v", nodeId, msg.MessageId, delErr)
			}
		case errors.Is(err, warpnet.ErrNodeIsOffline):
			return
		default:
			log.Warnf("outbox: redeliver %s/%s: %v", nodeId, msg.MessageId, err)
		}
	}

	o.clearIfDrained(nodeId)
}

func (o *Outbox) clearIfDrained(nodeId string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	messages, err := o.store.ListByNode(nodeId)
	if err == nil && len(messages) == 0 {
		delete(o.pending, nodeId)
	}
}

func (o *Outbox) isPending(nodeId string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	_, ok := o.pending[nodeId]
	return ok
}
