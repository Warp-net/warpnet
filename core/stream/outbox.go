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
	"fmt"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const (
	outboxTriggerBuffer = 256
	outboxFlushInterval = 5 * time.Minute
)

const (
	ErrQueuedInOutbox = warpnet.WarpError("queued in outbox")
	errNotQueued      = warpnet.WarpError("outbox: not queued")
	errRateLimited    = warpnet.WarpError("outbox: rate limited")
)

type OutboxStore interface {
	Enqueue(destNodeId, route string, payload []byte) (event.Message, error)
	ListByNode(destNodeId string) ([]event.Message, error)
	Delete(destNodeId, messageId string) error
	ListNodes() ([]string, error)
}

type Sender interface {
	SendMessage(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte, msgID string) ([]byte, error)
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

func (o *Outbox) Enqueue(nodeIdStr string, route WarpRoute, payload []byte) error {
	if o == nil || nodeIdStr == "" || route.IsGet() {
		return errNotQueued
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	queued, err := o.store.ListByNode(nodeIdStr)
	if err != nil {
		log.Warnf("outbox: enqueue: list queued for %s: %v", nodeIdStr, err)
		return err
	}
	if n := len(queued); n > 0 {
		last := queued[n-1]
		if last.Destination == string(route) && bytes.Equal(last.Body, payload) {
			return nil
		}
	}

	if _, err := o.store.Enqueue(nodeIdStr, string(route), payload); err != nil {
		log.Warnf("outbox: enqueue for %s: %v", nodeIdStr, err)
		return err
	}
	o.pending[nodeIdStr] = struct{}{}
	return nil
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
	ticker := time.NewTicker(outboxFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-o.stop:
			return
		case <-ticker.C:
			o.flushQueuedNodes()
		case nodeId := <-o.trigger:
			if !o.hasQueued(nodeId) {
				continue
			}
			go o.flushNode(nodeId)
		}
	}
}

func (o *Outbox) flushQueuedNodes() {
	nodes, err := o.store.ListNodes()
	if err != nil {
		log.Errorf("outbox: list queued nodes: %v", err)
		return
	}
	for _, nodeId := range nodes {
		go o.flushNode(nodeId)
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
		return o.flush(nodeId), nil
	})
}

// deliveryResult is the outcome of one queued entry in a flush. A nil err means
// the entry left the queue; ErrQueuedInOutbox means it stays queued for redelivery.
type deliveryResult struct {
	response []byte
	err      error
}

func stillQueued(cause error) deliveryResult {
	return deliveryResult{err: fmt.Errorf("%w: %w", ErrQueuedInOutbox, cause)}
}

const errFlushInterrupted = warpnet.WarpError("outbox: flush interrupted")

// Deliver flushes nodeId's queue now and reports the outcome for messageId, an
// entry the caller already committed to the store. It flushes the whole queue
// rather than sending the one entry, so per-node order holds and a concurrent
// flush cannot send the entry twice.
func (o *Outbox) Deliver(nodeId, messageId string) ([]byte, error) {
	if o == nil || nodeId == "" || messageId == "" {
		return nil, errNotQueued
	}
	o.mu.Lock()
	o.pending[nodeId] = struct{}{}
	o.mu.Unlock()

	// A flush already in progress may have listed the queue before the entry
	// was committed; the second pass then starts a flush that sees it.
	for range 2 {
		v, _, _ := o.sf.Do(nodeId, func() (any, error) {
			return o.flush(nodeId), nil
		})
		results, _ := v.(map[string]deliveryResult)
		if res, ok := results[messageId]; ok {
			return res.response, res.err
		}
	}
	return nil, fmt.Errorf("%w: %w", ErrQueuedInOutbox, errFlushInterrupted)
}

func (o *Outbox) flush(nodeId string) map[string]deliveryResult {
	results := make(map[string]deliveryResult)

	sender := o.getSender()
	if sender == nil {
		return results
	}
	peerID := warpnet.FromStringToPeerID(nodeId)
	if peerID == "" {
		log.Errorf("outbox: malformed queued node id: %s", nodeId)
		return results
	}

	messages, err := o.store.ListByNode(nodeId)
	if err != nil {
		log.Errorf("outbox: list queued for %s: %v", nodeId, err)
		return results
	}
	// Entries the loop does not reach stay queued.
	for _, msg := range messages {
		results[msg.MessageId] = stillQueued(errFlushInterrupted)
	}

	limitedRoutes := make(map[string]struct{})

	for i, msg := range messages {
		select {
		case <-o.stop:
			return results
		default:
		}
		if o.ctx.Err() != nil {
			return results
		}
		if _, ok := limitedRoutes[msg.Destination]; ok {
			results[msg.MessageId] = stillQueued(errRateLimited)
			continue
		}

		// The entry id goes on the wire as the message id, so every redelivery
		// of this entry is the same request to the peer's idempotency cache.
		response, err := sender.SendMessage(
			warpnet.WarpAddrInfo{ID: peerID}, WarpRoute(msg.Destination), msg.Body, msg.MessageId,
		)
		switch {
		case err == nil:
			if isRateLimited(response) {
				log.Infof("outbox: %s rate limited %s, keeping it queued", nodeId, msg.Destination)
				limitedRoutes[msg.Destination] = struct{}{}
				res := stillQueued(errRateLimited)
				res.response = response
				results[msg.MessageId] = res
				continue
			}
			if delErr := o.store.Delete(nodeId, msg.MessageId); delErr != nil {
				log.Errorf("outbox: delete delivered %s/%s: %v", nodeId, msg.MessageId, delErr)
			}
			results[msg.MessageId] = deliveryResult{response: response}
		case errors.Is(err, warpnet.ErrNodeIsOffline):
			for _, rest := range messages[i:] {
				results[rest.MessageId] = stillQueued(err)
			}
			return results
		default:
			// ErrResponseRead lands here too: the request reached the peer, but
			// nothing says the peer applied it, so the entry is not delivered.
			// Redelivering it is safe because the peer dedups on its id.
			log.Warnf("outbox: redeliver %s/%s: %v", nodeId, msg.MessageId, err)
			results[msg.MessageId] = stillQueued(err)
		}
	}

	o.clearIfDrained(nodeId)
	return results
}

func isRateLimited(response []byte) bool {
	if len(response) == 0 {
		return false
	}
	var respErr event.ResponseError
	if err := json.Unmarshal(response, &respErr); err != nil {
		return false
	}
	return respErr.Code == event.RateLimitErrorCode
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
