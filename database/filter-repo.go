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
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
)

const FilterRepoName = "/FILTERS"

var ErrFilterNotFound = local_store.DBError("filter not found")

type FilterStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type FilterRepo struct {
	db FilterStorer
}

func NewFilterRepo(db FilterStorer) *FilterRepo {
	return &FilterRepo{db: db}
}

func filterKey(userId, filterId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(FilterRepoName).
		AddRootID(userId).
		AddParentId(filterId).
		Build()
}

func (repo *FilterRepo) Create(userId string, f domain.Filter) (domain.Filter, error) {
	if userId == "" {
		return domain.Filter{}, local_store.DBError("empty user id")
	}
	if f.Title == "" {
		return domain.Filter{}, local_store.DBError("empty title")
	}
	if f.Id == "" {
		f.Id = uuid.New().String()
	}
	f.UserId = userId
	for i := range f.Keywords {
		if f.Keywords[i].Id == "" {
			f.Keywords[i].Id = uuid.New().String()
		}
	}

	bt, err := json.Marshal(f)
	if err != nil {
		return domain.Filter{}, err
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Filter{}, err
	}
	defer txn.Rollback()
	if err := txn.Set(filterKey(userId, f.Id), bt); err != nil {
		return domain.Filter{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.Filter{}, err
	}
	return f, nil
}

func (repo *FilterRepo) Get(userId, filterId string) (domain.Filter, error) {
	if userId == "" || filterId == "" {
		return domain.Filter{}, ErrFilterNotFound
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Filter{}, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(filterKey(userId, filterId))
	if local_store.IsNotFoundError(err) {
		return domain.Filter{}, ErrFilterNotFound
	}
	if err != nil {
		return domain.Filter{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.Filter{}, err
	}
	var f domain.Filter
	if err := json.Unmarshal(bt, &f); err != nil {
		return domain.Filter{}, err
	}
	return f, nil
}

func (repo *FilterRepo) Update(userId string, f domain.Filter) (domain.Filter, error) {
	if userId == "" {
		return domain.Filter{}, local_store.DBError("empty user id")
	}
	if f.Id == "" {
		return domain.Filter{}, local_store.DBError("empty filter id")
	}
	existing, err := repo.Get(userId, f.Id)
	if err != nil {
		return domain.Filter{}, err
	}
	if f.Title != "" {
		existing.Title = f.Title
	}
	if len(f.Context) != 0 {
		existing.Context = f.Context
	}
	if f.Action != "" {
		existing.Action = f.Action
	}
	if f.ExpiresAt != nil {
		existing.ExpiresAt = f.ExpiresAt
	}
	// Keyword sub-CRUD is exposed via separate routes; whole-filter Update
	// intentionally does NOT overwrite the keywords array — use
	// AddKeyword / UpdateKeyword / DeleteKeyword for that.
	bt, err := json.Marshal(existing)
	if err != nil {
		return domain.Filter{}, err
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Filter{}, err
	}
	defer txn.Rollback()
	if err := txn.Set(filterKey(userId, existing.Id), bt); err != nil {
		return domain.Filter{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.Filter{}, err
	}
	return existing, nil
}

func (repo *FilterRepo) Delete(userId, filterId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if filterId == "" {
		return local_store.DBError("empty filter id")
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err := txn.Delete(filterKey(userId, filterId)); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	return txn.Commit()
}

func (repo *FilterRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Filter, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("empty user id")
	}
	prefix := local_store.NewPrefixBuilder(FilterRepoName).
		AddRootID(userId).
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
	if err := txn.Commit(); err != nil {
		return nil, "", err
	}
	out := make([]domain.Filter, 0, len(items))
	for _, it := range items {
		var f domain.Filter
		if err := json.Unmarshal(it.Value, &f); err != nil {
			return nil, "", err
		}
		out = append(out, f)
	}
	return out, cur, nil
}

// AddKeyword appends a new keyword to an existing filter.
func (repo *FilterRepo) AddKeyword(userId, filterId string, kw domain.FilterKeyword) (domain.FilterKeyword, error) {
	if kw.Keyword == "" {
		return domain.FilterKeyword{}, local_store.DBError("empty keyword")
	}
	f, err := repo.Get(userId, filterId)
	if err != nil {
		return domain.FilterKeyword{}, err
	}
	if kw.Id == "" {
		kw.Id = uuid.New().String()
	}
	f.Keywords = append(f.Keywords, kw)
	if err := repo.saveExact(userId, f); err != nil {
		return domain.FilterKeyword{}, err
	}
	return kw, nil
}

// UpdateKeyword replaces a keyword on the filter that owns it.
func (repo *FilterRepo) UpdateKeyword(userId string, kw domain.FilterKeyword) (domain.FilterKeyword, error) {
	if kw.Id == "" {
		return domain.FilterKeyword{}, local_store.DBError("empty keyword id")
	}
	f, found, err := repo.findFilterForKeyword(userId, kw.Id)
	if err != nil {
		return domain.FilterKeyword{}, err
	}
	if !found {
		return domain.FilterKeyword{}, ErrFilterNotFound
	}
	for i := range f.Keywords {
		if f.Keywords[i].Id == kw.Id {
			if kw.Keyword != "" {
				f.Keywords[i].Keyword = kw.Keyword
			}
			f.Keywords[i].WholeWord = kw.WholeWord
			if err := repo.saveExact(userId, f); err != nil {
				return domain.FilterKeyword{}, err
			}
			return f.Keywords[i], nil
		}
	}
	return domain.FilterKeyword{}, ErrFilterNotFound
}

// DeleteKeyword removes a keyword by id from whichever filter owns it.
func (repo *FilterRepo) DeleteKeyword(userId, keywordId string) error {
	if keywordId == "" {
		return local_store.DBError("empty keyword id")
	}
	f, found, err := repo.findFilterForKeyword(userId, keywordId)
	if err != nil {
		return err
	}
	if !found {
		return nil // idempotent — gone is gone
	}
	out := f.Keywords[:0]
	for _, k := range f.Keywords {
		if k.Id == keywordId {
			continue
		}
		out = append(out, k)
	}
	f.Keywords = out
	return repo.saveExact(userId, f)
}

func (repo *FilterRepo) findFilterForKeyword(userId, keywordId string) (domain.Filter, bool, error) {
	// Keyword search is O(filters) for the user — filter counts are small
	// (Mastodon caps at a few dozen), so a flat scan is acceptable.
	var cursor string
	limit := uint64(100)
	for {
		filters, cur, err := repo.List(userId, &limit, &cursor)
		if err != nil {
			return domain.Filter{}, false, err
		}
		for _, f := range filters {
			for _, k := range f.Keywords {
				if k.Id == keywordId {
					return f, true, nil
				}
			}
		}
		if cur == "" || cur == "end" || uint64(len(filters)) < limit {
			return domain.Filter{}, false, nil
		}
		cursor = cur
	}
}

// saveExact persists the filter without going through Update's
// merge-with-existing path — used by the keyword sub-CRUD which has
// already loaded the full record.
func (repo *FilterRepo) saveExact(userId string, f domain.Filter) error {
	bt, err := json.Marshal(f)
	if err != nil {
		return err
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err := txn.Set(filterKey(userId, f.Id), bt); err != nil {
		return err
	}
	return txn.Commit()
}
