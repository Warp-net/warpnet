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
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

const testPeerA = "12D3KooWT37h7ojLbHwwFdzHeRakaP8cyakXYPspK7kVsWYNCY1x"

const testPeerB = "12D3KooWRqJrnUdkzB6YaPaJ5oxtBVsqZ3XpH1FkMkFg2nNYk7ws"

const (
	routeMessage  = WarpRoute("/public/post/message/0.0.0")
	routeReaction = WarpRoute("/public/post/react/0.0.0")
	routeFollow   = WarpRoute("/public/post/follow/0.0.0")
	routeUnfollow = WarpRoute("/public/post/unfollow/0.0.0")
	routeGetUser  = WarpRoute("/public/get/user/0.0.0")
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
	mu        sync.Mutex
	results   []error
	responses [][]byte
	sentTo    []WarpRoute
	msgIDs    []string
	calls     int
}

func (s *fakeSender) SendMessage(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte, msgID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.sentTo = append(s.sentTo, r)
	s.msgIDs = append(s.msgIDs, msgID)
	var err error
	if len(s.results) > 0 {
		err = s.results[0]
		s.results = s.results[1:]
	}
	var response []byte
	if len(s.responses) > 0 {
		response = s.responses[0]
		s.responses = s.responses[1:]
	}
	return response, err
}

func (s *fakeSender) routes() []WarpRoute {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]WarpRoute(nil), s.sentTo...)
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

	require.NoError(t, o.Enqueue(testPeerA, routeMessage, []byte(`{}`)))

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1)
	require.Equal(t, string(routeMessage), entries[0].Destination)
	require.True(t, o.isPending(testPeerA))
	require.False(t, o.isPending(testPeerB), "only the enqueued peer is pending")
}

func TestEnqueueQueuesFollowAgainAfterUnfollow(t *testing.T) {
	store := newFakeStore()
	o := newTestOutbox(t, store, nil)
	payload := []byte(`{"follower_id":"a","following_id":"b"}`)

	require.NoError(t, o.Enqueue(testPeerA, routeFollow, payload))
	require.NoError(t, o.Enqueue(testPeerA, routeUnfollow, payload))
	require.NoError(t, o.Enqueue(testPeerA, routeFollow, payload))

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 3)
	require.Equal(t, string(routeFollow), entries[2].Destination, "the last action wins on delivery")
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

	require.Error(t, o.Enqueue(testPeerA, routeGetUser, []byte(`{}`)))

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

func TestFlushQueuedNodesDeliversToEveryNode(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeFollow))
	mustEnqueue(store, testPeerB, string(routeMessage))

	sender := &fakeSender{results: []error{nil, nil}}
	o := newTestOutbox(t, store, sender)

	o.flushQueuedNodes()

	require.Eventually(t, func() bool {
		nodes, _ := store.ListNodes()
		return len(nodes) == 0 && sender.callCount() == 2
	}, 3*time.Second, 10*time.Millisecond, "a tick retries every queued node, online event or not")
}

func TestFlushDeliversAllAndClearsPending(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))
	mustEnqueue(store, testPeerA, string(routeReaction))

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
	mustEnqueue(store, testPeerA, string(routeReaction))
	mustEnqueue(store, testPeerA, string(routeFollow))

	sender := &fakeSender{results: []error{nil, warpnet.ErrNodeIsOffline}}
	o := newTestOutbox(t, store, sender)

	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 2, "only the first delivered; rest kept")
	require.Equal(t, string(routeReaction), entries[0].Destination, "FIFO order preserved")
	require.Equal(t, 2, sender.callCount(), "stopped after offline, third not attempted")
}

func TestFlushKeepsResponseReadQueuedAndRedeliversWithSameId(t *testing.T) {
	// A lost response says the request reached the peer, not that the peer
	// applied it, so the entry stays until a response confirms delivery.
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))
	queued, _ := store.ListByNode(testPeerA)

	sender := &fakeSender{results: []error{ErrResponseRead, nil}}
	o := newTestOutbox(t, store, sender)

	o.flushNode(testPeerA)
	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1, "delivered-but-response-lost entry kept")

	o.flushNode(testPeerA)
	entries, _ = store.ListByNode(testPeerA)
	require.Empty(t, entries, "confirmed on redelivery")

	// The same wire id both times lets the peer's idempotency cache answer the
	// redelivery instead of applying the request again.
	require.Equal(t, []string{queued[0].MessageId, queued[0].MessageId}, sender.msgIDs)
}

func TestFlushKeepsRejectedForTTL(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))
	mustEnqueue(store, testPeerA, string(routeReaction))

	sender := &fakeSender{results: []error{warpnet.WarpError("rejected"), nil}}
	o := newTestOutbox(t, store, sender)

	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1, "rejected message stays queued")
	require.Equal(t, string(routeMessage), entries[0].Destination)
	require.Equal(t, 2, sender.callCount(), "failure doesn't block later messages")
}

func TestFlushKeepsRateLimitedQueued(t *testing.T) {
	store := newFakeStore()
	mustEnqueue(store, testPeerA, string(routeMessage))
	mustEnqueue(store, testPeerA, string(routeMessage))
	mustEnqueue(store, testPeerA, string(routeReaction))

	limited, _ := json.Marshal(event.ResponseError{
		Code: event.RateLimitErrorCode, Message: "too many requests for this route",
	})
	sender := &fakeSender{
		results:   []error{nil, nil},
		responses: [][]byte{limited, nil},
	}
	o := newTestOutbox(t, store, sender)
	o.mu.Lock()
	o.pending[testPeerA] = struct{}{}
	o.mu.Unlock()

	o.flushNode(testPeerA)

	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 2, "rate limited messages stay queued")
	for _, e := range entries {
		require.Equal(t, string(routeMessage), e.Destination)
	}
	require.Equal(
		t, []WarpRoute{routeMessage, routeReaction}, sender.routes(),
		"the limited route is skipped for the rest of the pass, other routes still flush",
	)
	require.True(t, o.hasQueued(testPeerA), "peer still has work for the next flush")
}

func TestIsRateLimited(t *testing.T) {
	limited, err := json.Marshal(event.ResponseError{
		Code: event.RateLimitErrorCode, Message: "too many requests",
	})
	require.NoError(t, err)
	require.True(t, isRateLimited(limited))

	internal, err := json.Marshal(event.ResponseError{Code: 5000, Message: "internal node error"})
	require.NoError(t, err)
	require.False(t, isRateLimited(internal))

	require.False(t, isRateLimited([]byte(event.Accepted)))
	require.False(t, isRateLimited([]byte(`{"tweet_id":"tweet-1"}`)))
	require.False(t, isRateLimited([]byte("not json")))
	require.False(t, isRateLimited(nil))
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

func TestDeliverSendsCommittedEntryAndRemovesIt(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{responses: [][]byte{[]byte(`{"ok":true}`)}}
	o := newTestOutbox(t, store, sender)

	// Committed by the caller, e.g. together with the chat message it delivers.
	entry, _ := store.Enqueue(testPeerA, string(routeMessage), []byte(`{}`))

	resp, err := o.Deliver(testPeerA, entry.MessageId)
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(resp))
	entries, _ := store.ListByNode(testPeerA)
	require.Empty(t, entries)
	require.False(t, o.isPending(testPeerA))
}

func TestDeliverKeepsEntryQueuedWhenPeerIsOffline(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{results: []error{warpnet.ErrNodeIsOffline}}
	o := newTestOutbox(t, store, sender)

	entry, _ := store.Enqueue(testPeerA, string(routeMessage), []byte(`{}`))

	_, err := o.Deliver(testPeerA, entry.MessageId)
	require.ErrorIs(t, err, ErrQueuedInOutbox)
	require.ErrorIs(t, err, warpnet.ErrNodeIsOffline)
	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1)
	require.True(t, o.isPending(testPeerA), "the next flush must still pick it up")
}

func TestDeliverKeepsEntryQueuedOnSendFailure(t *testing.T) {
	// A timeout or reset used to mark the message undelivered and forget it.
	store := newFakeStore()
	sendErr := warpnet.WarpError("stream reset")
	sender := &fakeSender{results: []error{sendErr}}
	o := newTestOutbox(t, store, sender)

	entry, _ := store.Enqueue(testPeerA, string(routeMessage), []byte(`{}`))

	_, err := o.Deliver(testPeerA, entry.MessageId)
	require.ErrorIs(t, err, ErrQueuedInOutbox)
	require.ErrorIs(t, err, sendErr)
	entries, _ := store.ListByNode(testPeerA)
	require.Len(t, entries, 1)
}

func TestDeliverSendsEarlierQueuedEntriesFirst(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{}
	o := newTestOutbox(t, store, sender)

	store.Enqueue(testPeerA, string(routeReaction), []byte(`{"n":1}`))
	entry, _ := store.Enqueue(testPeerA, string(routeMessage), []byte(`{"n":2}`))

	_, err := o.Deliver(testPeerA, entry.MessageId)
	require.NoError(t, err)
	require.Equal(t, []WarpRoute{routeReaction, routeMessage}, sender.routes())
}

func TestDeliverReportsEntryBehindOfflineStopAsQueued(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{results: []error{warpnet.ErrNodeIsOffline}}
	o := newTestOutbox(t, store, sender)

	store.Enqueue(testPeerA, string(routeReaction), []byte(`{"n":1}`))
	entry, _ := store.Enqueue(testPeerA, string(routeMessage), []byte(`{"n":2}`))

	_, err := o.Deliver(testPeerA, entry.MessageId)
	require.ErrorIs(t, err, ErrQueuedInOutbox)
	require.ErrorIs(t, err, warpnet.ErrNodeIsOffline)
	require.Equal(t, 1, sender.callCount(), "an offline peer stops the flush")
}

func TestDeliverRejectsEmptyIds(t *testing.T) {
	o := newTestOutbox(t, newFakeStore(), &fakeSender{})
	_, err := o.Deliver("", "id")
	require.ErrorIs(t, err, errNotQueued)
	_, err = o.Deliver(testPeerA, "")
	require.ErrorIs(t, err, errNotQueued)
}

type blockingSender struct {
	fakeSender
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (s *blockingSender) SendMessage(peerAddr warpnet.WarpAddrInfo, r WarpRoute, data []byte, msgID string) ([]byte, error) {
	s.once.Do(func() {
		close(s.entered)
		<-s.release
	})
	return s.fakeSender.SendMessage(peerAddr, r, data, msgID)
}

func TestDeliverJoinsFlushThatListedQueueBeforeEntry(t *testing.T) {
	store := newFakeStore()
	sender := &blockingSender{entered: make(chan struct{}), release: make(chan struct{})}
	o := newTestOutbox(t, store, sender)

	store.Enqueue(testPeerA, string(routeReaction), []byte(`{"n":1}`))
	go o.flushNode(testPeerA)
	<-sender.entered // that flush has listed the queue and is sending

	entry, _ := store.Enqueue(testPeerA, string(routeMessage), []byte(`{"n":2}`))
	done := make(chan error, 1)
	go func() {
		_, err := o.Deliver(testPeerA, entry.MessageId)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond) // let Deliver join the in-flight flush
	close(sender.release)

	select {
	case err := <-done:
		require.NoError(t, err, "the entry missed by the shared flush is sent by a second one")
	case <-time.After(5 * time.Second):
		t.Fatal("Deliver did not return")
	}
	entries, _ := store.ListByNode(testPeerA)
	require.Empty(t, entries)
	require.Equal(t, []WarpRoute{routeReaction, routeMessage}, sender.routes())
}
