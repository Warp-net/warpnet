package handler

import (
	"sort"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

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

type NotifierFetcher interface {
	List(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error)
}

type NotifierAuthStorer interface {
	GetOwner() domain.Owner
}

func StreamGetNotificationsHandler(
	repo NotifierFetcher,
	authRepo NotifierAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetNotificationsEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		owner := authRepo.GetOwner()

		notifications, cur, err := repo.List(owner.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		if len(notifications) != 0 {
			log.Infof("found %d notifications", len(notifications))
		}

		var unreadCount uint64
		sort.SliceStable(notifications, func(i, j int) bool {
			if !notifications[i].IsRead {
				unreadCount++
			}
			return notifications[i].IsRead
		})
		return event.GetNotificationsResponse{
			Cursor:        cur,
			UnreadCount:   unreadCount,
			Notifications: notifications,
		}, nil
	}
}
