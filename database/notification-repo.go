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
	"github.com/oklog/ulid/v2"
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
		not.Id = ulid.Make().String()
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

	if err = txn.SetWithTTL(notKey, bt, time.Hour*24); err != nil {
		return err
	}
	return txn.Commit()
}

// MarkRead flips Notification.IsRead to true for the given notification.
// The record is re-written at its existing key (Notification keys are
// id-indexed within a per-user prefix, so we have to scan to find the
// match like Get does — count stays bounded by the per-user notification
// retention window).
func (repo *NotificationsRepo) MarkRead(userId, notificationId string) error {
	if userId == "" {
		return local_store.DBError("missing user id")
	}
	if notificationId == "" {
		return local_store.DBError("missing notification id")
	}

	prefix := local_store.NewPrefixBuilder(NotificationsRepoName).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	var (
		cursor string
		limit  uint64 = 100
	)
	for {
		items, cur, err := txn.List(prefix, &limit, &cursor)
		if err != nil {
			return err
		}
		for _, item := range items {
			var not domain.Notification
			if err := json.Unmarshal(item.Value, &not); err != nil {
				return err
			}
			if not.Id != notificationId {
				continue
			}
			if not.IsRead {
				return txn.Commit()
			}
			not.IsRead = true
			bt, err := json.Marshal(not)
			if err != nil {
				return err
			}
			if err := txn.SetWithTTL(local_store.DatabaseKey(item.Key), bt, time.Hour*24); err != nil {
				return err
			}
			return txn.Commit()
		}
		if cur == "" || cur == local_store.EndCursor || uint64(len(items)) < limit {
			break
		}
		cursor = cur
	}
	return ErrNotificationsNotFound
}

func (repo *NotificationsRepo) Get(userId, notificationId string) (domain.Notification, error) {
	if userId == "" {
		return domain.Notification{}, local_store.DBError("missing user id")
	}
	if notificationId == "" {
		return domain.Notification{}, local_store.DBError("missing notification id")
	}

	prefix := local_store.NewPrefixBuilder(NotificationsRepoName).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Notification{}, err
	}
	defer txn.Rollback()

	var (
		cursor string
		limit  uint64 = 100
	)
	for {
		items, cur, err := txn.List(prefix, &limit, &cursor)
		if err != nil {
			return domain.Notification{}, err
		}
		for _, item := range items {
			var not domain.Notification
			if err := json.Unmarshal(item.Value, &not); err != nil {
				return domain.Notification{}, err
			}
			if not.Id == notificationId {
				if err := txn.Commit(); err != nil {
					return domain.Notification{}, err
				}
				return not, nil
			}
		}
		if cur == "" || cur == local_store.EndCursor || uint64(len(items)) < limit {
			break
		}
		cursor = cur
	}

	if err := txn.Commit(); err != nil {
		return domain.Notification{}, err
	}
	return domain.Notification{}, ErrNotificationsNotFound
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
