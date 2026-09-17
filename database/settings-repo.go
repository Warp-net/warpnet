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
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

const SettingsRepoName = "/SETTINGS"

type SettingsStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type SettingsRepo struct {
	db SettingsStorer
}

func NewSettingsRepo(db SettingsStorer) *SettingsRepo {
	return &SettingsRepo{db: db}
}

func settingsKey(userId, kind string) local_store.DatabaseKey {
	root := local_store.NewPrefixBuilder(SettingsRepoName).AddRootID(userId)
	if kind == "" {
		return root.Build()
	}
	return root.AddParentId(kind).Build()
}

// GetNotificationSettings returns the user's notification settings, or a
// zero-value (email disabled) record when none has been saved yet.
func (repo *SettingsRepo) GetNotificationSettings(userId string) (domain.NotificationSettings, error) {
	if userId == "" {
		return domain.NotificationSettings{}, local_store.DBError("empty user id")
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(settingsKey(userId, ""))
	if local_store.IsNotFoundError(err) {
		return domain.NotificationSettings{}, nil
	}
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.NotificationSettings{}, err
	}
	var s domain.NotificationSettings
	if err := json.Unmarshal(bt, &s); err != nil {
		return domain.NotificationSettings{}, err
	}
	return s, nil
}

// SetNotificationSettings persists the user's notification settings.
func (repo *SettingsRepo) SetNotificationSettings(userId string, s domain.NotificationSettings) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	bt, err := json.Marshal(s)
	if err != nil {
		return err
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err := txn.Set(settingsKey(userId, ""), bt); err != nil {
		return err
	}
	return txn.Commit()
}

// GetGatewaySettings returns the user's gateway settings, or a zero-value
// (empty NodeID) record when none has been saved yet.
func (repo *SettingsRepo) GetGatewaySettings(userId string) (domain.GatewaySettings, error) {
	if userId == "" {
		return domain.GatewaySettings{}, local_store.DBError("empty user id")
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.GatewaySettings{}, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(settingsKey(userId, "gateway"))
	if local_store.IsNotFoundError(err) {
		return domain.GatewaySettings{}, nil
	}
	if err != nil {
		return domain.GatewaySettings{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.GatewaySettings{}, err
	}
	var s domain.GatewaySettings
	if err := json.Unmarshal(bt, &s); err != nil {
		return domain.GatewaySettings{}, err
	}
	return s, nil
}

// SetGatewaySettings persists the user's gateway settings.
func (repo *SettingsRepo) SetGatewaySettings(userId string, s domain.GatewaySettings) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	bt, err := json.Marshal(s)
	if err != nil {
		return err
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err := txn.Set(settingsKey(userId, "gateway"), bt); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *SettingsRepo) GetRateLimitSettings(userId string) (event.RateLimitSettings, error) {
	if userId == "" {
		return event.RateLimitSettings{}, local_store.DBError("empty user id")
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return event.RateLimitSettings{}, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(settingsKey(userId, "ratelimit"))
	if local_store.IsNotFoundError(err) {
		return event.RateLimitSettings{}, nil
	}
	if err != nil {
		return event.RateLimitSettings{}, err
	}
	if err := txn.Commit(); err != nil {
		return event.RateLimitSettings{}, err
	}
	var s event.RateLimitSettings
	if err := json.Unmarshal(bt, &s); err != nil {
		return event.RateLimitSettings{}, err
	}
	return s, nil
}

func (repo *SettingsRepo) SetRateLimitSettings(userId string, s event.RateLimitSettings) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	bt, err := json.Marshal(s)
	if err != nil {
		return err
	}
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err := txn.Set(settingsKey(userId, "ratelimit"), bt); err != nil {
		return err
	}
	return txn.Commit()
}
