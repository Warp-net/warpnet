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
package outbox

import (
	"context"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// a valid libp2p peer id so flush's FromStringToPeerID conversion succeeds.
const testPeerA = "12D3KooWT37h7ojLbHwwFdzHeRakaP8cyakXYPspK7kVsWYNCY1x"

// fakeRepo is an ordered in-memory Repo.
type fakeRepo struct {
	mu    sync.Mutex
	items map[string][]database.OutboxEntry
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[string][]database.OutboxEntry{}} }

func (r *fakeRepo) Enqueue(dest, route string, payload []byte) (database.OutboxEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := database.OutboxEntry{Id: ulid.Make().String(), DestNodeId: dest, Route: route, Payload: payload}
	r.items[dest] = append(r.items[dest], e)
	return e, nil
}

func (r *fakeRepo) ListByNode(dest string) ([]database.OutboxEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]database.OutboxEntry, len(r.items[dest]))
	copy(out, r.items[dest])
	return out, nil
}

func (r *fakeRepo) Save(entry database.OutboxEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.items[entry.DestNodeId] {
		if e.Id == entry.Id {
			r.items[entry.DestNodeId][i] = entry
			return nil
		}
	}
	return nil
}

func (r *fakeRepo) Delete(dest, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.items[dest]
	for i, e := range cur {
		if e.Id == id {
			r.items[dest] = append(cur[:i], cur[i+1:]...)
			break
		}
	}
	if len(r.items[dest]) == 0 {
		delete(r.items, dest)
	}
	return nil
}

func (r *fakeRepo) ListNodes() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	nodes := make([]string, 0, len(r.items))
	for n := range r.items {
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// fakeSender returns a scripted error per delivery attempt.
type fakeSender struct {
	mu       sync.Mutex
	results  []error // consumed in order
	calls    int
	gotRoute []stream.WarpRoute
}

func (s *fakeSender) Stream(nodeId warpnet.WarpPeerID, path stream.WarpRoute, data any) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.gotRoute = append(s.gotRoute, path)
	var err error
	if len(s.results) > 0 {
		err = s.results[0]
		s.results = s.results[1:]
	}
	return nil, err
}

func newService(t *testing.T, repo Repo) *RetryService {
	t.Helper()
	s := NewRetryService(context.Background(), repo)
	t.Cleanup(s.Close)
	return s
}

func TestFlushDeliversAllAndClearsPending(t *testing.T) {
	repo := newFakeRepo()
	node := testPeerA
	require.NoError(t, mustEnqueue(repo, node, "/public/post/message/0.0.0"))
	require.NoError(t, mustEnqueue(repo, node, "/public/post/like/0.0.0"))

	s := newService(t, repo)
	sender := &fakeSender{results: []error{nil, nil}}
	s.SetSender(sender)
	s.mu.Lock()
	s.pending[node] = struct{}{}
	s.mu.Unlock()

	s.flushNode(node)

	entries, _ := repo.ListByNode(node)
	require.Empty(t, entries, "all delivered entries deleted")
	require.False(t, s.isPending(node), "pending cleared after drain")
	require.Equal(t, 2, sender.calls)
}

func TestFlushStopsOnOfflinePreservingFIFO(t *testing.T) {
	repo := newFakeRepo()
	node := testPeerA
	require.NoError(t, mustEnqueue(repo, node, "/public/post/message/0.0.0"))
	require.NoError(t, mustEnqueue(repo, node, "/public/post/like/0.0.0"))
	require.NoError(t, mustEnqueue(repo, node, "/public/post/follow/0.0.0"))

	s := newService(t, repo)
	// first delivers, second reports offline -> stop
	sender := &fakeSender{results: []error{nil, warpnet.ErrNodeIsOffline}}
	s.SetSender(sender)

	s.flushNode(node)

	entries, _ := repo.ListByNode(node)
	require.Len(t, entries, 2, "only the first was delivered; rest kept")
	require.Equal(t, "/public/post/like/0.0.0", entries[0].Route, "FIFO order preserved")
	require.Equal(t, 2, sender.calls, "stopped after offline, third not attempted")
}

func TestFlushDropsAfterMaxAttempts(t *testing.T) {
	repo := newFakeRepo()
	node := testPeerA
	require.NoError(t, mustEnqueue(repo, node, "/public/post/message/0.0.0"))

	s := newService(t, repo)
	rejectErr := warpnet.WarpError("rejected")
	sender := &fakeSender{}
	s.SetSender(sender)

	// Each flush increments attempts by 1 (one non-offline error).
	for i := 0; i < maxAttempts; i++ {
		sender.mu.Lock()
		sender.results = []error{rejectErr}
		sender.mu.Unlock()
		s.flushNode(node)
	}

	entries, _ := repo.ListByNode(node)
	require.Empty(t, entries, "entry dropped once attempts hit the cap")
}

func TestFlushNoSenderKeepsEntries(t *testing.T) {
	repo := newFakeRepo()
	node := testPeerA
	require.NoError(t, mustEnqueue(repo, node, "/public/post/message/0.0.0"))

	s := newService(t, repo)
	// no SetSender
	s.flushNode(node)

	entries, _ := repo.ListByNode(node)
	require.Len(t, entries, 1, "nothing delivered without a sender")
}

func mustEnqueue(repo Repo, node, route string) error {
	_, err := repo.Enqueue(node, route, []byte(`{}`))
	return err
}
