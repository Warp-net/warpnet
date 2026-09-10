// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"crypto/ed25519"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/stretchr/testify/require"
)

type identity struct {
	id   warpnet.WarpPeerID
	priv ed25519.PrivateKey
}

func newIdentity(t *testing.T) identity {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	return identity{id: id, priv: priv}
}

type expiry struct {
	dimension    string
	beforeBucket int64
}

// fakeStore is an in-memory Storer with the CRDT store's contract: one
// key per record tuple, hooks on every put and delete, own-only deletes.
type fakeStore struct {
	mu      sync.Mutex
	self    string
	data    map[string]domain.RatingRecord
	putErr  error
	listErr error
	expired []expiry

	puts    []func(domain.RatingRecord)
	deletes []func(domain.RatingRecord)
}

func newFakeStore(self warpnet.WarpPeerID) *fakeStore {
	return &fakeStore{self: self.String(), data: make(map[string]domain.RatingRecord)}
}

func recordKeyOf(rec domain.RatingRecord) string {
	return fmt.Sprintf("%s/%s/%s/%d/%s", rec.PeerId, rec.ObserverId, rec.Dimension, rec.Bucket, rec.Generation)
}

func (s *fakeStore) Put(rec domain.RatingRecord) error {
	s.mu.Lock()
	if s.putErr != nil {
		s.mu.Unlock()
		return s.putErr
	}
	s.data[recordKeyOf(rec)] = rec
	hooks := s.puts
	s.mu.Unlock()
	for _, hook := range hooks {
		hook(rec)
	}
	return nil
}

// merge is a record arriving over the DAG from another node.
func (s *fakeStore) merge(rec domain.RatingRecord) {
	_ = s.Put(rec)
}

func (s *fakeStore) List(peerId string) ([]domain.RatingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []domain.RatingRecord
	for _, rec := range s.data {
		if rec.PeerId == peerId {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (s *fakeStore) DeleteExpired(dimension string, beforeBucket int64) error {
	s.mu.Lock()
	s.expired = append(s.expired, expiry{dimension: dimension, beforeBucket: beforeBucket})
	var removed []domain.RatingRecord
	for key, rec := range s.data {
		if rec.ObserverId == s.self && rec.Dimension == dimension && rec.Bucket < beforeBucket {
			delete(s.data, key)
			removed = append(removed, rec)
		}
	}
	hooks := s.deletes
	s.mu.Unlock()
	for _, rec := range removed {
		for _, hook := range hooks {
			hook(rec)
		}
	}
	return nil
}

func (s *fakeStore) OnPut(hook func(domain.RatingRecord)) {
	s.mu.Lock()
	s.puts = append(s.puts, hook)
	s.mu.Unlock()
}

func (s *fakeStore) OnDelete(hook func(domain.RatingRecord)) {
	s.mu.Lock()
	s.deletes = append(s.deletes, hook)
	s.mu.Unlock()
}

func (s *fakeStore) setPutErr(err error) {
	s.mu.Lock()
	s.putErr = err
	s.mu.Unlock()
}

func (s *fakeStore) setListErr(err error) {
	s.mu.Lock()
	s.listErr = err
	s.mu.Unlock()
}

func (s *fakeStore) records() []domain.RatingRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.RatingRecord, 0, len(s.data))
	for _, rec := range s.data {
		out = append(out, rec)
	}
	return out
}

func (s *fakeStore) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data)
}

func (s *fakeStore) expiries() []expiry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]expiry(nil), s.expired...)
}

// fakeConns reports every peer as connected since opened; a zero opened
// means nobody is connected.
type fakeConns struct {
	opened time.Time
}

type fakeConn struct {
	network.Conn

	opened time.Time
}

func (c fakeConn) Stat() network.ConnStats {
	return network.ConnStats{Stats: network.Stats{Opened: c.opened}}
}

func (c fakeConns) ConnsToPeer(warpnet.WarpPeerID) []network.Conn {
	if c.opened.IsZero() {
		return nil
	}
	return []network.Conn{fakeConn{opened: c.opened}}
}

type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fixedClock {
	return &fixedClock{now: time.Now().UTC().Truncate(time.Hour)}
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixedClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// acquainted is a network that has known every peer for a day.
func acquainted(clock *fixedClock) fakeConns {
	return fakeConns{opened: clock.Now().Add(-24 * time.Hour)}
}

func newTestEngine(t *testing.T, self identity, store Storer, clock *fixedClock, nodeType string) *Engine {
	t.Helper()
	e, err := NewEngine(
		t.Context(), store, acquainted(clock), self.priv, nodeType,
		WithClock(clock.Now),
		WithFlushInterval(time.Hour), // tests drive the flush by hand
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func newMemberEngine(t *testing.T, self identity, store Storer, clock *fixedClock) *Engine {
	t.Helper()
	return newTestEngine(t, self, store, clock, warpnet.MemberNode)
}

// signedRecord builds a valid record from observer about peerId.
func signedRecord(
	observer identity, peerId warpnet.WarpPeerID, dim Dimension,
	bucket int64, generation string, counts ...kindCount,
) domain.RatingRecord {
	offences := make([]domain.OffenceCount, 0, len(counts))
	for _, c := range counts {
		offences = append(offences, domain.OffenceCount{Kind: c.kind.String(), Count: c.count})
	}
	rec := domain.RatingRecord{
		PeerId:     peerId.String(),
		ObserverId: observer.id.String(),
		Dimension:  dim.String(),
		Bucket:     bucket,
		Generation: generation,
		Offences:   offences,
		UpdatedAt:  bucketTime(bucket),
	}
	if err := signRecord(&rec, observer.priv); err != nil {
		panic(err) // a test identity always carries a usable key
	}
	return rec
}

const (
	genA = "00112233445566778899aabbccddeeff"
	genB = "ffeeddccbbaa99887766554433221100"
)

// seed stores a record without firing hooks: history that was already
// on disk before this process started.
func (s *fakeStore) seed(rec domain.RatingRecord) {
	s.mu.Lock()
	s.data[recordKeyOf(rec)] = rec
	s.mu.Unlock()
}
