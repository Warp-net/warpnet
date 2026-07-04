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
	ReverseList(userId string, cursor *string, limit *uint64) ([]domain.Notification, string, error)
	UnreadCount(userId string) (uint64, error)
}

type NotifierGetter interface {
	Get(userId, notificationId string) (domain.Notification, error)
}

type NotifierMarker interface {
	MarkRead(userId, notificationId string) error
}

type NotifierAllMarker interface {
	MarkAllRead(userId string) error
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
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		owner := authRepo.GetOwner()
		notifications, cur, err := repo.List(owner.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		return notificationsResponse(repo, owner.UserId, notifications, cur), nil
	}
}

// StreamGetPushesHandler returns notifications newer than the given
// cursor (delta pull) via ReverseList, keeping the response cursor as a
// high-water mark. The plain notifications route stays on List so the desktop
// UI's older-page pagination (cursor -> end) is unaffected.
func StreamGetPushesHandler(
	repo NotifierFetcher,
	authRepo NotifierAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetNotificationsEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		owner := authRepo.GetOwner()
		notifications, cur, err := repo.ReverseList(owner.UserId, ev.Cursor, ev.Limit)
		if err != nil {
			return nil, err
		}
		return notificationsResponse(repo, owner.UserId, notifications, cur), nil
	}
}

// notificationsResponse attaches the all-notifications unread count (not the
// page-local count, which makes the badge flicker as the UI re-polls) and
// sorts unread-first, newest-first.
func notificationsResponse(
	repo NotifierFetcher,
	userId string,
	notifications []domain.Notification,
	cur string,
) event.GetNotificationsResponse {
	unreadCount, err := repo.UnreadCount(userId)
	if err != nil {
		log.Errorf("notification handler: unread count: %v", err)
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

// StreamMarkAllNotificationsReadHandler flips every unread notification for
// the owner in one round-trip. The UI's "Mark all as read" (and the
// open-the-notifications-view auto-read) used to page through
// PRIVATE_GET_NOTIFICATIONS and post per-id reads, which only ever covered
// the first page — unread items beyond it kept the badge alive forever.
func StreamMarkAllNotificationsReadHandler(
	repo NotifierAllMarker,
	authRepo NotifierAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(_ []byte, s warpnet.WarpStream) (any, error) {
		owner := authRepo.GetOwner()
		if err := repo.MarkAllRead(owner.UserId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}
