// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"context"
	"crypto/ed25519"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	dsq "github.com/ipfs/go-datastore/query"
	"github.com/stretchr/testify/require"
)

// identity is a throwaway node keypair: rating records are verified
// against the pubkey derived from the observer's peer id, so tests
// need real ed25519 identities, not string literals.
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

// memStore is an in-memory stand-in for the CRDT datastore. Prefix
// queries are all the store ever issues.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
	// putErr, when set, fails every write — used to prove Record
	// stays non-blocking when persistence is broken.
	putErr error
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string][]byte)}
}

func (m *memStore) Get(_ context.Context, key ds.Key) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key.String()]
	if !ok {
		return nil, ds.ErrNotFound
	}
	return v, nil
}

func (m *memStore) Put(_ context.Context, key ds.Key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.putErr != nil {
		return m.putErr
	}
	m.data[key.String()] = value
	return nil
}

func (m *memStore) Delete(_ context.Context, key ds.Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key.String())
	return nil
}

func (m *memStore) Query(_ context.Context, q ds.Query) (ds.Results, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := make([]dsq.Entry, 0, len(m.data))
	for k, v := range m.data {
		if q.Prefix != "" && !strings.HasPrefix(k, q.Prefix) {
			continue
		}
		e := dsq.Entry{Key: k}
		if !q.KeysOnly {
			e.Value = v
		}
		entries = append(entries, e)
	}
	return dsq.ResultsWithEntries(q, entries), nil
}

func (m *memStore) keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.data))
	for k := range m.data {
		out = append(out, k)
	}
	return out
}

func (m *memStore) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}

// clone copies the contents, standing in for what the CRDT DAG would
// replay back to a node that restarted with nothing.
func (m *memStore) clone() *memStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := newMemStore()
	for k, v := range m.data {
		out.data[k] = v
	}
	return out
}

func opener(store Datastore) Opener {
	return func(Hooks) (Datastore, error) { return store, nil }
}

// fixedClock lets a test place entries in specific buckets and
// then age them.
type fixedClock struct {
	mu  sync.Mutex
	now time.Time
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

// signedRecord builds a valid record from observer about subject.
func signedRecord(
	observer identity, subject warpnet.WarpPeerID, dim Dimension,
	bucket int64, generation string, counts ...CountEntry,
) Record {
	rec := Record{
		Subject:    subject.String(),
		Observer:   observer.id.String(),
		Dim:        dim,
		Bucket:     bucket,
		Generation: generation,
		Counts:     counts,
		UpdatedAt:  bucketTime(bucket),
	}
	if err := rec.Sign(observer.priv); err != nil {
		panic(err) // a test identity always carries a usable key
	}
	return rec
}

const (
	genA = "00112233445566778899aabbccddeeff"
	genB = "ffeeddccbbaa99887766554433221100"
)
