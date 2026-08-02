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
package database

import (
	"context"
	"strings"
	"testing"
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type NodeRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *NodeRepo
	ctx  context.Context
}

func (s *NodeRepoTestSuite) SetupSuite() {
	var err error
	s.ctx = context.Background()

	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	auth := NewAuthRepo(s.db, "test")
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewNodeRepo(s.db)

}

func (s *NodeRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *NodeRepoTestSuite) TestPutGetHasDelete() {
	key := datastore.NewKey("test/key")
	value := []byte("hello")

	err := s.repo.Put(s.ctx, key, value)
	s.Require().NoError(err)

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Equal(value, got)

	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.True(has)

	err = s.repo.Delete(s.ctx, key)
	s.Require().NoError(err)

	_, err = s.repo.Get(s.ctx, key)
	s.ErrorIs(err, datastore.ErrNotFound)
}

func (s *NodeRepoTestSuite) TestPutWithTTLAndSetTTL() {
	key := datastore.NewKey("ttl/key")
	value := []byte("expiring")

	// The margin has to survive a loaded CI box: with a 2s TTL and a 1s sleep
	// the key could expire before SetTTL ran.
	err := s.repo.PutWithTTL(s.ctx, key, value, time.Second*30)
	s.Require().NoError(err)

	// overwrite ttl
	time.Sleep(100 * time.Millisecond)
	err = s.repo.SetTTL(s.ctx, key, time.Minute)
	s.Require().NoError(err)

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Equal(value, got)
}

func (s *NodeRepoTestSuite) TestDiskUsage() {
	_, err := s.repo.DiskUsage(s.ctx)
	s.Require().NoError(err)
}

func (s *NodeRepoTestSuite) TestQuerySimple() {
	key := datastore.NewKey("querysimple/key/item")
	val := []byte("qval")
	err := s.repo.Put(s.ctx, key, val)
	s.Require().NoError(err)

	q := query.Query{Prefix: "querysimple/key"}
	results, err := s.repo.Query(s.ctx, q)
	s.Require().NoError(err)
	s.Require().NotNil(results)

	defer func() {
		_ = results.Close()
	}()
	var found bool
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		s.Equal("/querysimple/key/item", r.Entry.Key)
		found = true
		break
	}
	s.True(found)
}

func (s *NodeRepoTestSuite) TestQueryEmptyPrefix() {
	key := datastore.NewKey("all/key")
	val := []byte("all")
	err := s.repo.Put(s.ctx, key, val)
	s.Require().NoError(err)

	results, err := s.repo.Query(s.ctx, query.Query{})
	s.Require().NoError(err)
	s.Require().NotNil(results)

	defer func() {
		_ = results.Close()
	}()

	var found bool
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		if r.Entry.Key == "/all/key" {
			found = true
			break
		}
	}

	s.True(found)
}

func (s *NodeRepoTestSuite) TestQueryPrefixDoesNotMatchSiblingKeys() {
	err := s.repo.Put(s.ctx, datastore.NewKey("query/key/child"), []byte("child"))
	s.Require().NoError(err)
	err = s.repo.Put(s.ctx, datastore.NewKey("query/key2"), []byte("sibling"))
	s.Require().NoError(err)

	results, err := s.repo.Query(s.ctx, query.Query{Prefix: "query/key"})
	s.Require().NoError(err)
	s.Require().NotNil(results)
	defer func() {
		_ = results.Close()
	}()

	keys := make([]string, 0)
	for r := range results.Next() {
		s.Require().NoError(r.Error)
		keys = append(keys, r.Entry.Key)
	}

	s.Contains(keys, "/query/key/child")
	s.NotContains(keys, "/query/key2")
}

func TestNodeRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(NodeRepoTestSuite))
}

// notRunningDB returns a DB that was never authenticated, i.e. IsClosed() == true.
// It models a node whose storage went away underneath libp2p — every datastore
// call must degrade to ErrNotRunning instead of panicking or corrupting state.
func notRunningDB(t *testing.T) *local_store.DB {
	t.Helper()
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.True(t, db.IsClosed())
	return db
}

type NodeRepoDatastoreTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *NodeRepo
	ctx  context.Context
}

func (s *NodeRepoDatastoreTestSuite) SetupSuite() {
	var err error
	s.ctx = context.Background()

	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	auth := NewAuthRepo(s.db, "test")
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewNodeRepo(s.db)
}

func (s *NodeRepoDatastoreTestSuite) TearDownSuite() {
	s.db.Close()
}

// ---------------------------------------------------------------------------
// Blocklist — the abuse-control surface of the network.
// ---------------------------------------------------------------------------

// A peer that keeps misbehaving must climb the ladder and then stay pinned at
// PermanentBlock. If Next() ever wrapped around, a serial abuser would silently
// fall back to a one-hour ban.
func (s *NodeRepoDatastoreTestSuite) TestBlocklistEscalationLadderSaturates() {
	peer := "12D3KooWEscalate"

	expected := []BlockLevel{InitialBlock, MediumBlock, AdvancedBlock, PermanentBlock}
	for i, want := range expected {
		s.Require().NoError(s.repo.BlocklistExponential(peer), "strike %d", i+1)

		term, err := s.repo.BlocklistTerm(peer)
		s.Require().NoError(err)
		s.Equal(want, term.Level, "strike %d should reach level %d", i+1, want)
		s.True(s.repo.IsBlocklisted(peer), "peer must be blocked after strike %d", i+1)
	}

	// Further strikes must not roll the level over.
	for i := 0; i < 5; i++ {
		s.Require().NoError(s.repo.BlocklistExponential(peer))
		term, err := s.repo.BlocklistTerm(peer)
		s.Require().NoError(err)
		s.Equal(PermanentBlock, term.Level, "extra strike %d must stay permanent", i+1)
	}
}

func (s *NodeRepoDatastoreTestSuite) TestBlockLevelNextSaturates() {
	s.Equal(MediumBlock, InitialBlock.Next())
	s.Equal(AdvancedBlock, MediumBlock.Next())
	s.Equal(PermanentBlock, AdvancedBlock.Next())
	s.Equal(PermanentBlock, PermanentBlock.Next())
	// A corrupted/out-of-range level must clamp, never wrap into a shorter ban.
	s.Equal(PermanentBlock, BlockLevel(99).Next())
}

// Unblocking is rehabilitation: the term resets so the peer starts again at
// InitialBlock rather than being instantly re-banned at the old severity.
func (s *NodeRepoDatastoreTestSuite) TestBlocklistRemoveResetsEscalation() {
	peer := "12D3KooWRehab"

	s.Require().NoError(s.repo.BlocklistExponential(peer))
	s.Require().NoError(s.repo.BlocklistExponential(peer))
	term, err := s.repo.BlocklistTerm(peer)
	s.Require().NoError(err)
	s.Equal(MediumBlock, term.Level)

	s.Require().NoError(s.repo.BlocklistRemove(peer))
	s.False(s.repo.IsBlocklisted(peer), "unblock must lift the ban immediately")

	term, err = s.repo.BlocklistTerm(peer)
	s.Require().NoError(err)
	s.Equal(BlockLevel(0), term.Level, "unblock must clear the accumulated term")

	s.Require().NoError(s.repo.BlocklistExponential(peer))
	term, err = s.repo.BlocklistTerm(peer)
	s.Require().NoError(err)
	s.Equal(InitialBlock, term.Level, "a rehabilitated peer restarts at the mildest ban")
}

// A social block ("I never want to see this person") must not expire on its own.
func (s *NodeRepoDatastoreTestSuite) TestBlocklistPermanentIsNotAnEscalationStep() {
	peer := "12D3KooWSocialBlock"

	s.Require().NoError(s.repo.BlocklistPermanent(peer))
	s.True(s.repo.IsBlocklisted(peer))

	term, err := s.repo.BlocklistTerm(peer)
	s.Require().NoError(err)
	s.Equal(PermanentBlock, term.Level)
	s.Equal(peer, term.PeerID)

	// Permanent is idempotent — blocking twice must not throw or downgrade.
	s.Require().NoError(s.repo.BlocklistPermanent(peer))
	term, err = s.repo.BlocklistTerm(peer)
	s.Require().NoError(err)
	s.Equal(PermanentBlock, term.Level)

	// And it must still be liftable by the user who set it.
	s.Require().NoError(s.repo.BlocklistRemove(peer))
	s.False(s.repo.IsBlocklisted(peer))
}

// An escalated ban must be upgradable to a permanent, user-driven block.
func (s *NodeRepoDatastoreTestSuite) TestEscalatedPeerCanBeSociallyBlocked() {
	peer := "12D3KooWUpgrade"

	s.Require().NoError(s.repo.BlocklistExponential(peer))
	s.Require().NoError(s.repo.BlocklistPermanent(peer))

	term, err := s.repo.BlocklistTerm(peer)
	s.Require().NoError(err)
	s.Equal(PermanentBlock, term.Level)
	s.True(s.repo.IsBlocklisted(peer))
}

// Never ban a peer we know nothing about, and never ban "everyone" because an
// empty peer ID slipped through from a malformed wire message.
func (s *NodeRepoDatastoreTestSuite) TestBlocklistRejectsEmptyPeerAndUnknownIsFree() {
	s.Error(s.repo.BlocklistExponential(""))
	s.Error(s.repo.BlocklistPermanent(""))

	_, err := s.repo.BlocklistTerm("")
	s.Error(err)

	s.False(s.repo.IsBlocklisted(""), "empty peer ID must never read as blocked")
	s.False(s.repo.IsBlocklisted("12D3KooWTotallyUnknownStranger"))

	// Removing a peer that was never blocked is a no-op, not an error.
	s.NoError(s.repo.BlocklistRemove(""))
	s.NoError(s.repo.BlocklistRemove("12D3KooWNeverBlocked"))
}

func (s *NodeRepoDatastoreTestSuite) TestBlocklistTermOfUnknownPeerIsZeroValue() {
	term, err := s.repo.BlocklistTerm("12D3KooWFreshFace")
	s.Require().NoError(err)
	s.Require().NotNil(term)
	s.Equal(BlockLevel(0), term.Level)
	s.Empty(term.PeerID)
}

// Bans are per-peer: blocking one abuser must not leak onto a neighbour whose
// ID merely shares a prefix.
func (s *NodeRepoDatastoreTestSuite) TestBlocklistDoesNotLeakAcrossPrefixSiblings() {
	victim := "12D3KooWPrefix"
	sibling := "12D3KooWPrefixSibling"

	s.Require().NoError(s.repo.BlocklistPermanent(victim))
	s.True(s.repo.IsBlocklisted(victim))
	s.False(s.repo.IsBlocklisted(sibling), "prefix sibling must stay unblocked")
}

// ---------------------------------------------------------------------------
// Datastore contract — nil repo / dead DB / cancelled context.
// ---------------------------------------------------------------------------

func (s *NodeRepoDatastoreTestSuite) TestNilRepoNeverPanics() {
	var repo *NodeRepo
	key := datastore.NewKey("nil/key")

	s.ErrorIs(repo.Put(s.ctx, key, []byte("v")), ErrNilNodeRepo)
	s.ErrorIs(repo.PutWithTTL(s.ctx, key, []byte("v"), time.Minute), ErrNilNodeRepo)
	s.ErrorIs(repo.SetTTL(s.ctx, key, time.Minute), ErrNilNodeRepo)
	s.ErrorIs(repo.Delete(s.ctx, key), ErrNilNodeRepo)
	s.ErrorIs(repo.Sync(s.ctx, key), ErrNilNodeRepo)

	_, err := repo.Get(s.ctx, key)
	s.ErrorIs(err, ErrNilNodeRepo)
	_, err = repo.Has(s.ctx, key)
	s.ErrorIs(err, ErrNilNodeRepo)
	_, err = repo.GetSize(s.ctx, key)
	s.ErrorIs(err, ErrNilNodeRepo)
	_, err = repo.GetExpiration(s.ctx, key)
	s.ErrorIs(err, ErrNilNodeRepo)
	_, err = repo.DiskUsage(s.ctx)
	s.ErrorIs(err, ErrNilNodeRepo)
	_, err = repo.Query(s.ctx, query.Query{})
	s.ErrorIs(err, ErrNilNodeRepo)
	_, err = repo.Batch(s.ctx)
	s.ErrorIs(err, ErrNilNodeRepo)

	s.ErrorIs(repo.BlocklistExponential("p"), ErrNilNodeRepo)
	s.ErrorIs(repo.BlocklistPermanent("p"), ErrNilNodeRepo)
	_, err = repo.BlocklistTerm("p")
	s.ErrorIs(err, ErrNilNodeRepo)
	s.False(repo.IsBlocklisted("p"))
	s.NoError(repo.BlocklistRemove("p"))
	s.NoError(repo.Close())
}

func (s *NodeRepoDatastoreTestSuite) TestDeadDatabaseDegradesToErrNotRunning() {
	dead := notRunningDB(s.T())
	repo := NewNodeRepo(dead)
	key := datastore.NewKey("dead/key")

	s.ErrorIs(repo.Put(s.ctx, key, []byte("v")), local_store.ErrNotRunning)
	s.ErrorIs(repo.PutWithTTL(s.ctx, key, []byte("v"), time.Minute), local_store.ErrNotRunning)
	s.ErrorIs(repo.SetTTL(s.ctx, key, time.Minute), local_store.ErrNotRunning)
	s.ErrorIs(repo.Delete(s.ctx, key), local_store.ErrNotRunning)
	s.ErrorIs(repo.Sync(s.ctx, key), local_store.ErrNotRunning)

	_, err := repo.Get(s.ctx, key)
	s.ErrorIs(err, local_store.ErrNotRunning)
	_, err = repo.Has(s.ctx, key)
	s.ErrorIs(err, local_store.ErrNotRunning)
	_, err = repo.GetSize(s.ctx, key)
	s.ErrorIs(err, local_store.ErrNotRunning)
	_, err = repo.GetExpiration(s.ctx, key)
	s.ErrorIs(err, local_store.ErrNotRunning)
	_, err = repo.DiskUsage(s.ctx)
	s.ErrorIs(err, local_store.ErrNotRunning)
	_, err = repo.Query(s.ctx, query.Query{})
	s.ErrorIs(err, local_store.ErrNotRunning)
	_, err = repo.Batch(s.ctx)
	s.ErrorIs(err, local_store.ErrNotRunning)

	// Closing a repo whose DB already died must be a silent no-op.
	s.NoError(repo.Close())
}

// A cancelled context must abort every call before it touches storage —
// otherwise a shutting-down node keeps writing peer records.
func (s *NodeRepoDatastoreTestSuite) TestCancelledContextAbortsEveryCall() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	key := datastore.NewKey("cancelled/key")

	s.ErrorIs(s.repo.Put(ctx, key, []byte("v")), context.Canceled)
	s.ErrorIs(s.repo.PutWithTTL(ctx, key, []byte("v"), time.Minute), context.Canceled)
	s.ErrorIs(s.repo.SetTTL(ctx, key, time.Minute), context.Canceled)
	s.ErrorIs(s.repo.Delete(ctx, key), context.Canceled)
	s.ErrorIs(s.repo.Sync(ctx, key), context.Canceled)

	_, err := s.repo.Get(ctx, key)
	s.ErrorIs(err, context.Canceled)
	_, err = s.repo.Has(ctx, key)
	s.ErrorIs(err, context.Canceled)
	_, err = s.repo.GetSize(ctx, key)
	s.ErrorIs(err, context.Canceled)
	_, err = s.repo.GetExpiration(ctx, key)
	s.ErrorIs(err, context.Canceled)
	_, err = s.repo.DiskUsage(ctx)
	s.ErrorIs(err, context.Canceled)
	_, err = s.repo.Query(ctx, query.Query{})
	s.ErrorIs(err, context.Canceled)
	_, err = s.repo.Batch(ctx)
	s.ErrorIs(err, context.Canceled)

	// Nothing may have been written by the aborted calls.
	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.False(has)
}

// ---------------------------------------------------------------------------
// Missing keys, sizes and expirations.
// ---------------------------------------------------------------------------

func (s *NodeRepoDatastoreTestSuite) TestMissingKeyReportsDatastoreErrNotFound() {
	missing := datastore.NewKey("definitely/not/here")

	_, err := s.repo.Get(s.ctx, missing)
	s.ErrorIs(err, datastore.ErrNotFound)

	_, err = s.repo.GetSize(s.ctx, missing)
	s.ErrorIs(err, datastore.ErrNotFound)

	_, err = s.repo.GetExpiration(s.ctx, missing)
	s.ErrorIs(err, datastore.ErrNotFound)

	// SetTTL on a key that does not exist must not resurrect it as empty.
	s.ErrorIs(s.repo.SetTTL(s.ctx, missing, time.Minute), datastore.ErrNotFound)

	has, err := s.repo.Has(s.ctx, missing)
	s.Require().NoError(err)
	s.False(has)

	// Deleting a missing key is idempotent.
	s.NoError(s.repo.Delete(s.ctx, missing))
}

func (s *NodeRepoDatastoreTestSuite) TestGetSizeMatchesStoredPayload() {
	key := datastore.NewKey("size/key")
	value := []byte(strings.Repeat("x", 4096))
	s.Require().NoError(s.repo.Put(s.ctx, key, value))

	size, err := s.repo.GetSize(s.ctx, key)
	s.Require().NoError(err)
	s.Equal(len(value), size)
}

func (s *NodeRepoDatastoreTestSuite) TestEmptyValueRoundTripsAsPresentNotMissing() {
	key := datastore.NewKey("empty/value")
	s.Require().NoError(s.repo.Put(s.ctx, key, []byte{}))

	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.True(has, "a zero-length value is still a present record")

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Empty(got)

	size, err := s.repo.GetSize(s.ctx, key)
	s.Require().NoError(err)
	s.Zero(size)
}

func (s *NodeRepoDatastoreTestSuite) TestExpirationIsZeroWithoutTTLAndSetWithIt() {
	noTTL := datastore.NewKey("expiry/none")
	s.Require().NoError(s.repo.Put(s.ctx, noTTL, []byte("v")))
	exp, err := s.repo.GetExpiration(s.ctx, noTTL)
	s.Require().NoError(err)
	s.Equal(time.Unix(0, 0), exp, "a key without TTL must not claim a real expiry")

	withTTL := datastore.NewKey("expiry/some")
	s.Require().NoError(s.repo.PutWithTTL(s.ctx, withTTL, []byte("v"), time.Hour))
	exp, err = s.repo.GetExpiration(s.ctx, withTTL)
	s.Require().NoError(err)
	s.WithinDuration(time.Now().Add(time.Hour), exp, time.Minute)
}

// The peerstore writes keys that contain ':' and long base32 blobs — exactly the
// shape called out in the node-repo prefix comment. They must survive a round trip.
func (s *NodeRepoDatastoreTestSuite) TestAwkwardKeysRoundTrip() {
	awkward := []string{
		"peers/keys/AASAQAISEAXNRKHMX2O3AA26JM7NGIWUPOGIITJ2UHHXGX4OWIEKPNAW6YCSK/priv",
		"peers/addrs/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j",
		"weird/key:with:colons",
		"unicode/ключ/ключ",
		"emoji/🔥/value",
		"dots/../traversal",
		"trailing/space ",
	}

	for _, raw := range awkward {
		key := datastore.NewKey(raw)
		value := []byte("payload:" + raw)

		s.Require().NoErrorf(s.repo.Put(s.ctx, key, value), "put %q", raw)

		got, err := s.repo.Get(s.ctx, key)
		s.Require().NoErrorf(err, "get %q", raw)
		s.Equalf(value, got, "round trip %q", raw)

		s.Require().NoErrorf(s.repo.Delete(s.ctx, key), "delete %q", raw)
	}
}

// ---------------------------------------------------------------------------
// Batch semantics.
// ---------------------------------------------------------------------------

func (s *NodeRepoDatastoreTestSuite) TestBatchIsInvisibleUntilCommit() {
	key := datastore.NewKey("batch/pending")

	b, err := s.repo.Batch(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(b.Put(s.ctx, key, []byte("staged")))

	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.False(has, "uncommitted batch writes must not be readable")

	s.Require().NoError(b.Commit(s.ctx))

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Equal([]byte("staged"), got)
}

func (s *NodeRepoDatastoreTestSuite) TestBatchCancelDiscardsEverything() {
	key := datastore.NewKey("batch/discarded")

	b, err := s.repo.Batch(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(b.Put(s.ctx, key, []byte("never")))

	cancellable, ok := b.(interface{ Cancel() error })
	s.Require().True(ok)
	s.Require().NoError(cancellable.Cancel())

	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.False(has)

	// Cancel is idempotent, and a cancelled batch refuses further work
	// instead of writing into a nil write-batch.
	s.NoError(cancellable.Cancel())
	s.ErrorIs(b.Put(s.ctx, key, []byte("zombie")), ErrNilNodeRepo)
	s.ErrorIs(b.Delete(s.ctx, key), ErrNilNodeRepo)
	s.ErrorIs(b.Commit(s.ctx), ErrNilNodeRepo)
}

func (s *NodeRepoDatastoreTestSuite) TestBatchDeleteAndReuseAfterCommit() {
	key := datastore.NewKey("batch/delete")
	s.Require().NoError(s.repo.Put(s.ctx, key, []byte("doomed")))

	b, err := s.repo.Batch(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(b.Delete(s.ctx, key))
	s.Require().NoError(b.Commit(s.ctx))

	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.False(has)

	// A committed batch is spent: reusing it must error, not silently drop writes.
	s.ErrorIs(b.Put(s.ctx, key, []byte("resurrect")), ErrNilNodeRepo)
	s.ErrorIs(b.Commit(s.ctx), ErrNilNodeRepo)
}

func (s *NodeRepoDatastoreTestSuite) TestBatchRespectsCancelledContext() {
	key := datastore.NewKey("batch/ctx")

	b, err := s.repo.Batch(s.ctx)
	s.Require().NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.ErrorIs(b.Put(ctx, key, []byte("v")), context.Canceled)
	s.ErrorIs(b.Delete(ctx, key), context.Canceled)
	s.ErrorIs(b.Commit(ctx), context.Canceled)

	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.False(has)
}

func (s *NodeRepoDatastoreTestSuite) TestBatchLastWriteWins() {
	key := datastore.NewKey("batch/overwrite")

	b, err := s.repo.Batch(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(b.Put(s.ctx, key, []byte("first")))
	s.Require().NoError(b.Put(s.ctx, key, []byte("second")))
	s.Require().NoError(b.Commit(s.ctx))

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Equal([]byte("second"), got)
}

// ---------------------------------------------------------------------------
// Query semantics — the surface DHT/peerstore scans rely on.
// ---------------------------------------------------------------------------

func (s *NodeRepoDatastoreTestSuite) seedQuerySet(prefix string, n int) {
	s.T().Helper()
	for i := 0; i < n; i++ {
		key := datastore.NewKey(prefix + "/" + string(rune('a'+i)))
		s.Require().NoError(s.repo.Put(s.ctx, key, []byte{byte('0' + i)}))
	}
}

func (s *NodeRepoDatastoreTestSuite) collect(q query.Query) []query.Entry {
	s.T().Helper()
	res, err := s.repo.Query(s.ctx, q)
	s.Require().NoError(err)
	s.Require().NotNil(res)
	defer func() { _ = res.Close() }()

	entries := make([]query.Entry, 0)
	for r := range res.Next() {
		s.Require().NoError(r.Error)
		entries = append(entries, r.Entry)
	}
	return entries
}

func (s *NodeRepoDatastoreTestSuite) TestQueryKeysOnlyOmitsValuesButKeepsSize() {
	s.seedQuerySet("keysonly", 3)

	entries := s.collect(query.Query{Prefix: "keysonly", KeysOnly: true})
	s.Require().Len(entries, 3)
	for _, e := range entries {
		s.Empty(e.Value, "KeysOnly must not ship payloads over the wire")
		s.Equal(1, e.Size)
	}
}

func (s *NodeRepoDatastoreTestSuite) TestQueryLimitAndOffsetPaginateWithoutGaps() {
	s.seedQuerySet("paging", 6)

	all := s.collect(query.Query{Prefix: "paging"})
	s.Require().Len(all, 6)

	first := s.collect(query.Query{Prefix: "paging", Limit: 2})
	s.Require().Len(first, 2)
	s.Equal(all[0].Key, first[0].Key)
	s.Equal(all[1].Key, first[1].Key)

	second := s.collect(query.Query{Prefix: "paging", Offset: 2, Limit: 2})
	s.Require().Len(second, 2)
	s.Equal(all[2].Key, second[0].Key)
	s.Equal(all[3].Key, second[1].Key)

	// An offset past the end yields nothing rather than wrapping around.
	beyond := s.collect(query.Query{Prefix: "paging", Offset: 999})
	s.Empty(beyond)

	// A limit larger than the result set is not an error.
	over := s.collect(query.Query{Prefix: "paging", Limit: 100})
	s.Len(over, 6)
}

func (s *NodeRepoDatastoreTestSuite) TestQueryDescendingOrderIsExactReverse() {
	s.seedQuerySet("descending", 4)

	asc := s.collect(query.Query{Prefix: "descending"})
	desc := s.collect(query.Query{
		Prefix: "descending",
		Orders: []query.Order{query.OrderByKeyDescending{}},
	})

	s.Require().Len(desc, len(asc))
	for i := range asc {
		s.Equal(asc[len(asc)-1-i].Key, desc[i].Key)
	}
}

// An order the storage layer cannot push down must fall back to the naive
// in-memory sort instead of silently returning unordered rows.
func (s *NodeRepoDatastoreTestSuite) TestQueryUnsupportedOrderFallsBackToNaiveSort() {
	prefix := "naiveorder"
	s.Require().NoError(s.repo.Put(s.ctx, datastore.NewKey(prefix+"/a"), []byte("ccc")))
	s.Require().NoError(s.repo.Put(s.ctx, datastore.NewKey(prefix+"/b"), []byte("aaa")))
	s.Require().NoError(s.repo.Put(s.ctx, datastore.NewKey(prefix+"/c"), []byte("bbb")))

	entries := s.collect(query.Query{
		Prefix: prefix,
		Orders: []query.Order{query.OrderByValue{}},
	})

	s.Require().Len(entries, 3)
	s.Equal([]byte("aaa"), entries[0].Value)
	s.Equal([]byte("bbb"), entries[1].Value)
	s.Equal([]byte("ccc"), entries[2].Value)
}

func (s *NodeRepoDatastoreTestSuite) TestQueryFilterExcludesNonMatchingKeys() {
	prefix := "filtered"
	s.Require().NoError(s.repo.Put(s.ctx, datastore.NewKey(prefix+"/keep"), []byte("1")))
	s.Require().NoError(s.repo.Put(s.ctx, datastore.NewKey(prefix+"/drop"), []byte("2")))

	entries := s.collect(query.Query{
		Prefix: prefix,
		Filters: []query.Filter{
			query.FilterKeyCompare{Op: query.Equal, Key: "/" + prefix + "/keep"},
		},
	})

	s.Require().Len(entries, 1)
	s.Equal("/"+prefix+"/keep", entries[0].Key)
}

func (s *NodeRepoDatastoreTestSuite) TestQueryReturnExpirationsSurfacesTTL() {
	prefix := "expiring"
	s.Require().NoError(s.repo.PutWithTTL(s.ctx, datastore.NewKey(prefix+"/a"), []byte("v"), time.Hour))

	entries := s.collect(query.Query{Prefix: prefix, ReturnExpirations: true})
	s.Require().Len(entries, 1)
	s.WithinDuration(time.Now().Add(time.Hour), entries[0].Expiration, time.Minute)
}

func (s *NodeRepoDatastoreTestSuite) TestQueryOnEmptyPrefixNamespaceIsEmptyNotAll() {
	entries := s.collect(query.Query{Prefix: "no/such/namespace/at/all"})
	s.Empty(entries, "an unknown prefix must not fall back to a full scan")
}

// Query results carry datastore-relative keys, never the internal NODES prefix —
// leaking it would corrupt every key the DHT writes back.
func (s *NodeRepoDatastoreTestSuite) TestQueryKeysAreStripedOfInternalNamespace() {
	s.Require().NoError(s.repo.Put(s.ctx, datastore.NewKey("stripped/key"), []byte("v")))

	entries := s.collect(query.Query{Prefix: "stripped"})
	s.Require().Len(entries, 1)
	s.Equal("/stripped/key", entries[0].Key)
	s.NotContains(entries[0].Key, nodesPrefix)
}

func (s *NodeRepoDatastoreTestSuite) TestResultKeyFromStorageKeyEdgeCases() {
	s.Equal(requiredPrefixSlash, s.repo.resultKeyFromStorageKey(s.repo.prefix))
	s.Equal("/peers/x", s.repo.resultKeyFromStorageKey(s.repo.prefix+"/peers/x"))
	// A key that does not belong to us is returned untouched rather than mangled.
	s.Equal("/OTHER/peers/x", s.repo.resultKeyFromStorageKey("/OTHER/peers/x"))
}

func (s *NodeRepoDatastoreTestSuite) TestStorageQueryPrefixAlwaysTerminatesWithSlash() {
	s.Equal("/NODES/", string(s.repo.storageQueryPrefix("")))
	s.Equal("/NODES/peers/", string(s.repo.storageQueryPrefix("peers")))
	s.Equal("/NODES/peers/", string(s.repo.storageQueryPrefix("/peers")))
	// Without the trailing slash "/peers" would also match "/peersomething".
	s.Equal("/NODES/peers/", string(s.repo.storageQueryPrefix("/peers/")))
}

func (s *NodeRepoDatastoreTestSuite) TestBuildRootKeyKeepsRootSlash() {
	s.Equal("peers/x", buildRootKey(datastore.NewKey("/peers/x")))
	s.Equal("peers/x", buildRootKey(datastore.NewKey("peers/x")))
	// datastore.NewKey("") normalises to "/" — trimming it away would produce an
	// empty storage key that collides with the namespace root.
	s.Equal("/", buildRootKey(datastore.NewKey("")))
}

func (s *NodeRepoDatastoreTestSuite) TestFilterHelperInvertsMatch() {
	entry := query.Entry{Key: "/a", Value: []byte("v")}

	s.False(filter(nil, entry), "no filters means nothing is excluded")
	s.False(filter([]query.Filter{
		query.FilterKeyCompare{Op: query.Equal, Key: "/a"},
	}, entry))
	s.True(filter([]query.Filter{
		query.FilterKeyCompare{Op: query.Equal, Key: "/b"},
	}, entry), "a non-matching filter must exclude the entry")
}

func (s *NodeRepoDatastoreTestSuite) TestPutOverwritesAndDeleteIsFinal() {
	key := datastore.NewKey("overwrite/key")

	s.Require().NoError(s.repo.Put(s.ctx, key, []byte("v1")))
	s.Require().NoError(s.repo.Put(s.ctx, key, []byte("v2")))

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Equal([]byte("v2"), got)

	s.Require().NoError(s.repo.Delete(s.ctx, key))
	_, err = s.repo.Get(s.ctx, key)
	s.ErrorIs(err, datastore.ErrNotFound)

	// Deleting twice stays clean.
	s.NoError(s.repo.Delete(s.ctx, key))
}

func (s *NodeRepoDatastoreTestSuite) TestSyncAndDiskUsageOnLiveDB() {
	s.Require().NoError(s.repo.Put(s.ctx, datastore.NewKey("sync/key"), []byte("v")))
	s.Require().NoError(s.repo.Sync(s.ctx, datastore.NewKey("sync/key")))

	_, err := s.repo.DiskUsage(s.ctx)
	s.Require().NoError(err)
}

func TestNodeRepoDatastoreTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(NodeRepoDatastoreTestSuite))
}
