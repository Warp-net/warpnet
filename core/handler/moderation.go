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
	log "github.com/sirupsen/logrus"
)

type ModerationNotifier interface {
	Add(not domain.Notification) error
}

func StreamModerationResultHandler(repo ModerationNotifier) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.ModerationResultEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.Result == domain.OK {
			return event.Accepted, nil
		}

		notificationText := "moderation result: "
		if ev.ObjectID != nil {
			notificationText += *ev.ObjectID + ": "
		}
		if ev.Reason != nil {
			notificationText += *ev.Reason
		}

		// TODO: handle notification
		log.Infof("moderation: result received: %s", notificationText)
		return event.Accepted, repo.Add(domain.Notification{
			Type:   domain.NotificationModerationType,
			Text:   notificationText,
			UserId: ev.UserID,
			IsRead: false,
		})
	}
}
