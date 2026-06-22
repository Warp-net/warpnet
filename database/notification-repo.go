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
	NotificationsRepoName = "/NOTIFICATIONS_V2"
)

var ErrNotificationsNotFound = local_store.DBError("notifications not found")

type NotificationsStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	NewReadTxn() (local_store.WarpTransactioner, error)
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
		AddParentId(not.Id).
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
// It scans the per-user prefix to locate the row from (userId, notificationId).
//
// The scan and the write live in *separate* transactions on purpose.
// Badger's SSI tracks every key the txn reads; doing the prefix scan
// inside the same RW txn that later writes one key would add the entire
// page (~100 sibling notifications) to the read set, and a concurrent
// MarkRead on any of those siblings would commit-conflict the second
// writer. Splitting the work means the write txn's read+write sets are
// both just {targetKey}, so two MarkRead calls on different notifications
// no longer collide. Two MarkRead calls on the *same* notification can
// still race, but IsRead is monotonic, so the loser's view-after-commit
// matches the winner's regardless.
func (repo *NotificationsRepo) MarkRead(userId, notificationId string) error {
	if userId == "" {
		return local_store.DBError("missing user id")
	}
	if notificationId == "" {
		return local_store.DBError("missing notification id")
	}

	key, err := repo.findNotificationKey(userId, notificationId)
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	raw, err := txn.Get(key)
	if err != nil {
		return err
	}
	var not domain.Notification
	if err := json.Unmarshal(raw, &not); err != nil {
		return err
	}
	if not.IsRead {
		return nil
	}
	not.IsRead = true
	bt, err := json.Marshal(not)
	if err != nil {
		return err
	}
	if err := txn.SetWithTTL(key, bt, time.Hour*24); err != nil {
		return err
	}
	return txn.Commit()
}

// findNotificationKey scans the per-user prefix in a discardable txn and
// returns the storage key of the row with the matching notification id.
// Discarding the txn (Rollback) drops every key from Badger's conflict
// table for this caller, so the subsequent write txn starts clean.
func (repo *NotificationsRepo) findNotificationKey(userId, notificationId string) (local_store.DatabaseKey, error) {
	prefix := local_store.NewPrefixBuilder(NotificationsRepoName).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return "", err
	}
	defer txn.Rollback()

	var (
		cursor string
		limit  uint64 = 100
	)
	for {
		items, cur, err := txn.List(prefix, &limit, &cursor)
		if err != nil {
			return "", err
		}
		for _, item := range items {
			var not domain.Notification
			if err := json.Unmarshal(item.Value, &not); err != nil {
				return "", err
			}
			if not.Id == notificationId {
				return local_store.DatabaseKey(item.Key), nil
			}
		}
		if cur == "" || cur == local_store.EndCursor || uint64(len(items)) < limit {
			break
		}
		cursor = cur
	}
	return "", ErrNotificationsNotFound
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

	items, cur, err := txn.ReverseList(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	return decodeNotifications(items, cur)
}

func decodeNotifications(items []local_store.ListItem, cursor string) ([]domain.Notification, string, error) {
	nots := make([]domain.Notification, 0, len(items))
	for _, item := range items {
		var not domain.Notification
		if err := json.Unmarshal(item.Value, &not); err != nil {
			return nil, "", err
		}
		nots = append(nots, not)
	}
	return nots, cursor, nil
}

func (repo *NotificationsRepo) ListSince(userId, since string, limit *uint64) ([]domain.Notification, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("missing user id")
	}
	if since == "" {
		return repo.List(userId, limit, nil)
	}

	prefix := local_store.NewPrefixBuilder(NotificationsRepoName).
		AddRootID(userId).
		Build()
	sinceKey := local_store.NewPrefixBuilder(NotificationsRepoName).
		AddRootID(userId).
		AddParentId(since).
		Build()

	txn, err := repo.db.NewReadTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	if _, gerr := txn.Get(sinceKey); gerr != nil {
		items, cur, lerr := txn.ReverseList(prefix, limit, nil)
		if lerr != nil {
			return nil, "", lerr
		}
		return decodeNotifications(items, cur)
	}

	var maxItems uint64
	if limit != nil {
		maxItems = *limit
	}

	var (
		out    []domain.Notification
		cursor        = sinceKey.String()
		page   uint64 = 100
	)
	for {
		items, next, lerr := txn.List(prefix, &page, &cursor)
		if lerr != nil {
			return nil, "", lerr
		}
		for _, item := range items {
			var not domain.Notification
			if uerr := json.Unmarshal(item.Value, &not); uerr != nil {
				return nil, "", uerr
			}
			if not.Id == since {
				continue
			}
			out = append(out, not)
			if maxItems > 0 && uint64(len(out)) >= maxItems {
				return out, next, nil
			}
		}
		if next == "" || next == local_store.EndCursor || next == cursor || uint64(len(items)) < page {
			break
		}
		cursor = next
	}
	return out, local_store.EndCursor, nil
}

// unreadCountPageSize is the per-iteration batch UnreadCount uses to
// walk a user's notifications. Exposed as a package var so tests can
// shrink it to force multiple iterations without having to seed
// hundreds of rows.
var unreadCountPageSize uint64 = 200

// UnreadCount scans every notification under the user's prefix and
// returns the number with IsRead == false. List paginates, so the
// caller can't count "unread across all pages" without doing the full
// scan itself; this method centralises that walk so handlers don't
// derive the unread count from one page (which gave a flickering
// "20 unread" badge that mirrored whatever happened to be on page 1).
//
// O(N) over the user's stored notifications. The repo's 24 h TTL on
// every Add() bounds N, so a scan per call is acceptable for now; if
// volume grows we'll move to a maintained counter mirrored on Add /
// MarkRead.
func (repo *NotificationsRepo) UnreadCount(userId string) (uint64, error) {
	if userId == "" {
		return 0, local_store.DBError("missing user id")
	}
	prefix := local_store.NewPrefixBuilder(NotificationsRepoName).
		AddRootID(userId).
		Build()

	// Read-only txn: this is a pure scan with no writes, so Badger's
	// read-conflict tracking is pointless overhead — it grows O(N)
	// with the number of keys touched. NewReadTxn opens
	// badger.NewTransaction(false), which skips that tracking.
	txn, err := repo.db.NewReadTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	var (
		count  uint64
		cursor string
		page   = unreadCountPageSize
	)
	for {
		items, next, lerr := txn.List(prefix, &page, &cursor)
		if lerr != nil {
			return 0, lerr
		}
		for _, item := range items {
			var not domain.Notification
			if uerr := json.Unmarshal(item.Value, &not); uerr != nil {
				return 0, uerr
			}
			if !not.IsRead {
				count++
			}
		}
		if next == "" || next == local_store.EndCursor || next == cursor || len(items) == 0 {
			break
		}
		cursor = next
	}

	// No Commit: this is a read-only txn, the deferred Rollback
	// (Discard under the hood) is the correct close.
	return count, nil
}
