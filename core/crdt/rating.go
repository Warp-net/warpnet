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

package crdt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/Warp-net/warpnet/json"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/libp2p/go-libp2p/core/host"
	log "github.com/sirupsen/logrus"
)

const (
	RepoName        = "/RATING"
	generationBytes = 16
)

type CRDTRatingStore struct {
	ctx context.Context

	store           *crdt.Datastore
	addCallbacks    []func(k ds.Key, v []byte)
	removeCallbacks []func(k ds.Key)
}

func NewCRDTRatingStore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore CRDTStorer,
	node host.Host,
	router CRDTRouter,
	addCallbacks []func(k ds.Key, v []byte),
	removeCallbacks []func(k ds.Key),
) (*CRDTRatingStore, error) {
	s := new(CRDTRatingStore)
	generation, err := newGeneration()
	if err != nil {
		return nil, fmt.Errorf("rating: generation: %w", err)
	}

	baseStore := ds.MutexWrap(datastore)

	blockstore := ds.NewIdStore(ds.NewBlockstore(baseStore, ds.WriteThrough(true)))

	bitswapNetwork := warpnet.NewBitswapNetwork(node)
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, router, blockstore)

	for _, p := range node.Network().Peers() {
		bitswapExchange.PeerConnected(p)
	}

	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	l := log.StandardLogger().WithContext(ctx)

	opts := crdt.DefaultOptions()
	opts.Logger = l
	opts.PutHook = func(k ds.Key, v []byte) {
		for _, h := range s.addCallbacks {
			h(k, v)
		}
	}
	opts.DeleteHook = func(k ds.Key) {
		for _, h := range s.removeCallbacks {
			h(k)
		}
	}
	opts.RebroadcastInterval = time.Minute
	opts.DAGSyncerTimeout = time.Minute
	opts.MultiHeadProcessing = true

	crdtStore, err := crdt.New(
		baseStore,
		ds.NewKey(""),
		dagService,
		broadcaster,
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}
	s.store = crdtStore
	return s, nil
}

func (s *CRDTRatingStore) entriesFor(peerId string) ([]entry, error) {
	if peerId == "" {
		return nil, ErrEmptySubject
	}
	if s.idx.has(peerId) {
		return s.idx.entries(peerId), nil
	}

	s.fallbackMx.Lock()
	defer s.fallbackMx.Unlock()
	if s.idx.has(peerId) { // another caller filled it while we waited
		return s.idx.entries(peerId), nil
	}
	err := s.loadSubject(peerId)
	return s.idx.entries(peerId), err
}

func (s *CRDTRatingStore) loadSubject(peerId string) error {
	if s.store == nil {
		s.idx.ensure(peerId)
		return nil
	}
	results, err := s.store.Query(s.ctx, ds.Query{Prefix: PeerPrefix(peerId)})
	if err != nil {
		// Not marked present: a failed load must be retried on the
		// next read, not remembered as an empty peerId.
		return fmt.Errorf("rating: query peerId %s: %w", peerId, err)
	}
	defer func() { _ = results.Close() }()

	s.idx.ensure(peerId)
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		rec, err := s.admit(r.Value)
		if err != nil {
			log.Debugf("rating: dropping record for %s: %v", peerId, err)
			continue
		}
		s.idx.put(rec)
	}
	return nil
}

func (s *CRDTRatingStore) scan() error {
	if s.store == nil {
		return nil
	}
	results, err := s.store.Query(s.ctx, ds.Query{Prefix: KeyPrefix()})
	if err != nil {
		return err
	}
	defer func() { _ = results.Close() }()

	var admitted int
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		rec, err := s.admit(r.Value)
		if err != nil {
			log.Debugf("rating: dropping record at startup: %v", err)
			continue
		}
		s.idx.put(rec)
		admitted++
	}
	log.Infof("rating: indexed %d entry records at startup", admitted)
	return nil
}

// admit parses and authenticates one raw record; indexing it is the
// caller's decision, because only the caller knows whether it holds the
// peerId's complete entry set.
func (s *CRDTRatingStore) admit(value []byte) (Record, error) {
	var rec Record
	if len(value) == 0 {
		return rec, ErrEmptyRecord
	}
	if err := json.Unmarshal(value, &rec); err != nil {
		return rec, fmt.Errorf("rating: unmarshal record: %w", err)
	}
	if err := rec.Verify(); err != nil {
		return rec, fmt.Errorf("rating: unverifiable record for %s: %w", rec.Subject, err)
	}
	if err := rec.Validate(s.now()); err != nil {
		log.Warnf("rating: observer %s authored an invalid record: %v", rec.Observer, err)
		if chargeErr := s.Record(warpnet.FromStringToPeerID(rec.Observer), KindForgedRecord); chargeErr != nil {
			log.Warnf("rating: charging %s for a forged record: %v", rec.Observer, chargeErr)
		}
		return rec, err
	}
	return rec, nil
}

func (s *CRDTRatingStore) onPut(key string, value []byte) {
	if !strings.HasPrefix(key, KeyPrefix()) {
		return
	}
	rec, err := s.admit(value)
	if err != nil {
		log.Debugf("rating: dropping merged record: %v", err)
		return
	}
	// Update-only: the record is already in the datastore, so if the
	// peerId is not indexed the next read loads it whole.
	s.idx.update(rec)
}

func (s *CRDTRatingStore) onDelete(key string) {
	peerId, observer, dim, bucket, generation, ok := parseKey(key)
	if !ok {
		return
	}
	s.idx.drop(peerId, observer, dim, bucket, generation)
}

// parseKey splits /RATING/obs/{peerId}/{observer}/{dim}/{bucket}/{generation}.
func parseKey(key string) (peerId, observer string, dim string, bucket int64, generation string, ok bool) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(key, "/"), RepoName+"/obs/")
	if trimmed == key {
		return "", "", 0, 0, "", false
	}
	parts := strings.Split(trimmed, "/")
	const wantParts = 5
	if len(parts) != wantParts {
		return "", "", 0, 0, "", false
	}
	bucket, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return "", "", 0, 0, "", false
	}
	return parts[0], parts[1], dim, bucket, parts[4], true
}

func RecordKey(peerId, observer string, dim string, bucket int64, generation string) string {
	return "/" + RepoName + "/obs/" +
		peerId + "/" +
		observer + "/" +
		dim + "/" +
		strconv.FormatInt(bucket, 10) + "/" +
		generation
}

func KeyPrefix() string { return "/" + RepoName + "/obs" }

func PeerPrefix(peerId string) string { return KeyPrefix() + "/" + peerId }

func newGeneration() (string, error) {
	var buf [generationBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
