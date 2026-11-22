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

package trust

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

const owner = "owner"

type fakeFollows struct {
	followings map[string]bool // owner follows
	followers  map[string]bool // follows owner
}

func (f *fakeFollows) GetFollowings(_ string, limit *uint64, cursor *string) ([]domain.ID, string, error) {
	ids := make([]domain.ID, 0, len(f.followings))
	for id := range f.followings {
		ids = append(ids, id)
	}
	return page(ids, limit, cursor)
}

func (f *fakeFollows) IsFollowing(_, id string) bool { return f.followings[id] }
func (f *fakeFollows) IsFollower(_, id string) bool  { return f.followers[id] }

type fakeUsers map[string]domain.User

func (f fakeUsers) Get(id string) (domain.User, error) {
	u, ok := f[id]
	if !ok {
		return domain.User{}, fmt.Errorf("user %s not found", id)
	}
	return u, nil
}

type remoteLists struct {
	followings, followers []string
	isOffline             bool
	isOld                 bool // ignores is_mutual_only
	failure               string
	calls                 int
}

type fakeStreamer map[string]*remoteLists // node id -> its owner's lists

// GenericStream answers the followings route the way a node does for its own
// owner.
func (f fakeStreamer) GenericStream(nodeId string, _ stream.WarpRoute, data any) ([]byte, error) {
	lists := f[nodeId]
	if lists != nil {
		lists.calls++
	}
	if lists == nil || lists.isOffline {
		return nil, warpnet.ErrNodeIsOffline
	}
	if lists.failure != "" {
		return json.Marshal(event.ResponseError{Code: 500, Message: lists.failure})
	}
	ev := data.(event.GetFollowingsEvent)
	if lists.isOld {
		ids, cur, _ := page(lists.followings, ev.Limit, ev.Cursor)
		return json.Marshal(event.FollowingsResponse{Cursor: cur, Followings: ids})
	}
	followers := make(map[string]bool, len(lists.followers))
	for _, id := range lists.followers {
		followers[id] = true
	}
	mutuals := make([]string, 0, len(lists.followings))
	for _, id := range lists.followings {
		if followers[id] {
			mutuals = append(mutuals, id)
		}
	}
	ids, cur, _ := page(mutuals, ev.Limit, ev.Cursor)
	return json.Marshal(event.FollowingsResponse{Cursor: cur, Followings: ids, IsMutualOnly: ev.IsMutualOnly})
}

func page(ids []string, limit *uint64, cursor *string) ([]string, string, error) {
	start := 0
	if cursor != nil && *cursor != "" {
		start, _ = strconv.Atoi(*cursor)
	}
	end := min(start+int(*limit), len(ids))
	if start >= len(ids) {
		return nil, "end", nil
	}
	return ids[start:end], strconv.Itoa(end), nil
}

// stand is an owner with mutuals m1..mN, each on its own node.
type stand struct {
	follows  *fakeFollows
	users    fakeUsers
	streamer fakeStreamer
	circle   *Circle
}

func newStand(t *testing.T) *stand {
	t.Helper()
	s := &stand{
		follows:  &fakeFollows{followings: map[string]bool{}, followers: map[string]bool{}},
		users:    fakeUsers{},
		streamer: fakeStreamer{},
	}
	s.circle = NewCircle(owner, s.follows, s.users, s.streamer)
	s.circle.pause = 0
	return s
}

// mutual adds a mutual of the owner whose own mutuals are the owner plus
// the given users and padding others, so its degree is 1+len(lists)+pad.
func (s *stand) mutual(id string, pad int, lists ...string) *remoteLists {
	s.follows.followings[id] = true
	s.follows.followers[id] = true
	s.users[id] = domain.User{Id: id, NodeId: "node-" + id}
	mutuals := append([]string{owner}, lists...)
	for i := range pad {
		mutuals = append(mutuals, fmt.Sprintf("%s-pad-%d", id, i))
	}
	remote := &remoteLists{followings: mutuals, followers: mutuals}
	s.streamer["node-"+id] = remote
	return remote
}

func TestTrustDirect(t *testing.T) {
	s := newStand(t)
	s.follows.followings["friend"] = true

	require.Equal(t, domain.Trust{IsTrusted: true, Score: 1}, s.circle.Trust(owner))
	require.Equal(t, domain.Trust{IsTrusted: true, Score: 1}, s.circle.Trust("friend"))
	require.False(t, s.circle.Trust("stranger").IsTrusted)
}

func TestTrustFollowerOnlyIsStranger(t *testing.T) {
	s := newStand(t)
	s.follows.followers["fan"] = true

	require.NoError(t, s.circle.Refresh())
	require.False(t, s.circle.Trust("fan").IsTrusted)
}

func TestTrustSplitsByDegree(t *testing.T) {
	s := newStand(t)
	s.mutual("small", 8, "alice") // degree 10
	s.mutual("big1", 98, "bob")   // degree 100
	s.mutual("big2", 97, "carol") // degree 100
	s.mutual("big3", 97, "carol") // degree 100
	require.NoError(t, s.circle.Refresh())

	alice := s.circle.Trust("alice")
	require.True(t, alice.IsTrusted)
	require.InDelta(t, 0.1, alice.Score, 1e-9)
	require.Equal(t, 1, alice.Vouchers)

	bob := s.circle.Trust("bob")
	require.False(t, bob.IsTrusted, "one voucher with 100 mutuals is below the threshold")

	carol := s.circle.Trust("carol")
	require.True(t, carol.IsTrusted, "two vouchers with 100 mutuals reach it")
	require.Equal(t, 2, carol.Vouchers)
}

func TestTrustOnlyMutualEntriesCount(t *testing.T) {
	s := newStand(t)
	remote := s.mutual("m", 0)
	remote.followers = append(remote.followers, "sybil") // follows m, m does not follow back
	require.NoError(t, s.circle.Refresh())

	require.False(t, s.circle.Trust("sybil").IsTrusted)
}

func TestTrustDropsFormerMutualAtOnce(t *testing.T) {
	s := newStand(t)
	s.mutual("m", 0, "alice")
	require.NoError(t, s.circle.Refresh())
	require.True(t, s.circle.Trust("alice").IsTrusted)

	delete(s.follows.followings, "m")
	require.False(t, s.circle.Trust("alice").IsTrusted)
}

func TestTrustCappedMutualVouchesForNobody(t *testing.T) {
	s := newStand(t)
	s.mutual("hub", maxListed, "alice")
	require.NoError(t, s.circle.Refresh())
	require.Nil(t, s.circle.listings["hub"].mutuals)

	require.False(t, s.circle.Trust("alice").IsTrusted)
}

func fans(prefix string, n int) []string {
	ids := make([]string, 0, n)
	for i := range n {
		ids = append(ids, fmt.Sprintf("%s-fan-%d", prefix, i))
	}
	return ids
}

func TestTrustPopularMutualStillVouches(t *testing.T) {
	s := newStand(t)
	remote := s.mutual("star", 38, "alice") // degree 40
	remote.followers = append(fans("star", 160000), remote.followers...)
	require.NoError(t, s.circle.Refresh())

	alice := s.circle.Trust("alice")
	require.True(t, alice.IsTrusted, "many followers do not make a selective mutual indiscriminate")
	require.InDelta(t, 1.0/40, alice.Score, 1e-12)
	require.Equal(t, 1, remote.calls)
}

func TestTrustReaderStillVouches(t *testing.T) {
	s := newStand(t)
	remote := s.mutual("reader", 38, "alice") // degree 40
	remote.followings = append(fans("reads", 160000), remote.followings...)
	require.NoError(t, s.circle.Refresh())

	alice := s.circle.Trust("alice")
	require.True(t, alice.IsTrusted, "following many is not following back many")
	require.InDelta(t, 1.0/40, alice.Score, 1e-12)
	require.Equal(t, 1, remote.calls)
}

func TestTrustAutofollowerVouchesForNobody(t *testing.T) {
	s := newStand(t)
	remote := s.mutual("hub", 0, "alice")
	remote.followings = append(remote.followings, fans("hub", 160000)...)
	remote.followers = remote.followings
	require.NoError(t, s.circle.Refresh())

	require.False(t, s.circle.Trust("alice").IsTrusted)
	require.Equal(t, maxListed/int(pageLimit)+1, remote.calls, "reading stops past maxListed mutuals")
}

func TestTrustOldNodeVouchesForNobody(t *testing.T) {
	s := newStand(t)
	remote := s.mutual("m", 0, "alice")
	remote.isOld = true
	require.NoError(t, s.circle.Refresh())

	require.False(t, s.circle.Trust("alice").IsTrusted, "its followings are not known to be mutual")
}

func TestTrustBridgedNeverExpanded(t *testing.T) {
	s := newStand(t)
	s.mutual("bridged@mastodon.example", 0, "alice")
	s.mutual("m", 0, "remote@mastodon.example")
	require.NoError(t, s.circle.Refresh())

	require.False(t, s.circle.Trust("alice").IsTrusted, "a bridged mutual vouches for nobody")
	require.False(t, s.circle.Trust("remote@mastodon.example").IsTrusted, "a bridged user enters only by a direct follow")
}

func TestRefreshKeepsUnreachableMutual(t *testing.T) {
	s := newStand(t)
	remote := s.mutual("m", 0, "alice")
	require.NoError(t, s.circle.Refresh())

	remote.isOffline = true
	require.NoError(t, s.circle.Refresh(), "an offline mutual is routine, not an error")
	require.True(t, s.circle.Trust("alice").IsTrusted)

	remote.isOffline = false
	remote.failure = "boom"
	require.ErrorContains(t, s.circle.Refresh(), "boom")
	require.True(t, s.circle.Trust("alice").IsTrusted)
}

func TestRefreshPagesThroughMutuals(t *testing.T) {
	s := newStand(t)
	s.mutual("m", int(pageLimit)+5, "alice")
	require.NoError(t, s.circle.Refresh())

	alice := s.circle.Trust("alice")
	require.Equal(t, 1, alice.Vouchers)
	require.InDelta(t, 1/float64(pageLimit+7), alice.Score, 1e-12)
}

func TestRefreshNewListsOnlyUnlistedMutuals(t *testing.T) {
	s := newStand(t)
	old := s.mutual("m1", 0, "alice")
	require.NoError(t, s.circle.Refresh())
	listed := old.calls

	require.NoError(t, s.circle.RefreshNew())
	require.Equal(t, listed, old.calls, "nothing new, nobody asked")

	s.mutual("m2", 0, "bob")
	require.False(t, s.circle.Trust("bob").IsTrusted)
	require.NoError(t, s.circle.RefreshNew())

	require.True(t, s.circle.Trust("bob").IsTrusted, "a new mutual vouches before the next full refresh")
	require.True(t, s.circle.Trust("alice").IsTrusted, "listed mutuals are kept")
	require.Equal(t, listed, old.calls, "an already listed mutual is not asked again")
}

func TestRefreshNewRetriesUnreachableMutual(t *testing.T) {
	s := newStand(t)
	remote := s.mutual("m", 0, "alice")
	remote.isOffline = true
	require.NoError(t, s.circle.RefreshNew())
	require.False(t, s.circle.Trust("alice").IsTrusted)

	remote.isOffline = false
	require.NoError(t, s.circle.RefreshNew())
	require.True(t, s.circle.Trust("alice").IsTrusted)
}

func TestCloseStopsRefreshBeforeNextMutual(t *testing.T) {
	s := newStand(t)
	s.mutual("m1", 0, "alice")
	s.mutual("m2", 0, "bob")
	s.circle.pause = time.Hour

	refreshed := make(chan error, 1)
	go func() { refreshed <- s.circle.Refresh() }()
	time.Sleep(50 * time.Millisecond) // the first mutual is listed, the pause begins
	s.circle.Close()

	select {
	case err := <-refreshed:
		require.ErrorIs(t, err, ErrClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not stop on Close")
	}
}

func TestCloseStopsTheLoop(t *testing.T) {
	s := newStand(t)
	s.circle.Start()

	closed := make(chan struct{})
	go func() {
		s.circle.Close()
		s.circle.Close() // a second Close is a no-op
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not wait the loop out")
	}
}

func TestCloseWithoutStart(t *testing.T) {
	NewCircle(owner, &fakeFollows{}, fakeUsers{}, fakeStreamer{}).Close()
}
