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
	"github.com/Warp-net/warpnet/database/local-store"
)

const (
	MutesRepoName = "/MUTES"

	// muteesSubName indexes per-muter the user ids they have muted.
	muteesSubName = "MUTEES"
	// mutersSubName indexes per-mutee the user ids who muted them.
	mutersSubName = "MUTERS"
)

type MutesStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

// MutesRepo persists the set of user ids the local owner has muted.
type MutesRepo struct {
	db MutesStorer
}

func NewMutesRepo(db MutesStorer) *MutesRepo { return &MutesRepo{db: db} }

func (repo *MutesRepo) Mute(muterId, muteeId string) error {
	if muterId == "" {
		return local_store.DBError("empty muter id")
	}
	if muteeId == "" {
		return local_store.DBError("empty mutee id")
	}

	muteesKey := local_store.NewPrefixBuilder(MutesRepoName).
		AddSubPrefix(muteesSubName).
		AddRootID(muterId).
		AddParentId(muteeId).
		Build()
	mutersKey := local_store.NewPrefixBuilder(MutesRepoName).
		AddSubPrefix(mutersSubName).
		AddRootID(muteeId).
		AddParentId(muterId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Set(muteesKey, []byte(muteeId)); err != nil {
		return err
	}
	if err = txn.Set(mutersKey, []byte(muterId)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *MutesRepo) Unmute(muterId, muteeId string) error {
	if muterId == "" {
		return local_store.DBError("empty muter id")
	}
	if muteeId == "" {
		return local_store.DBError("empty mutee id")
	}

	muteesKey := local_store.NewPrefixBuilder(MutesRepoName).
		AddSubPrefix(muteesSubName).
		AddRootID(muterId).
		AddParentId(muteeId).
		Build()
	mutersKey := local_store.NewPrefixBuilder(MutesRepoName).
		AddSubPrefix(mutersSubName).
		AddRootID(muteeId).
		AddParentId(muterId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Delete(muteesKey); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	if err = txn.Delete(mutersKey); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	return txn.Commit()
}

func (repo *MutesRepo) IsMuted(muterId, muteeId string) (bool, error) {
	if muterId == "" || muteeId == "" {
		return false, nil
	}
	muteesKey := local_store.NewPrefixBuilder(MutesRepoName).
		AddSubPrefix(muteesSubName).
		AddRootID(muterId).
		AddParentId(muteeId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return false, err
	}
	defer txn.Rollback()

	_, err = txn.Get(muteesKey)
	if local_store.IsNotFoundError(err) {
		return false, txn.Commit()
	}
	if err != nil {
		return false, err
	}
	return true, txn.Commit()
}

func (repo *MutesRepo) List(muterId string, limit *uint64, cursor *string) ([]string, string, error) {
	if muterId == "" {
		return nil, "", local_store.DBError("empty muter id")
	}
	prefix := local_store.NewPrefixBuilder(MutesRepoName).
		AddSubPrefix(muteesSubName).
		AddRootID(muterId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.Value))
	}
	return ids, cur, nil
}
