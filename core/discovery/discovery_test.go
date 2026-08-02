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
package discovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/backoff"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	selfID  = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	peerID  = "12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU"
	peerID2 = "12D3KooWNXSGyfTuYc3JznW48jay73BtQgHszWfPpyF581EWcpGJ"
)

type fakeNode struct {
	mu sync.Mutex

	info  warpnet.NodeInfo
	store warpnet.WarpPeerstore

	connectErr   error
	connected    []string
	peerstoreHas map[string][]warpnet.WarpAddress

	infoResp []byte
	infoErr  error
	userResp []byte
	userErr  error
	streamed []string

	maxPriority []string
	minPriority []string
	priorities  map[string]warpnet.WarpReachability
}

func newFakeNode() *fakeNode {
	store, err := pstoremem.NewPeerstore()
	if err != nil {
		panic(err)
	}
	return &fakeNode{
		info:         warpnet.NodeInfo{ID: warpnet.FromStringToPeerID(selfID), OwnerId: "self-owner"},
		store:        store,
		peerstoreHas: map[string][]warpnet.WarpAddress{},
		priorities:   map[string]warpnet.WarpReachability{},
	}
}

func (f *fakeNode) NodeInfo() warpnet.NodeInfo { return f.info }

func (f *fakeNode) Peerstore() warpnet.WarpPeerstore { return f.store }

func (f *fakeNode) SimpleConnect(pi warpnet.WarpAddrInfo) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = append(f.connected, pi.ID.String())
	return f.connectErr
}

func (f *fakeNode) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streamed = append(f.streamed, string(path))

	switch path {
	case event.PUBLIC_GET_INFO:
		return f.infoResp, f.infoErr
	case event.PUBLIC_GET_USER:
		return f.userResp, f.userErr
	}
	return nil, nil
}

func (f *fakeNode) SetNodePriority(pid warpnet.WarpPeerID, r warpnet.WarpReachability) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.priorities[pid.String()] = r
}

func (f *fakeNode) SetMaxNodePriority(pid warpnet.WarpPeerID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maxPriority = append(f.maxPriority, pid.String())
}

func (f *fakeNode) SetMinNodePriority(pid warpnet.WarpPeerID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.minPriority = append(f.minPriority, pid.String())
}

func (f *fakeNode) snapshot() ([]string, []string, map[string]warpnet.WarpReachability) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := append([]string(nil), f.connected...)
	s := append([]string(nil), f.streamed...)
	p := make(map[string]warpnet.WarpReachability, len(f.priorities))
	for k, v := range f.priorities {
		p[k] = v
	}
	return c, s, p
}

type fakeNodeRepo struct {
	mu          sync.Mutex
	blocklisted map[string]bool
}

func newFakeNodeRepo() *fakeNodeRepo {
	return &fakeNodeRepo{blocklisted: map[string]bool{}}
}

func (f *fakeNodeRepo) BlocklistRemove(peerId string) error { return nil }

func (f *fakeNodeRepo) IsBlocklisted(peerId string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocklisted[peerId]
}

func (f *fakeNodeRepo) BlocklistExponential(peerId string) error { return nil }

func (f *fakeNodeRepo) BlocklistTerm(peerId string) (*database.BlocklistTerm, error) {
	return &database.BlocklistTerm{}, nil
}

type fakeUserRepo struct {
	mu sync.Mutex

	existing   map[string]domain.User
	getErr     error
	createErr  error
	created    []domain.User
	updated    []domain.User
	updateErr  error
	createResp domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{existing: map[string]domain.User{}, getErr: database.ErrUserNotFound}
}

func (f *fakeUserRepo) Create(user domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, user)
	if f.createErr != nil {
		return domain.User{}, f.createErr
	}
	return user, nil
}

func (f *fakeUserRepo) Update(userId string, newUser domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updated = append(f.updated, newUser)
	return newUser, f.updateErr
}

func (f *fakeUserRepo) GetByNodeID(nodeID string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.existing[nodeID]; ok {
		return u, nil
	}
	return domain.User{}, f.getErr
}

func (f *fakeUserRepo) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created), len(f.updated)
}

type fakeMetrics struct {
	mu      sync.Mutex
	online  []string
	offline []string
}

func (f *fakeMetrics) PushStatusOnline(nodeId string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online = append(f.online, nodeId)
}

func (f *fakeMetrics) PushStatusOffline(nodeId string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offline = append(f.offline, nodeId)
}

func (f *fakeMetrics) snapshot() ([]string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.online...), append([]string(nil), f.offline...)
}

func newService(t *testing.T) (*discoveryService, *fakeNode, *fakeUserRepo, *fakeNodeRepo, *fakeMetrics) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	node := newFakeNode()
	users := newFakeUserRepo()
	nodes := newFakeNodeRepo()
	metrics := &fakeMetrics{}

	s := NewDiscoveryService(ctx, users, nodes, metrics)
	t.Cleanup(s.Close)

	s.node = node
	s.ownId = node.info.ID
	return s, node, users, nodes, metrics
}

func infoJSON(t *testing.T, info warpnet.NodeInfo) []byte {
	t.Helper()
	if info.ID == "" {
		info.ID = warpnet.FromStringToPeerID(peerID)
	}
	bt, err := json.Marshal(info)
	require.NoError(t, err)
	return bt
}

func discovered(id string) discoveredPeer {
	return discoveredPeer{ID: warpnet.FromStringToPeerID(id), Source: sourceGossip}
}

func TestEnqueue_IgnoresSelfAndEmptyPeers(t *testing.T) {
	s, _, _, _, _ := newService(t)

	s.enqueue(warpnet.WarpAddrInfo{}, sourceMDNS)
	s.enqueue(warpnet.WarpAddrInfo{ID: s.ownId}, sourceDHT)

	assert.Empty(t, s.discoveryChan, "a node must never try to discover itself")
}

func TestEnqueue_AcceptsRealPeersFromEverySource(t *testing.T) {
	s, _, _, _, _ := newService(t)
	pi := warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)}

	s.DiscoveryHandlerMDNS(pi)
	s.DiscoveryHandlerPubSub(pi)
	s.DiscoveryHandlerDHT(pi.ID)
	s.DiscoveryHandlerStream(pi)

	assert.Len(t, s.discoveryChan, 4, "every discovery source must feed the same queue")

	sources := map[discoverySource]bool{}
	for len(s.discoveryChan) > 0 {
		sources[(<-s.discoveryChan).Source] = true
	}
	assert.True(t, sources[sourceMDNS])
	assert.True(t, sources[sourceGossip])
	assert.True(t, sources[sourceDHT])
	assert.True(t, sources[sourceStream])
}

func TestEnqueue_RateLimiterShedsFloods(t *testing.T) {
	s, _, _, _, _ := newService(t)

	for i := 0; i < 500; i++ {
		s.enqueue(warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)}, sourceGossip)
	}

	assert.LessOrEqual(t, len(s.discoveryChan), cap(s.discoveryChan),
		"the queue must never exceed its capacity")
}

func TestEnqueue_NilServiceIsInert(t *testing.T) {
	var s *discoveryService
	assert.NotPanics(t, func() {
		s.enqueue(warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)}, sourceMDNS)
	})
}

func TestHandleAsMember_BlocklistedPeerIsNeverDialled(t *testing.T) {
	s, node, users, nodes, metrics := newService(t)
	nodes.blocklisted[peerID] = true

	s.handleAsMember(discovered(peerID))

	connected, streamed, _ := node.snapshot()
	assert.Empty(t, connected, "a blocklisted peer must not be dialled")
	assert.Empty(t, streamed)

	created, _ := users.counts()
	assert.Zero(t, created)

	_, offline := metrics.snapshot()
	assert.Contains(t, offline, peerID)
}

func TestHandleAsMember_BackoffAndDialFailureAreReportedOffline(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"backoff", backoff.ErrBackoffEnabled},
		{"all dials failed", warpnet.ErrAllDialsFailed},
		{"generic failure", errors.New("connection refused")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, node, users, _, metrics := newService(t)
			node.connectErr = tc.err

			s.handleAsMember(discovered(peerID))

			_, streamed, _ := node.snapshot()
			assert.Empty(t, streamed, "an unreachable peer must not be queried")

			created, _ := users.counts()
			assert.Zero(t, created)

			_, offline := metrics.snapshot()
			assert.Contains(t, offline, peerID)
		})
	}
}

func TestHandleAsMember_KnownAliasIsPinnedAndSkipped(t *testing.T) {
	s, node, users, _, metrics := newService(t)

	aliasID := warpnet.FromStringToPeerID(peerID)
	s.aliasCache.Add(aliasID, warpnet.FromStringToPeerID(peerID2))

	s.handleAsMember(discovered(peerID))

	_, streamed, _ := node.snapshot()
	assert.Empty(t, streamed, "a known alias must not be re-interrogated")

	created, _ := users.counts()
	assert.Zero(t, created)

	assert.Contains(t, node.maxPriority, peerID, "an alias of our own node must stay connected")

	online, _ := metrics.snapshot()
	assert.Contains(t, online, peerID)
}

func TestHandleAsMember_RejectsUnusableNodeInfo(t *testing.T) {
	cases := []struct {
		name     string
		infoResp []byte
		infoErr  error
	}{
		{"stream failure", nil, errors.New("offline")},
		{"empty response", nil, nil},
		{"garbage response", []byte("<html>"), nil},
		{"no owner", []byte(`{"node_id":"x"}`), nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, node, users, _, _ := newService(t)
			node.infoResp, node.infoErr = c.infoResp, c.infoErr

			s.handleAsMember(discovered(peerID))

			created, updated := users.counts()
			assert.Zero(t, created, "a peer with unusable info must not create a user")
			assert.Zero(t, updated)
		})
	}
}

func TestHandleAsMember_InfrastructurePeersAreNotUsers(t *testing.T) {
	for _, tc := range []struct {
		name string
		info warpnet.NodeInfo
	}{
		{"relay", warpnet.NodeInfo{OwnerId: "relay-owner", Type: warpnet.RelayNode}},
		{"legacy relay", warpnet.NodeInfo{OwnerId: warpnet.RelayNode}},
		{"moderator", warpnet.NodeInfo{OwnerId: "mod-owner", Type: warpnet.ModeratorNode}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, node, users, _, _ := newService(t)
			node.infoResp = infoJSON(t, tc.info)

			s.handleAsMember(discovered(peerID))

			created, _ := users.counts()
			assert.Zero(t, created, "%s must not be registered as a user", tc.name)
		})
	}
}

func TestHandleAsMember_RegistersGenuinelyNewUser(t *testing.T) {
	s, node, users, _, metrics := newService(t)

	node.infoResp = infoJSON(t, warpnet.NodeInfo{
		OwnerId:      "remote-owner",
		Reachability: warpnet.ReachabilityPublic,
		Aliases:      []warpnet.WarpPeerID{warpnet.FromStringToPeerID(peerID2)},
	})
	node.userResp = []byte(`{"id":"remote-owner","username":"remote"}`)

	s.handleAsMember(discovered(peerID))

	created, _ := users.counts()
	require.Equal(t, 1, created)

	users.mu.Lock()
	got := users.created[0]
	users.mu.Unlock()

	assert.Equal(t, "remote-owner", got.Id)
	assert.Equal(t, peerID, got.NodeId, "the user must be bound to the node we found them on")
	assert.False(t, got.IsOffline, "a peer we just talked to is online")

	_, _, priorities := node.snapshot()
	assert.Equal(t, warpnet.ReachabilityPublic, priorities[peerID])

	online, _ := metrics.snapshot()
	assert.Contains(t, online, peerID)

	assert.True(t, s.aliasCache.Contains(warpnet.FromStringToPeerID(peerID2)))
}

func TestHandleAsMember_KnownOnlineUserIsNotRefetched(t *testing.T) {
	s, node, users, _, _ := newService(t)

	node.infoResp = infoJSON(t, warpnet.NodeInfo{OwnerId: "remote-owner"})
	users.existing[peerID] = domain.User{Id: "remote-owner", IsOffline: false}

	s.handleAsMember(discovered(peerID))

	_, streamed, _ := node.snapshot()
	assert.NotContains(t, streamed, string(event.PUBLIC_GET_USER),
		"a known, online user must not be re-fetched")

	created, _ := users.counts()
	assert.Zero(t, created)
}

func TestHandleAsMember_OfflineUserIsRefreshed(t *testing.T) {
	s, node, users, _, _ := newService(t)

	node.infoResp = infoJSON(t, warpnet.NodeInfo{OwnerId: "remote-owner"})
	node.userResp = []byte(`{"id":"remote-owner","username":"back"}`)
	users.existing[peerID] = domain.User{Id: "remote-owner", IsOffline: true}

	s.handleAsMember(discovered(peerID))

	_, streamed, _ := node.snapshot()
	assert.Contains(t, streamed, string(event.PUBLIC_GET_USER),
		"a peer that came back must be re-read")
}

func TestHandleAsMember_ExistingUserIsUpdatedNotDuplicated(t *testing.T) {
	s, node, users, _, _ := newService(t)

	node.infoResp = infoJSON(t, warpnet.NodeInfo{OwnerId: "remote-owner"})
	node.userResp = []byte(`{"id":"remote-owner","username":"remote"}`)
	users.createErr = database.ErrUserAlreadyExists

	s.handleAsMember(discovered(peerID))

	created, updated := users.counts()
	assert.Equal(t, 1, created, "creation is attempted once")
	assert.Equal(t, 1, updated, "a duplicate must fall through to an update")
}

func TestHandleAsMember_UnreadableUserPayloadIsDropped(t *testing.T) {
	s, node, users, _, _ := newService(t)

	node.infoResp = infoJSON(t, warpnet.NodeInfo{OwnerId: "remote-owner"})
	node.userResp = []byte("not json")

	s.handleAsMember(discovered(peerID))

	created, _ := users.counts()
	assert.Zero(t, created, "a peer that answers with garbage must not create a user")
}

func TestHandleAsMember_NilDependenciesAreInert(t *testing.T) {
	var s *discoveryService
	assert.NotPanics(t, func() { s.handleAsMember(discovered(peerID)) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bare := NewDiscoveryService(ctx, nil, nil, &fakeMetrics{})
	defer bare.Close()
	assert.NotPanics(t, func() { bare.handleAsMember(discovered(peerID)) })
}

func TestHandleAsRelay_SkipsSelfAndEmptyPeers(t *testing.T) {
	s, node, _, _, _ := newService(t)

	s.handleAsRelay(discoveredPeer{})
	s.handleAsRelay(discoveredPeer{ID: s.ownId})

	connected, _, _ := node.snapshot()
	assert.Empty(t, connected)
}

func TestHandleAsRelay_ConnectsAndRecordsPriority(t *testing.T) {
	s, node, _, _, metrics := newService(t)
	node.infoResp = infoJSON(t, warpnet.NodeInfo{
		OwnerId:      "member-owner",
		Reachability: warpnet.ReachabilityPrivate,
		Aliases:      []warpnet.WarpPeerID{warpnet.FromStringToPeerID(peerID2)},
	})

	s.handleAsRelay(discovered(peerID))

	connected, _, priorities := node.snapshot()
	assert.Contains(t, connected, peerID)
	assert.Equal(t, warpnet.ReachabilityPrivate, priorities[peerID])

	online, _ := metrics.snapshot()
	assert.Contains(t, online, peerID)

	assert.True(t, s.aliasCache.Contains(warpnet.FromStringToPeerID(peerID2)))
}

func TestHandleAsRelay_UnreachablePeerIsReportedOffline(t *testing.T) {
	s, node, _, _, metrics := newService(t)
	node.connectErr = warpnet.ErrAllDialsFailed

	s.handleAsRelay(discovered(peerID))

	_, offline := metrics.snapshot()
	assert.Contains(t, offline, peerID)

	_, streamed, _ := node.snapshot()
	assert.Empty(t, streamed)
}

func TestHandleAsRelay_KnownAliasSkipsInterrogation(t *testing.T) {
	s, node, _, _, _ := newService(t)
	s.aliasCache.Add(warpnet.FromStringToPeerID(peerID), warpnet.FromStringToPeerID(peerID2))

	s.handleAsRelay(discovered(peerID))

	_, streamed, _ := node.snapshot()
	assert.Empty(t, streamed)
}

func TestHandleAsRelay_NilServiceIsInert(t *testing.T) {
	var s *discoveryService
	assert.NotPanics(t, func() { s.handleAsRelay(discovered(peerID)) })
}

func TestHandleAsModerator_IsObserveOnly(t *testing.T) {
	s, node, users, _, _ := newService(t)

	s.handleAsModerator(discovered(peerID))

	connected, streamed, _ := node.snapshot()
	assert.Empty(t, connected, "a moderator only observes discoveries")
	assert.Empty(t, streamed)

	created, _ := users.counts()
	assert.Zero(t, created)
}

func TestRequestNodeUser_RejectsEmptyUserAndBadPayloads(t *testing.T) {
	s, node, _, _, _ := newService(t)
	pi := warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)}

	_, err := s.requestNodeUser(pi, "")
	assert.Error(t, err, "a node with no owner has no user to fetch")

	node.userErr = errors.New("offline")
	_, err = s.requestNodeUser(pi, "owner")
	assert.Error(t, err)

	node.userErr = nil
	node.userResp = nil
	_, err = s.requestNodeUser(pi, "owner")
	assert.Error(t, err)

	node.userResp = []byte("not json")
	_, err = s.requestNodeUser(pi, "owner")
	assert.Error(t, err)
}

func TestRequestNodeUser_StampsNodeIdentityAndRTT(t *testing.T) {
	s, node, _, _, _ := newService(t)
	node.userResp = []byte(`{"id":"owner","username":"u","node_id":"someone-elses-node","isOffline":true}`)

	pi := warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)}
	user, err := s.requestNodeUser(pi, "owner")
	require.NoError(t, err)

	assert.Equal(t, peerID, user.NodeId, "the node id must come from the connection, not the body")
	assert.False(t, user.IsOffline, "a peer that just answered is online")
	assert.GreaterOrEqual(t, user.RoundTripTime, int64(0))
}

func TestRun_RoutesByNodeRoleAndStopsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node := newFakeNode()
	node.info.Type = warpnet.RelayNode
	users := newFakeUserRepo()
	nodes := newFakeNodeRepo()
	metrics := &fakeMetrics{}

	s := NewDiscoveryService(ctx, users, nodes, metrics)
	require.NoError(t, s.Run(node))

	s.DiscoveryHandlerPubSub(warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)})

	require.Eventually(t, func() bool {
		connected, _, _ := node.snapshot()
		return len(connected) > 0
	}, 10*time.Second, 20*time.Millisecond, "the relay path must handle the discovery")

	s.Close()
	assert.NotPanics(t, s.Close, "closing twice must not panic")
}

func TestRun_RejectsUninitialisedService(t *testing.T) {
	s := &discoveryService{}
	assert.Error(t, s.Run(newFakeNode()), "a service with no queue cannot run")
}

func TestRelayDiscoveryService_HasNoUserRepositories(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewRelayDiscoveryService(ctx, &fakeMetrics{})
	defer s.Close()

	assert.Nil(t, s.userRepo, "a relay stores no users")
	assert.Nil(t, s.nodeRepo)
	assert.NotNil(t, s.discoveryChan)
}
