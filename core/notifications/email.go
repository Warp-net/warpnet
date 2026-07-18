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

package notifications

import (
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

// SettingsProvider resolves a user's notification settings.
type SettingsProvider interface {
	GetNotificationSettings(userId string) (domain.NotificationSettings, error)
}

// Sender delivers a single email using caller-supplied SMTP settings.
type Sender interface {
	Send(cfg domain.NotificationSettings, subject, body string) error
}

// EmailChannel emails a notification to the owner when their settings opt in
// for that notification type. Delivery is asynchronous and best-effort:
// emailing must never block or fail the in-app notification path.
type EmailChannel struct {
	settings SettingsProvider
	sender   Sender
}

func NewEmailChannel(settings SettingsProvider, sender Sender) *EmailChannel {
	return &EmailChannel{settings: settings, sender: sender}
}

func (c *EmailChannel) Deliver(n domain.Notification) error {
	go c.deliver(n)
	return nil
}

func (c *EmailChannel) deliver(n domain.Notification) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("notifications: email dispatch panic: %v", r)
		}
	}()
	if n.UserId == "" {
		return
	}
	cfg, err := c.settings.GetNotificationSettings(n.UserId)
	if err != nil {
		log.Warnf("notifications: load settings: %v", err)
		return
	}
	if !cfg.EmailEnabled || cfg.Recipient == "" || !cfg.Types[n.Type] {
		return
	}
	if err := c.sender.Send(cfg, "Warpnet: "+n.Type.String(), n.Text); err != nil {
		log.Warnf("notifications: send email: %v", err)
	}
}
