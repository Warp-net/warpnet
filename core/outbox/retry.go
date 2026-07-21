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

// Package outbox persists outgoing stream requests that failed because the
// destination node was offline and replays them, oldest-first, once that node
// is seen online again (via the pubsub discovery signal) or on startup.
package outbox

import (
	"context"
	"errors"
	"sync"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

// maxAttempts caps redelivery tries for a single entry so a message the
// receiver keeps rejecting (non-offline errors) is eventually dropped rather
// than replayed forever.
const maxAttempts = 20

// triggerBuffer bounds the pending flush queue; overflow is safe to drop since
// startup flush and later presence announcements re-trigger.
const triggerBuffer = 256

type Repo interface {
	Enqueue(destNodeId, route string, payload []byte) (database.OutboxEntry, error)
	ListByNode(destNodeId string) ([]database.OutboxEntry, error)
	Save(entry database.OutboxEntry) error
	Delete(destNodeId, id string) error
	ListNodes() ([]string, error)
}

// Sender is the raw node stream used to replay queued requests. It is the
// node's own Stream (not the enqueuing GenericStream wrapper), so a failed
// redelivery does not re-enqueue and loop; the retry service owns the queue.
type Sender interface {
	Stream(nodeId warpnet.WarpPeerID, path stream.WarpRoute, data any) ([]byte, error)
}

type RetryService struct {
	ctx     context.Context
	repo    Repo
	sf      singleflight.Group
	trigger chan string
	stop    chan struct{}
	stopped sync.Once

	mu      sync.RWMutex
	sender  Sender
	pending map[string]struct{} // nodes with at least one queued entry
}

func NewRetryService(ctx context.Context, repo Repo) *RetryService {
	s := &RetryService{
		ctx:     ctx,
		repo:    repo,
		trigger: make(chan string, triggerBuffer),
		stop:    make(chan struct{}),
		pending: make(map[string]struct{}),
	}
	go s.run()
	return s
}

// SetSender wires the redelivery transport. Until it is set, flushes are
// no-ops, so it is safe to construct the service before the node is up.
func (s *RetryService) SetSender(sender Sender) {
	s.mu.Lock()
	s.sender = sender
	s.mu.Unlock()
}

func (s *RetryService) getSender() Sender {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sender
}

// Enqueue stores an undelivered request and marks its destination pending so a
// later NotifyOnline flushes it.
func (s *RetryService) Enqueue(destNodeId, route string, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.repo.Enqueue(destNodeId, route, payload); err != nil {
		return err
	}
	s.pending[destNodeId] = struct{}{}
	return nil
}

// NotifyOnlinePeer matches the discovery.DiscoveryHandler signature so the
// retry service can be registered as an independent handler on the pubsub
// discovery topic (alongside, and without coupling to, the discovery service).
func (s *RetryService) NotifyOnlinePeer(info warpnet.WarpAddrInfo) {
	s.NotifyOnline(info.ID.String())
}

// NotifyOnline is the pubsub-discovery hook: it is called for every peer seen
// on the discovery topic. It is O(1) and does no I/O when the node has nothing
// queued, so it is cheap to call on every gossip announcement.
func (s *RetryService) NotifyOnline(nodeId string) {
	if nodeId == "" || !s.isPending(nodeId) {
		return
	}
	select {
	case s.trigger <- nodeId:
	default: // queue full; startup flush / later presence will retry
	}
}

// FlushAllOnStart schedules a one-shot replay of every node that still has
// queued entries, covering the case where a peer is already online at startup
// and won't re-announce for a while.
func (s *RetryService) FlushAllOnStart() {
	nodes, err := s.repo.ListNodes()
	if err != nil {
		log.Errorf("outbox: list nodes on start: %v", err)
		return
	}
	for _, nodeId := range nodes {
		s.mu.Lock()
		s.pending[nodeId] = struct{}{}
		s.mu.Unlock()
		select {
		case s.trigger <- nodeId:
		default:
		}
	}
}

func (s *RetryService) Close() {
	s.stopped.Do(func() { close(s.stop) })
}

func (s *RetryService) run() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stop:
			return
		case nodeId := <-s.trigger:
			s.flushNode(nodeId)
		}
	}
}

func (s *RetryService) flushNode(nodeId string) {
	if nodeId == "" {
		return
	}
	_, _, _ = s.sf.Do(nodeId, func() (any, error) {
		s.flush(nodeId)
		return nil, nil
	})
}

func (s *RetryService) flush(nodeId string) {
	sender := s.getSender()
	if sender == nil {
		return
	}

	peerID := warpnet.FromStringToPeerID(nodeId)
	if peerID == "" {
		log.Errorf("outbox: malformed dest node id: %s", nodeId)
		return
	}

	entries, err := s.repo.ListByNode(nodeId)
	if err != nil {
		log.Errorf("outbox: list queued for %s: %v", nodeId, err)
		return
	}

	for _, entry := range entries {
		if s.ctx.Err() != nil {
			return
		}
		_, err := sender.Stream(peerID, stream.WarpRoute(entry.Route), entry.Payload)
		switch {
		case err == nil:
			// The peer answered (delivered). A rejection is delivered too;
			// retrying wouldn't change the outcome, so drop it either way.
			if delErr := s.repo.Delete(nodeId, entry.Id); delErr != nil {
				log.Errorf("outbox: delete delivered %s/%s: %v", nodeId, entry.Id, delErr)
			}
		case errors.Is(err, warpnet.ErrNodeIsOffline):
			// Still offline: stop to preserve FIFO and avoid burning attempts.
			return
		default:
			entry.Attempts++
			if entry.Attempts >= maxAttempts {
				log.Warnf("outbox: dropping %s/%s after %d attempts: %v",
					nodeId, entry.Id, entry.Attempts, err)
				_ = s.repo.Delete(nodeId, entry.Id)
				continue
			}
			if saveErr := s.repo.Save(entry); saveErr != nil {
				log.Errorf("outbox: save attempt %s/%s: %v", nodeId, entry.Id, saveErr)
			}
		}
	}

	s.clearIfDrained(nodeId)
}

func (s *RetryService) clearIfDrained(nodeId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.repo.ListByNode(nodeId)
	if err == nil && len(entries) == 0 {
		delete(s.pending, nodeId)
	}
}

func (s *RetryService) isPending(nodeId string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.pending[nodeId]
	return ok
}
