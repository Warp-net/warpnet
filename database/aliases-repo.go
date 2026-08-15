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
)

var ErrNilAliasesRepo = local_store.DBError("aliases repo is nil")

type AliasesStorer interface {
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
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

	aliassPrefix := local_store.NewPrefixBuilder(AliasesRepoName).
		AddRootID("None").
		AddRange(local_store.NoneRangeKey).
		Build()

	tx, err := repo.db.NewTxn()
	if err != nil {
		return aliases, err
	}
	defer tx.Rollback()

	limit := uint64(10)
	items, _, err := tx.List(aliassPrefix, &limit, nil)
	if err != nil {
		return aliases, err
	}
	if len(items) == 0 {
		return aliases, nil
	}

	for _, item := range items {
		var a domain.Alias
		err = json.Unmarshal(item.Value, &a)
		if err != nil {
			return aliases, err
		}
		aliases = append(aliases, a)
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

func (repo *AliasesRepo) SetAlias(alias domain.Alias) error {
	if repo.db == nil {
		return ErrNilAliasesRepo
	}
	if alias.ID == "" {
		alias.ID = ulid.Make().String()
	}
	if alias.CreatedAt.IsZero() {
		alias.CreatedAt = time.Now()
	}
	aliasKey := local_store.NewPrefixBuilder(AliasesRepoName).
		AddRootID("None").
		AddRange(local_store.NoneRangeKey).
		AddParentId(alias.ID).
		Build()

	data, err := json.Marshal(alias)
	if err != nil {
		return err
	}

	return repo.db.SetWithTTL(aliasKey, data, time.Hour*72)
}
