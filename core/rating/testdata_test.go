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

type memStore struct {
	mu       sync.Mutex
	data     map[string][]byte
	putErr   error
	queryErr error
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
	if m.queryErr != nil {
		return nil, m.queryErr
	}
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

func (m *memStore) clone() *memStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := newMemStore()
	for k, v := range m.data {
		out.data[k] = v
	}
	return out
}

func (m *memStore) OnPut(func(ds.Key, []byte)) {}
func (m *memStore) OnDelete(func(ds.Key))      {}

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
