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
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/libp2p/go-libp2p/core/host"
	log "github.com/sirupsen/logrus"
)

const (
	RatingRepoName = "/RATING"

	recordNamespace = "record"
	recordPrefix    = RatingRepoName + "/" + recordNamespace
	recordKeyParts  = 5

	ErrForeignRatingRecord     = warpnet.WarpError("crdt rating: record is not authored by this node")
	ErrMalformedRatingRecord   = warpnet.WarpError("crdt rating: record key part is empty or contains a slash")
	ErrRatingRecordKeyMismatch = warpnet.WarpError("crdt rating: record content does not match its key")
)

// CRDTRatingStore replicates signed rating records. A node writes and
// deletes only its own records; everyone else's arrive through the DAG.
type CRDTRatingStore struct {
	crdt   *crdt.Datastore
	ctx    context.Context
	cancel context.CancelFunc
	nodeID string

	mu          sync.RWMutex
	putHooks    []func(domain.RatingRecord)
	deleteHooks []func(domain.RatingRecord)
}

// NewCRDTRatingStore creates a new CRDT-based rating records store
func NewCRDTRatingStore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore CRDTStorer,
	node host.Host,
	router CRDTRouter,
) (*CRDTRatingStore, error) {
	ctx, cancel := context.WithCancel(ctx)

	baseStore := ds.MutexWrap(datastore)

	blockstore := ds.NewIdStore(ds.NewBlockstore(baseStore, ds.WriteThrough(true)))

	bitswapNetwork := warpnet.NewBitswapNetwork(node)
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, router, blockstore)

	for _, p := range node.Network().Peers() {
		bitswapExchange.PeerConnected(p)
	}

	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	store := &CRDTRatingStore{
		ctx:    ctx,
		cancel: cancel,
		nodeID: node.ID().String(),
	}

	opts := crdt.DefaultOptions()
	opts.Logger = log.StandardLogger().WithContext(ctx)
	opts.PutHook = store.onPut
	opts.DeleteHook = store.onDelete
	opts.RebroadcastInterval = time.Minute
	opts.DAGSyncerTimeout = time.Minute
	opts.MultiHeadProcessing = true

	crdtStore, err := crdt.New(
		baseStore,
		ds.NewKey(""), // node repo's already set the prefix
		dagService,
		broadcaster,
		opts,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}
	store.crdt = crdtStore

	return store, nil
}

// Put writes one of this node's own records; the key is derived from the record.
func (s *CRDTRatingStore) Put(rec domain.RatingRecord) error {
	if rec.ObserverId != s.nodeID {
		return ErrForeignRatingRecord
	}
	key, err := recordKey(rec)
	if err != nil {
		return err
	}
	value, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("crdt rating: marshal record %s: %w", key, err)
	}
	if err := s.crdt.Put(s.ctx, key, value); err != nil {
		return fmt.Errorf("crdt rating: write record %s: %w", key, err)
	}
	return nil
}

// List returns every replicated record about peerId, own and foreign.
func (s *CRDTRatingStore) List(peerId string) ([]domain.RatingRecord, error) {
	if peerId == "" || strings.Contains(peerId, "/") {
		return nil, ErrMalformedRatingRecord
	}
	prefix := recordPrefix + "/" + peerId
	results, err := s.crdt.Query(s.ctx, ds.Query{Prefix: prefix})
	if err != nil {
		return nil, fmt.Errorf("crdt rating: query %s: %w", prefix, err)
	}
	defer func() { _ = results.Close() }()

	var records []domain.RatingRecord
	for r := range results.Next() {
		if r.Error != nil {
			return nil, fmt.Errorf("crdt rating: iterate %s: %w", prefix, r.Error)
		}
		rec, err := decodeRecord(r.Key, r.Value)
		if err != nil {
			log.Debugf("crdt rating: skipping record %s: %v", r.Key, err)
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// DeleteExpired removes this node's own records of one dimension with a
// bucket before beforeBucket. Foreign records are never deleted: a CRDT
// delete is a tombstone that propagates to every replica.
func (s *CRDTRatingStore) DeleteExpired(dimension string, beforeBucket int64) error {
	results, err := s.crdt.Query(s.ctx, ds.Query{Prefix: recordPrefix, KeysOnly: true})
	if err != nil {
		return fmt.Errorf("crdt rating: query records: %w", err)
	}

	var expired []string
	for r := range results.Next() {
		if r.Error != nil {
			_ = results.Close()
			return fmt.Errorf("crdt rating: iterate records: %w", r.Error)
		}
		rec, ok := parseRecordKey(r.Key)
		if !ok || rec.ObserverId != s.nodeID || rec.Dimension != dimension || rec.Bucket >= beforeBucket {
			continue
		}
		expired = append(expired, r.Key)
	}
	_ = results.Close()

	var errs []error
	for _, key := range expired {
		if err := s.crdt.Delete(s.ctx, ds.NewKey(key)); err != nil {
			errs = append(errs, fmt.Errorf("crdt rating: delete %s: %w", key, err))
		}
	}
	if removed := len(expired) - len(errs); removed > 0 {
		log.Infof("crdt rating: removed %d expired own records", removed)
	}
	return errors.Join(errs...)
}

// OnPut registers a hook fired for every record merged into the store, own or foreign.
func (s *CRDTRatingStore) OnPut(hook func(domain.RatingRecord)) {
	if s == nil || hook == nil {
		return
	}
	s.mu.Lock()
	s.putHooks = append(s.putHooks, hook)
	s.mu.Unlock()
}

// OnDelete registers a hook fired for every removed record; only its key fields are set.
func (s *CRDTRatingStore) OnDelete(hook func(domain.RatingRecord)) {
	if s == nil || hook == nil {
		return
	}
	s.mu.Lock()
	s.deleteHooks = append(s.deleteHooks, hook)
	s.mu.Unlock()
}

func (s *CRDTRatingStore) onPut(k ds.Key, v []byte) {
	rec, err := decodeRecord(k.String(), v)
	if err != nil {
		log.Debugf("crdt rating: ignoring merged record %s: %v", k, err)
		return
	}
	s.mu.RLock()
	hooks := s.putHooks
	s.mu.RUnlock()
	for _, hook := range hooks {
		hook(rec)
	}
}

func (s *CRDTRatingStore) onDelete(k ds.Key) {
	rec, ok := parseRecordKey(k.String())
	if !ok {
		return
	}
	s.mu.RLock()
	hooks := s.deleteHooks
	s.mu.RUnlock()
	for _, hook := range hooks {
		hook(rec)
	}
}

// Close stops the CRDT store
func (s *CRDTRatingStore) Close() error {
	if s == nil {
		return nil
	}
	s.cancel()
	return s.crdt.Close()
}

// recordKey is /RATING/record/{peerId}/{observerId}/{dimension}/{bucket}/{generation}.
func recordKey(rec domain.RatingRecord) (ds.Key, error) {
	for _, part := range []string{rec.PeerId, rec.ObserverId, rec.Dimension, rec.Generation} {
		if part == "" || strings.Contains(part, "/") {
			return ds.Key{}, ErrMalformedRatingRecord
		}
	}
	return ds.NewKey(fmt.Sprintf(
		"%s/%s/%s/%s/%d/%s",
		recordPrefix, rec.PeerId, rec.ObserverId, rec.Dimension, rec.Bucket, rec.Generation,
	)), nil
}

func parseRecordKey(key string) (domain.RatingRecord, bool) {
	trimmed, ok := strings.CutPrefix(key, recordPrefix+"/")
	if !ok {
		return domain.RatingRecord{}, false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != recordKeyParts {
		return domain.RatingRecord{}, false
	}
	if slices.Contains(parts, "") {
		return domain.RatingRecord{}, false
	}
	bucket, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return domain.RatingRecord{}, false
	}
	return domain.RatingRecord{
		PeerId:     parts[0],
		ObserverId: parts[1],
		Dimension:  parts[2],
		Bucket:     bucket,
		Generation: parts[4],
	}, true
}

// decodeRecord unmarshals a stored value and checks it sits under its own key.
func decodeRecord(key string, value []byte) (domain.RatingRecord, error) {
	var rec domain.RatingRecord
	if err := json.Unmarshal(value, &rec); err != nil {
		return rec, fmt.Errorf("unmarshal: %w", err)
	}
	own, err := recordKey(rec)
	if err != nil {
		return rec, err
	}
	if own.String() != key {
		return rec, ErrRatingRecordKeyMismatch
	}
	return rec, nil
}
