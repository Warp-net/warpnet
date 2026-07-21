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

//nolint:all
package stream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

const testPeerA = "12D3KooWT37h7ojLbHwwFdzHeRakaP8cyakXYPspK7kVsWYNCY1x"

const (
	routeMessage = WarpRoute("/public/post/message/0.0.0")
	routeLike    = WarpRoute("/public/post/like/0.0.0")
	routeFollow  = WarpRoute("/public/post/follow/0.0.0")
	routeGetUser = WarpRoute("/public/get/user/0.0.0")
)

type fakeStore struct {
	mu    sync.Mutex
	items map[string][]event.Message
}

func newFakeStore() *fakeStore { return &fakeStore{items: map[string][]event.Message{}} }

func (r *fakeStore) Enqueue(dest, route string, payload []byte) (event.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := event.Message{MessageId: ulid.Make().String(), Destination: route, Body: payload}
	r.items[dest] = append(r.items[dest], e)
	return e, nil
}

func (r *fakeStore) ListByNode(dest string) ([]event.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]event.Message, len(r.items[dest]))
	copy(out, r.items[dest])
	return out, nil
}

func (r *fakeStore) Delete(dest, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.items[dest]
	for i, e := range cur {
		if string(e.MessageId) == id {
			r.items[dest] = append(cur[:i], cur[i+1:]...)
			break
		}
	}
	if len(r.items[dest]) == 0 {
		delete(r.items, dest)
	}
	return nil
}

func (r *fakeStore) ListNodes() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	nodes := make([]string, 0, len(r.items))
	for n := range r.items {
		nodes = append(nodes, n)
	}
	return nodes, nil
}

type fakeSender struct {
	mu      sync.Mutex
	results []error
	calls   int
}

func (s *fakeSender) Send(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	var err error
	if len(s.results) > 0 {
		err = s.results[0]
		s.results = s.results[1:]
	}
	return nil, err
}

func (s *fakeSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func newTestOutbox(t *testing.T, store OutboxStore, sender Sender) *Outbox {
	t.Helper()
	o := NewOutbox(context.Background(), store)
	if sender != nil {
		o.mu.Lock()
		o.sender = sender
		o.mu.Unlock()
	}
	t.Cleanup(o.Close)
	return o
}

func TestEnqueueQueuesWriteAndMarksPending(t *testing.T) {
	store := newFakeStore()
	o := newTestOutbox(t, store, nil)

	o.Enqueue(testPeerA, routeMessage, []byte(`{}`))

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1)
	require.Equal(t, string(routeMessage), entries[0].Destination)
	require.True(t, o.isPending(testPeerA))
}

func TestEnqueueDedupesIdenticalPending(t *testing.T) {
	store := newFakeStore()
	o := newTestOutbox(t, store, nil)

	o.Enqueue(testPeerA, routeMessage, []byte(`{"n":1}`))
	o.Enqueue(testPeerA, routeMessage, []byte(`{"n":1}`))
	o.Enqueue(testPeerA, routeMessage, []byte(`{"n":2}`))

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 2, "identical (route, payload) queued once")
}

func TestNotifyOnlineFlushesQueued(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{results: []error{nil}}
	o := newTestOutbox(t, store, sender)

	o.Enqueue(testPeerA, routeMessage, []byte(`{}`))
	o.NotifyOnline(testPeerA)

	require.Eventually(t, func() bool {
		entries, _ := store.ListByNode(testPeerA)
		return len(entries) == 0 && sender.callCount() == 1
	}, 3*time.Second, 10*time.Millisecond, "presence announcement drains the queue")
}

func TestEnqueueSkipsReads(t *testing.T) {
	store := newFakeStore()
	o := newTestOutbox(t, store, nil)

	o.Enqueue(testPeerA, routeGetUser, []byte(`{}`))

	entries, _ := store.ListByNode(testPeerA)
	require.Empty(t, entries, "GET reads are never queued")
	require.False(t, o.isPending(testPeerA))
}

func TestRunReplaysQueuedFromPreviousRun(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))

	sender := &fakeSender{results: []error{nil}}
	o := newTestOutbox(t, store, nil)
	o.Run(sender)

	require.Eventually(t, func() bool {
		entries, _ := store.ListByNode(testPeerA)
		return len(entries) == 0 && sender.callCount() == 1
	}, 3*time.Second, 10*time.Millisecond, "startup flush delivers persisted entries")
}

func TestFlushDeliversAllAndClearsPending(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))
	mustEnqueue(store, testPeerA, string(routeLike))

	sender := &fakeSender{results: []error{nil, nil}}
	o := newTestOutbox(t, store, sender)
	o.mu.Lock()
	o.pending[testPeerA] = struct{}{}
	o.mu.Unlock()

	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Empty(t, entries, "all delivered entries deleted")
	require.False(t, o.isPending(testPeerA), "pending cleared after drain")
	require.Equal(t, 2, sender.callCount())
}

func TestFlushStopsOnOfflinePreservingFIFO(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))
	mustEnqueue(store, testPeerA, string(routeLike))
	mustEnqueue(store, testPeerA, string(routeFollow))

	sender := &fakeSender{results: []error{nil, warpnet.ErrNodeIsOffline}}
	o := newTestOutbox(t, store, sender)

	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 2, "only the first delivered; rest kept")
	require.Equal(t, string(routeLike), entries[0].Destination, "FIFO order preserved")
	require.Equal(t, 2, sender.callCount(), "stopped after offline, third not attempted")
}

func TestFlushTreatsResponseReadAsDelivered(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))

	sender := &fakeSender{results: []error{ErrResponseRead}}
	o := newTestOutbox(t, store, sender)

	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Empty(t, entries, "delivered-but-response-lost entry dropped")
}

func TestFlushKeepsRejectedForTTL(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))
	mustEnqueue(store, testPeerA, string(routeLike))

	sender := &fakeSender{results: []error{warpnet.WarpError("rejected"), nil}}
	o := newTestOutbox(t, store, sender)

	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1, "rejected message stays queued")
	require.Equal(t, string(routeMessage), entries[0].Destination)
	require.Equal(t, 2, sender.callCount(), "failure doesn't block later messages")
}

func TestFlushNoSenderKeepsEntries(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))

	o := newTestOutbox(t, store, nil)
	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1, "nothing delivered without a sender")
}

func mustEnqueue(store OutboxStore, node, route string) {
	_, _ = store.Enqueue(node, route, []byte(`{}`))
}
