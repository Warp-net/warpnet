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
	"errors"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/json"
)

// slash is required because of: invalid datastore key: NODES:/peers/keys/AASAQAISEAXNRKHMX2O3AA26JM7NGIWUPOGIITJ2UHHXGX4OWIEKPNAW6YCSK/priv
const (
	NodesNamespace        = "/NODES"
	BlocklistSubNamespace = "BLOCKLIST"

	ForeverBlockDuration time.Duration = 0
	MaxBlockDuration                   = 90 * 24 * time.Hour
)

var (
	ErrNilNodeRepo = local.DBError("node repo is nil")
)

type NodeStorer interface {
	NewTxn() (local.WarpTransactioner, error)
	Get(key local.DatabaseKey) ([]byte, error)
	SetWithTTL(key local.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local.DatabaseKey, value []byte) error
	Delete(key local.DatabaseKey) error
}

type NodeRepo struct {
	db NodeStorer
}

func NewNodeRepo(db NodeStorer) *NodeRepo {
	nr := &NodeRepo{
		db: db,
	}

	return nr
}

type BlocklistedItem struct {
	PeerID   warpnet.WarpPeerID
	Duration *time.Duration
}

func (d *NodeRepo) BlocklistExponential(peerId warpnet.WarpPeerID) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if peerId == "" {
		return local.DBError("empty peer ID")
	}
	blocklistKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddRootID(peerId.String()).
		Build()

	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	bt, err := txn.Get(blocklistKey)
	if err != nil && !errors.Is(err, local.ErrKeyNotFound) {
		return err
	}

	var item BlocklistedItem
	if len(bt) != 0 {
		if err := json.Unmarshal(bt, &item); err != nil {
			return err
		}
	}

	if item.Duration == nil {
		item.Duration = func(d time.Duration) *time.Duration { return &d }(time.Hour)
	}
	if *item.Duration == ForeverBlockDuration {
		return nil
	}

	newDuration := *item.Duration * 2
	item.Duration = &newDuration
	item.PeerID = peerId

	if *item.Duration > MaxBlockDuration {
		*item.Duration = ForeverBlockDuration
	}

	bt, err = json.Marshal(item)
	if err != nil {
		return err
	}

	if err := txn.SetWithTTL(blocklistKey, bt, *item.Duration); err != nil {
		return err
	}

	return txn.Commit()
}

func (d *NodeRepo) IsBlocklisted(peerId warpnet.WarpPeerID) (bool, error) {
	if d == nil {
		return false, ErrNilNodeRepo
	}
	if peerId == "" {
		return false, nil
	}
	blocklistKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddRootID(peerId.String()).
		Build()
	_, err := d.db.Get(blocklistKey)

	if errors.Is(err, local.ErrKeyNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *NodeRepo) BlocklistRemove(peerId warpnet.WarpPeerID) (err error) {
	if d == nil {
		return ErrNilNodeRepo
	}
	if peerId == "" {
		return local.DBError("empty peer ID")
	}
	blocklistKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddRootID(peerId.String()).
		Build()

	err = d.db.Delete(blocklistKey)
	if errors.Is(err, local.ErrKeyNotFound) {
		return nil
	}
	return err
}
