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
package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/mdns"
	corenode "github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/require"
)

func testDB(t *testing.T) *local_store.DB {
	t.Helper()
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("test", "test"))
	t.Cleanup(db.Close)
	return db
}

func testKeyAndID(t *testing.T) (ed25519.PrivateKey, warpnet.WarpPeerID) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	return priv, id
}

// newTestMemberNode builds the node without starting it: NewMemberNode wires
// every service up, Start is what binds sockets.
func newTestMemberNode(t *testing.T) (*MemberNode, *local_store.DB, *database.AuthRepo) {
	t.Helper()

	db := testDB(t)
	authRepo := database.NewAuthRepo(db, "testnet")
	require.NoError(t, authRepo.Authenticate("test", "test"))
	_, err := authRepo.SetOwner(domain.Owner{UserId: "owner-1", Username: "owner"})
	require.NoError(t, err)

	return newMemberNodeOn(t, db, authRepo), db, authRepo
}

// newMemberNodeOn raises a node over a store that is already unlocked.
func newMemberNodeOn(t *testing.T, db *local_store.DB, authRepo *database.AuthRepo) *MemberNode {
	t.Helper()

	privKey, ownNodeId := testKeyAndID(t)
	psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
	require.NoError(t, err)

	m, err := NewMemberNode(context.Background(), privKey, psk, ownNodeId, authRepo, db, nil)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.mdnsService, "NewMemberNode must wire multicast discovery")
	m.mdnsService = noopMDNS{}
	t.Cleanup(m.Stop)
	return m
}

// noopMDNS stands in for multicast discovery. These tests stop a node the
// moment it is up, which no desktop does - a logout follows a login by a
// human - and libp2p's service sets its socket up inside Start while Close
// reads what that writes, so the real one would be closed mid-start. What it
// does on its own is core/mdns's to cover.
type noopMDNS struct{}

func (noopMDNS) Start(mdns.NodeConnector) {}
func (noopMDNS) Close()                   {}

func TestNewMemberNodeRequiresPrivateKey(t *testing.T) {
	db := testDB(t)
	authRepo := database.NewAuthRepo(db, "testnet")
	require.NoError(t, authRepo.Authenticate("test", "test"))

	_, err := NewMemberNode(context.Background(), nil, nil, "", authRepo, db, nil)
	require.ErrorIs(t, err, corenode.ErrPrivateKeyRequired)
}

func TestNewMemberNodeWiresServices(t *testing.T) {
	m, _, _ := newTestMemberNode(t)

	require.NotNil(t, m.discService)
	require.NotNil(t, m.mdnsService)
	require.NotNil(t, m.pubsubService)
	require.NotNil(t, m.dHashTable)
	require.NotNil(t, m.nodeRepo)
	require.NotNil(t, m.userRepo)
	require.Equal(t, "owner-1", m.ownerId)
	require.NotEmpty(t, m.opts)

	// The mastodon entry account is seeded so the bridge is reachable.
	entry, err := m.userRepo.Get("warpnet@mastodon.social")
	require.NoError(t, err)
	require.Equal(t, "mastodon", entry.Network)
}

func TestNewMemberNodeCarriesFollowings(t *testing.T) {
	db := testDB(t)
	authRepo := database.NewAuthRepo(db, "testnet")
	require.NoError(t, authRepo.Authenticate("test", "test"))
	_, err := authRepo.SetOwner(domain.Owner{UserId: "owner-1"})
	require.NoError(t, err)

	followRepo := database.NewFollowRepo(db)
	require.NoError(t, followRepo.Follow("owner-1", "followed-1"))

	privKey, ownNodeId := testKeyAndID(t)
	psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
	require.NoError(t, err)

	m, err := NewMemberNode(context.Background(), privKey, psk, ownNodeId, authRepo, db, nil)
	require.NoError(t, err)
	t.Cleanup(m.Stop)
}

func TestFetchFollowingIds(t *testing.T) {
	t.Run("nil repo yields nothing", func(t *testing.T) {
		ids, err := fetchFollowingIds("owner-1", nil)
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("skips the owner and pages through", func(t *testing.T) {
		repo := &pagedFollowStore{pages: [][]domain.ID{
			// a full page (limit is 20) forces a second round
			append(idRange(19), "owner-1"),
			{"late-1"},
		}}
		ids, err := fetchFollowingIds("owner-1", repo)
		require.NoError(t, err)
		require.NotContains(t, ids, domain.ID("owner-1"))
		require.Contains(t, ids, "late-1")
		require.Len(t, ids, 20)
	})

	t.Run("a lookup failure surfaces", func(t *testing.T) {
		repo := &pagedFollowStore{err: errors.New("store down")}
		_, err := fetchFollowingIds("owner-1", repo)
		require.Error(t, err)
	})
}

func idRange(n int) []domain.ID {
	out := make([]domain.ID, 0, n)
	for i := range n {
		out = append(out, domain.ID(string(rune('a'+i%26)))+domain.ID(rune('0'+i/26)))
	}
	return out
}

// pagedFollowStore serves a fixed sequence of following pages.
type pagedFollowStore struct {
	pages [][]domain.ID
	calls int
	err   error
}

func (p *pagedFollowStore) GetFollowings(_ string, _ *uint64, _ *string) ([]domain.ID, string, error) {
	if p.err != nil {
		return nil, "", p.err
	}
	if p.calls >= len(p.pages) {
		return nil, "", nil
	}
	page := p.pages[p.calls]
	p.calls++
	return page, "cursor", nil
}

func (p *pagedFollowStore) GetFollowersCount(string) (uint64, error)  { return 0, nil }
func (p *pagedFollowStore) GetFollowingsCount(string) (uint64, error) { return 0, nil }
func (p *pagedFollowStore) Follow(string, string) error               { return nil }
func (p *pagedFollowStore) Unfollow(string, string) error             { return nil }
func (p *pagedFollowStore) GetFollowers(string, *uint64, *string) ([]domain.ID, string, error) {
	return nil, "", nil
}
func (p *pagedFollowStore) IsFollowing(string, string) bool          { return false }
func (p *pagedFollowStore) IsFollower(string, string) bool           { return false }
func (p *pagedFollowStore) AddFollowRequest(string, string) error    { return nil }
func (p *pagedFollowStore) RemoveFollowRequest(string, string) error { return nil }
func (p *pagedFollowStore) ListFollowRequests(string, *uint64, *string) ([]domain.ID, string, error) {
	return nil, "", nil
}

// TestHandlerListsAreComplete builds every per-feature handler list. They are
// pure route tables, so this both covers them and guards against a duplicate
// route silently shadowing another.
func TestHandlerListsAreComplete(t *testing.T) {
	m, db, authRepo := newTestMemberNode(t)

	statsDB := database.NewStatsRepo(db)
	r := &memberRepos{
		timelineRepo:     database.NewTimelineRepo(db),
		tweetRepo:        database.NewTweetRepo(db, nil),
		reactionRepo:     database.NewReactionRepo(db, nil),
		pollRepo:         database.NewPollRepo(db, nil),
		chatRepo:         database.NewChatRepo(db),
		mediaRepo:        database.NewMediaRepo(db),
		notificationRepo: database.NewNotificationsRepo(db),
		settingsRepo:     database.NewSettingsRepo(db),
		bookmarkRepo:     database.NewBookmarkRepo(db),
		blocksRepo:       database.NewBlocksRepo(db),
		mutesRepo:        database.NewMutesRepo(db),
		subsRepo:         database.NewSubscriptionsRepo(db),
		filterRepo:       database.NewFilterRepo(db),
	}
	_ = statsDB

	followRepo := database.NewFollowRepo(db)
	userRepo := database.NewUserRepo(db)

	lists := map[string][]warpnet.WarpStreamHandler{
		"admin":         m.adminHandlers(authRepo, db, r),
		"tweet":         m.tweetHandlers(authRepo, userRepo, r),
		"engagement":    m.engagementHandlers(userRepo, r),
		"follow":        m.followHandlers(authRepo, userRepo, followRepo),
		"followRequest": m.followRequestHandlers(followRepo),
		"filter":        m.filterHandlers(r),
		"user":          m.userHandlers(authRepo, userRepo, followRepo, r),
		"chat":          m.chatHandlers(authRepo, userRepo, r),
		"media":         m.mediaHandlers(userRepo, r),
		"notification":  m.notificationHandlers(authRepo, r),
		"settings":      m.settingsHandlers(authRepo, r),
		"socialFilter":  m.socialFilterHandlers(userRepo, r),
		"bookmarks":     m.bookmarksHandlers(r),
	}

	seen := map[string]string{}
	total := 0
	for group, hs := range lists {
		require.NotEmpty(t, hs, "%s handlers must not be empty", group)
		for _, h := range hs {
			require.NotNil(t, h.Handler, "%s: %s has no handler", group, h.Path)
			if prev, ok := seen[string(h.Path)]; ok {
				t.Fatalf("route %s registered by both %s and %s", h.Path, prev, group)
			}
			seen[string(h.Path)] = group
			total++
		}
	}
	require.Greater(t, total, 50, "the member node exposes the full route table")
}

func TestSetupHandlersRejectsNilNode(t *testing.T) {
	var m *MemberNode
	require.Panics(t, func() { m.setupHandlers(nil, nil, nil, nil, nil) })
}

func TestAccessorsAreNilSafeBeforeStart(t *testing.T) {
	m, _, _ := newTestMemberNode(t)

	// Start has not run, so the libp2p node is absent: every accessor must
	// answer rather than dereference it.
	require.Nil(t, m.Node())
	require.Nil(t, m.Peerstore())
	require.Nil(t, m.Network())
	require.Nil(t, m.Addrs())
	require.NoError(t, m.Connect(warpnet.WarpAddrInfo{}))

	var nilNode *MemberNode
	require.Nil(t, nilNode.Node())
	require.Nil(t, nilNode.Peerstore())
	require.Nil(t, nilNode.Network())
	require.Nil(t, nilNode.Addrs())
	require.NoError(t, nilNode.Connect(warpnet.WarpAddrInfo{}))
	require.NotPanics(t, nilNode.Stop)
}

func TestSetUserOffline(t *testing.T) {
	m, db, _ := newTestMemberNode(t)

	userRepo := database.NewUserRepo(db)
	_, err := userRepo.Create(domain.User{Id: "user-1", NodeId: "12D3KooWOfflineNode"})
	require.NoError(t, err)

	// unknown node: nothing to flag, and no panic
	require.NotPanics(t, func() { m.setUserOffline("12D3KooWUnknownNode") })

	m.setUserOffline("12D3KooWOfflineNode")
	got, err := userRepo.Get("user-1")
	require.NoError(t, err)
	require.True(t, got.IsOffline)

	// already offline: the second call is a no-op
	require.NotPanics(t, func() { m.setUserOffline("12D3KooWOfflineNode") })
}

// TestNodeInfoAliasesAreDecodablePeerIDs pins the wire form of a paired
// device. A peer ID holds the binary multihash, so the stored text has to be
// decoded: converted, it reaches peers base58-encoded twice and every one of
// them drops the whole NodeInfo.
func TestNodeInfoAliasesAreDecodablePeerIDs(t *testing.T) {
	m, db, _ := newTestMemberNode(t)
	aliases := database.NewAliasesRepo(db)

	_, phone := testKeyAndID(t)
	_, tablet := testKeyAndID(t)
	require.NoError(t, aliases.SetAlias(domain.Alias{NodeId: phone.String(), Token: "phone-token"}))
	require.NoError(t, aliases.SetAlias(domain.Alias{NodeId: tablet.String(), Token: "tablet-token"}))
	// a record that is no peer id at all has no business on the wire
	require.NoError(t, aliases.SetAlias(domain.Alias{NodeId: "not-a-peer-id", Token: "junk-token"}))

	info := m.NodeInfo()
	require.ElementsMatch(t, warpnet.AliasIDs{phone, tablet}, info.Aliases)

	// the node is not started, so it carries no id of its own yet
	_, info.ID = testKeyAndID(t)

	data, err := json.Marshal(info)
	require.NoError(t, err)
	require.Contains(t, string(data), phone.String(), "a device goes out as its own peer id text")

	var decoded warpnet.NodeInfo
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.ElementsMatch(t, warpnet.AliasIDs{phone, tablet}, decoded.Aliases)
}

func TestNodeInfoAdvertisesOnlyPairedDevices(t *testing.T) {
	m, db, _ := newTestMemberNode(t)
	aliases := database.NewAliasesRepo(db)

	require.Empty(t, m.NodeInfo().Aliases, "a node with no device paired advertises none")

	_, phone := testKeyAndID(t)
	require.NoError(t, aliases.SetAlias(domain.Alias{NodeId: phone.String(), Token: "phone-token"}))
	require.Equal(t, warpnet.AliasIDs{phone}, m.NodeInfo().Aliases)

	require.NoError(t, aliases.DeleteAlias(phone.String()))
	require.Empty(t, m.NodeInfo().Aliases, "an unpaired device stops being advertised")
}

// TestStartBringsUpTheNode exercises the full startup path: libp2p host,
// pubsub, discovery, mDNS, the CRDT stats store and the route table. It binds
// the configured port, so it skips when something else already holds it.
func TestStartBringsUpTheNode(t *testing.T) {
	m, _, _ := newTestMemberNode(t)

	if err := m.Start(); err != nil {
		t.Skipf("cannot bind the configured node port: %v", err)
	}

	info := m.NodeInfo()
	require.Equal(t, warpnet.MemberNode, info.Type)
	require.Equal(t, "owner-1", info.OwnerId)
	require.NotEmpty(t, info.Addresses)

	require.NotNil(t, m.Node())
	require.NotNil(t, m.Peerstore())
	require.NotNil(t, m.Network())
	require.NotNil(t, m.Addrs())

	// priority tuning is a no-op on an unknown peer but must not panic
	_, otherID := testKeyAndID(t)
	require.NotPanics(t, func() {
		m.SetMaxNodePriority(otherID)
		m.SetMinNodePriority(otherID)
		m.SetNodePriority(otherID, warpnet.WarpReachability(0))
	})

	// a stream to an unknown peer fails rather than hanging
	_, err := m.GenericStream(otherID.String(), "/public/get/info", nil)
	require.Error(t, err)
}

// TestASecondNodeCanBeRaisedAfterAStop walks the desktop logout/login cycle:
// the node goes down and the database closes, then the store reopens and a
// fresh node comes up in the same process. Like TestStartBringsUpTheNode it
// binds the configured port, so it skips when something else holds it - but
// once the first node is up, the second one failing is the bug.
func TestASecondNodeCanBeRaisedAfterAStop(t *testing.T) {
	first, db, authRepo := newTestMemberNode(t)
	if err := first.Start(); err != nil {
		t.Skipf("cannot bind the configured node port: %v", err)
	}

	first.Stop() // logout
	db.Close()

	require.NoError(t, db.Run("test", "test")) // login again
	require.NoError(t, authRepo.Authenticate("test", "test"))

	second := newMemberNodeOn(t, db, authRepo)
	require.NoError(t, second.Start(), "signing in again must raise a node in the same process")
	require.NotEmpty(t, second.NodeInfo().Addresses)
}

// The desktop app stops the node twice: logout (PRIVATE_POST_LOGOUT) stops it,
// then the wails shutdown hook (App.close) stops it again when the window
// closes. A second Stop must be a no-op instead of panicking on an
// already closed channel.
func TestStopIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(db.Close)

	authRepo := database.NewAuthRepo(db, "test")
	require.NoError(t, authRepo.Authenticate("test", "test"))

	privKey := authRepo.PrivateKey()
	ownNodeId, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	_, err = authRepo.SetOwner(domain.Owner{
		NodeId:   ownNodeId.String(),
		UserId:   ownNodeId.String(),
		Username: "test",
	})
	require.NoError(t, err)

	n, err := NewMemberNode(ctx, privKey, nil, ownNodeId, authRepo, db, nil)
	require.NoError(t, err)

	n.Stop()
	n.Stop()
}
