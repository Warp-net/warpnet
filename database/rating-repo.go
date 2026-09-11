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

package database

import (
	"strings"
	"time"

	ds "github.com/Warp-net/warpnet/database/datastore"
	local_store "github.com/Warp-net/warpnet/database/local-store"
)

type RatingStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	Get(key local_store.DatabaseKey) ([]byte, error)
	GetExpiration(key local_store.DatabaseKey) (uint64, error)
	GetSize(key local_store.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local_store.WarpDB
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local_store.DatabaseKey, value []byte) error
	Delete(key local_store.DatabaseKey) error
}

const ratingPrefix = "RATING"

// NewRatingRepo backs the rating CRDT. It is kept apart from the stats
// CRDT because each go-ds-crdt instance owns its whole namespace.
func NewRatingRepo(db RatingStorer) ds.Datastore {
	prefix := ratingPrefix
	if !strings.HasPrefix(prefix, requiredPrefixSlash) {
		prefix = requiredPrefixSlash + prefix
	}
	return &NodeRepo{
		db:       db,
		prefix:   prefix,
		stopChan: make(chan struct{}),
	}
}
