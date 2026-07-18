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

package handler

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

// SettingsStorer is the narrow surface the notification-settings handlers need.
type SettingsStorer interface {
	GetNotificationSettings(userId string) (domain.NotificationSettings, error)
	SetNotificationSettings(userId string, s domain.NotificationSettings) error
}

// SettingsAuthStorer resolves the local node owner.
type SettingsAuthStorer interface {
	GetOwner() domain.Owner
}

func StreamGetNotificationSettingsHandler(
	repo SettingsStorer,
	authRepo SettingsAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		owner := authRepo.GetOwner()
		settings, err := repo.GetNotificationSettings(owner.UserId)
		if err != nil {
			return nil, err
		}
		return event.GetNotificationSettingsResponse(settings), nil
	}
}

func StreamUpdateNotificationSettingsHandler(
	repo SettingsStorer,
	authRepo SettingsAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UpdateNotificationSettingsEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		owner := authRepo.GetOwner()
		if owner.UserId == "" {
			return nil, warpnet.WarpError("update notification settings: empty owner")
		}
		if err := repo.SetNotificationSettings(owner.UserId, ev); err != nil {
			return nil, err
		}
		return event.GetNotificationSettingsResponse(ev), nil
	}
}
