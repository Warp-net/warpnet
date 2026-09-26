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

package database

import (
	"github.com/oklog/ulid/v2"
	"time"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
)

const (
	AliasesRepoName = "/ALIASES"

	// MaxAliases is how many devices may stay paired to a node at once.
	MaxAliases = 10

	// aliasScanLimit bounds the scan that folds stale records away:
	// devices used to be stored under a random ULID, which gave one
	// device a record per pair refresh.
	aliasScanLimit = 100

	aliasTTL = time.Hour * 72
)

var (
	ErrNilAliasesRepo = local_store.DBError("aliases repo is nil")
	ErrTooManyAliases = local_store.DBError("too many aliases")
	ErrAliasNotFound  = local_store.DBError("alias not found")
	ErrAliasRevoked   = local_store.DBError("pairing revoked")
)

type AliasesStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type AliasesRepo struct {
	db AliasesStorer
}

func NewAliasesRepo(db AliasesStorer) *AliasesRepo {
	return &AliasesRepo{db: db}
}

func (repo *AliasesRepo) GetAliases() (aliases []domain.Alias, err error) {
	if repo.db == nil {
		return nil, ErrNilAliasesRepo
	}

	aliasesPrefix := local_store.NewPrefixBuilder(AliasesRepoName).
		AddRootID("None").
		AddRange(local_store.NoneRangeKey).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return aliases, err
	}
	defer txn.Rollback()

	limit := uint64(aliasScanLimit)
	items, _, err := txn.List(aliasesPrefix, &limit, nil)
	if err != nil {
		return aliases, err
	}

	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		var alias domain.Alias
		if err := json.Unmarshal(item.Value, &alias); err != nil {
			return aliases, err
		}
		if alias.IsRevoked() {
			continue
		}
		if _, ok := seen[alias.NodeId]; ok {
			continue
		}
		seen[alias.NodeId] = struct{}{}
		aliases = append(aliases, alias)
	}
	return aliases, nil
}

func (repo *AliasesRepo) GetNodeIDs() (ids []string, err error) {
	aliases, err := repo.GetAliases()
	if err != nil {
		return nil, err
	}
	for _, a := range aliases {
		ids = append(ids, a.NodeId)
	}
	return ids, nil
}

// SetAlias pairs a device or refreshes the one already paired under the
// same node id. A device owns a single record, so a pair refresh renews
// its TTL instead of claiming another slot.
func (repo *AliasesRepo) SetAlias(alias domain.Alias) error {
	if repo.db == nil {
		return ErrNilAliasesRepo
	}
	if alias.NodeId == "" {
		return local_store.DBError("empty alias node id")
	}

	aliasesPrefix := local_store.NewPrefixBuilder(AliasesRepoName).
		AddRootID("None").
		AddRange(local_store.NoneRangeKey).
		Build()
	aliasKey := local_store.NewPrefixBuilder(AliasesRepoName).
		AddRootID("None").
		AddRange(local_store.NoneRangeKey).
		AddParentId(alias.NodeId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	limit := uint64(aliasScanLimit)
	items, _, err := txn.List(aliasesPrefix, &limit, nil)
	if err != nil {
		return err
	}

	var (
		others   = make(map[string]struct{}, len(items))
		previous domain.Alias
		isPaired bool
	)

	for _, item := range items {
		var stored domain.Alias
		if err := json.Unmarshal(item.Value, &stored); err != nil {
			return err
		}
		if stored.NodeId != alias.NodeId {
			if !stored.IsRevoked() {
				others[stored.NodeId] = struct{}{}
			}
			continue
		}
		if stored.IsRevoked() {
			// The owner unpaired this device while it still held a
			// working pairing payload, so the payload itself no longer
			// counts. Only one the owner has shown since — carrying a
			// session token this node did not issue before — pairs it
			// back.
			if stored.Token == alias.Token {
				return ErrAliasRevoked
			}
			continue
		}
		if !isPaired {
			previous, isPaired = stored, true
		}
		// Devices used to be stored under a random ULID, which gave one
		// device a record per pair refresh. Fold those away.
		if local_store.DatabaseKey(item.Key) == aliasKey {
			continue
		}
		if err := txn.Delete(local_store.DatabaseKey(item.Key)); err != nil {
			return err
		}
	}

	if !isPaired && len(others) >= MaxAliases {
		return ErrTooManyAliases
	}
	if isPaired {
		alias.ID = previous.ID
		alias.CreatedAt = previous.CreatedAt
	}
	if alias.ID == "" {
		alias.ID = ulid.Make().String()
	}
	if alias.CreatedAt.IsZero() {
		alias.CreatedAt = time.Now()
	}
	alias.LastActive = time.Now()

	data, err := json.Marshal(alias)
	if err != nil {
		return err
	}
	if err := txn.SetWithTTL(aliasKey, data, aliasTTL); err != nil {
		return err
	}
	return txn.Commit()
}

// DeleteAlias unpairs a device. Authorization ends with the record — the
// middleware reads the alias set on every private request — but the device
// keeps a usable pairing payload, and the session token that payload
// carries stays valid for as long as this node process runs. So the
// tombstone is kept without a TTL: it outlives an active alias's 72h
// window on purpose, and only a later SetAlias for the same node id with a
// different token (a fresh login's session token) ever overwrites it.
func (repo *AliasesRepo) DeleteAlias(nodeId string) error {
	if repo.db == nil {
		return ErrNilAliasesRepo
	}
	if nodeId == "" {
		return local_store.DBError("empty alias node id")
	}

	aliasesPrefix := local_store.NewPrefixBuilder(AliasesRepoName).
		AddRootID("None").
		AddRange(local_store.NoneRangeKey).
		Build()
	aliasKey := local_store.NewPrefixBuilder(AliasesRepoName).
		AddRootID("None").
		AddRange(local_store.NoneRangeKey).
		AddParentId(nodeId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	limit := uint64(aliasScanLimit)
	items, _, err := txn.List(aliasesPrefix, &limit, nil)
	if err != nil {
		return err
	}

	var (
		revoked  domain.Alias
		isPaired bool
	)

	for _, item := range items {
		var stored domain.Alias
		if err := json.Unmarshal(item.Value, &stored); err != nil {
			return err
		}
		if stored.NodeId != nodeId || stored.IsRevoked() {
			continue
		}
		if !isPaired {
			revoked, isPaired = stored, true
		}
		if local_store.DatabaseKey(item.Key) == aliasKey {
			continue
		}
		if err := txn.Delete(local_store.DatabaseKey(item.Key)); err != nil {
			return err
		}
	}
	if !isPaired {
		return ErrAliasNotFound
	}

	revoked.RevokedAt = time.Now()
	data, err := json.Marshal(revoked)
	if err != nil {
		return err
	}
	if err := txn.Set(aliasKey, data); err != nil {
		return err
	}
	return txn.Commit()
}
