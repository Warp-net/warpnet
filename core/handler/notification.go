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
	ListSince(userId, since string, limit *uint64) ([]domain.Notification, string, error)
	UnreadCount(userId string) (uint64, error)
}

type NotifierGetter interface {
	Get(userId, notificationId string) (domain.Notification, error)
}

type NotifierMarker interface {
	MarkRead(userId, notificationId string) error
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

		var (
			notifications []domain.Notification
			cur           string
		)
		if ev.Since != nil && *ev.Since != "" {
			notifications, cur, err = repo.ListSince(owner.UserId, *ev.Since, ev.Limit)
		} else {
			notifications, cur, err = repo.List(owner.UserId, ev.Limit, ev.Cursor)
		}
		if err != nil {
			return nil, err
		}

		// Unread count must reflect ALL of the user's notifications,
		// not just the page returned above; otherwise the SideNav
		// "N unread" badge flickers between page-local counts as the
		// front-end re-polls every 2 s.
		unreadCount, err := repo.UnreadCount(owner.UserId)
		if err != nil {
			log.Errorf("notification handler: unread count: %v", err)
			// Fall back to the page-local count instead of zero so a
			// transient db hiccup doesn't drop the badge to 0 — it
			// still lags reality by whatever lives off-page, but a
			// stale > 0 is closer than a confidently wrong 0.
			for _, n := range notifications {
				if !n.IsRead {
					unreadCount++
				}
			}
		}
		sort.SliceStable(notifications, func(i, j int) bool {
			if notifications[i].IsRead != notifications[j].IsRead {
				return !notifications[i].IsRead
			}
			return notifications[i].CreatedAt.After(notifications[j].CreatedAt)
		})
		return event.GetNotificationsResponse{
			Cursor:        cur,
			UnreadCount:   unreadCount,
			Notifications: notifications,
		}, nil
	}
}

func StreamGetNotificationHandler(
	repo NotifierGetter,
	authRepo NotifierAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetNotificationEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.NotificationId == "" {
			return nil, warpnet.WarpError("notification: empty notification id")
		}

		owner := authRepo.GetOwner()

		not, err := repo.Get(owner.UserId, ev.NotificationId)
		if err != nil {
			return nil, err
		}
		return event.GetNotificationResponse(not), nil
	}
}

func StreamMarkNotificationReadHandler(
	repo NotifierMarker,
	authRepo NotifierAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.MarkNotificationReadEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.NotificationId == "" {
			return nil, warpnet.WarpError("notification: empty notification id")
		}
		owner := authRepo.GetOwner()
		if err := repo.MarkRead(owner.UserId, ev.NotificationId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}
