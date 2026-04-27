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
	DevicesRepoName = "/DEVICES"
)

var ErrNilDevicesRepo = local_store.DBError("devices repo is nil")

type DevicesStorer interface {
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	NewTxn() (local_store.WarpTransactioner, error)
}

type DevicesRepo struct {
	db DevicesStorer
}

func NewDevicesRepo(db DevicesStorer) *DevicesRepo {
	return &DevicesRepo{db: db}
}

func (repo *DevicesRepo) GetDevices(ownerNodeId string) (devices []domain.Device, err error) {
	if repo.db == nil {
		return nil, ErrNilDevicesRepo
	}

	devicesPrefix := local_store.NewPrefixBuilder(DevicesRepoName).
		AddRootID(ownerNodeId).
		AddRange(local_store.NoneRangeKey).
		Build()

	tx, err := repo.db.NewTxn()
	if err != nil {
		return devices, err
	}

	limit := uint64(10)
	items, _, err := tx.List(devicesPrefix, &limit, nil)
	if err != nil {
		return devices, err
	}
	if len(items) == 0 {
		return devices, nil
	}

	for _, item := range items {
		var d domain.Device
		err = json.Unmarshal(item.Value, &d)
		if err != nil {
			return devices, err
		}
		devices = append(devices, d)
	}
	return devices, nil
}

func (repo *DevicesRepo) SetDevice(ownerNodeId string, device domain.Device) error {
	if repo.db == nil {
		return ErrNilDevicesRepo
	}
	if device.ID == "" {
		device.ID = ulid.Make().String()
	}
	if device.CreatedAt.IsZero() {
		device.CreatedAt = time.Now()
	}
	deviceKey := local_store.NewPrefixBuilder(DevicesRepoName).
		AddRootID(ownerNodeId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(device.ID).
		Build()

	data, err := json.Marshal(device)
	if err != nil {
		return err
	}

	return repo.db.SetWithTTL(deviceKey, data, time.Hour*72)
}
