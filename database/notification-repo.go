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
	"time"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
)

const (
	NotificationsRepoName = "/NOTIFICATIONS"
)

var ErrNotificationsNotFound = local_store.DBError("notifications not found")

type NotificationsStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type NotificationsRepo struct {
	db NotificationsStorer
}

func NewNotificationsRepo(db NotificationsStorer) *NotificationsRepo {
	return &NotificationsRepo{db: db}
}

func (repo *NotificationsRepo) Add(not domain.Notification) error {
	if not.UserId == "" {
		return local_store.DBError("missing user id")
	}

	if not.Id == "" {
		not.Id = uuid.New().String()
	}
	if not.CreatedAt.IsZero() {
		not.CreatedAt = time.Now()
	}

	notKey := local_store.NewPrefixBuilder(NotificationsRepoName).
		AddRootID(not.UserId).
		AddReversedTimestamp(not.CreatedAt).
		AddParentId(not.Type.String()).
		AddId(not.Id).
		Build()

	bt, err := json.Marshal(not)
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	return txn.SetWithTTL(notKey, bt, time.Hour*24)
}

func (repo *NotificationsRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("missing user id")
	}
	prefix := local_store.NewPrefixBuilder(NotificationsRepoName).
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

	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	nots := make([]domain.Notification, 0, len(items))
	for _, item := range items {
		var not domain.Notification
		err = json.Unmarshal(item.Value, &not)
		if err != nil {
			return nil, "", err
		}
		nots = append(nots, not)
	}

	return nots, cur, nil
}
