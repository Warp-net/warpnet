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

package node

import (
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

// newUserNotifier records a notification for the local owner.
type newUserNotifier interface {
	Add(not domain.Notification) error
}

// notifyingUserRepo decorates UserProvider so that persisting a genuinely new
// user also records a "new user discovered" notification for the owner.
// UserRepo.Create is the single choke point for caching remote users (both
// discovery and the user-list sync handler funnel through it and it returns
// ErrUserAlreadyExists for duplicates), so hooking Create here covers every
// path without scattering notification logic across handlers.
type notifyingUserRepo struct {
	UserProvider

	notifier    newUserNotifier
	ownerUserId string
}

func newNotifyingUserRepo(inner UserProvider, notifier newUserNotifier, ownerUserId string) *notifyingUserRepo {
	return &notifyingUserRepo{
		UserProvider: inner,
		notifier:     notifier,
		ownerUserId:  ownerUserId,
	}
}

func (r *notifyingUserRepo) Create(user domain.User) (domain.User, error) {
	created, err := r.UserProvider.Create(user)
	if err != nil {
		// Includes ErrUserAlreadyExists — not a new user, so no notification.
		return created, err
	}
	if user.Id == r.ownerUserId {
		return created, nil
	}
	name := user.Username
	if name == "" {
		name = user.Id
	}
	if nerr := r.notifier.Add(domain.Notification{
		Type:   domain.NotificationNewUserType,
		Text:   name + " joined Warpnet",
		UserId: r.ownerUserId,
	}); nerr != nil {
		log.Warnf("member: notify new user: %v", nerr)
	}
	return created, nil
}
