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

// Package notifications encapsulates how a notification reaches a user.
// Feature code depends only on the Notifier interface; the concrete Service
// fans each notification out to a set of delivery Channels (in-app store,
// email, and any future medium) that it treats agnostically.
package notifications

import (
	"errors"

	"github.com/Warp-net/warpnet/domain"
)

// Notifier records a notification. Feature code (handlers, repos) depends on
// this and nothing else about how notifications are delivered.
type Notifier interface {
	Add(n domain.Notification) error
}

// Channel delivers a notification through one medium. A new delivery method
// is added by implementing Channel and registering it with a Service —
// nothing else in the system changes.
type Channel interface {
	Deliver(n domain.Notification) error
}

// Service fans a notification out to every registered Channel.
type Service struct {
	channels []Channel
}

func New(channels ...Channel) *Service {
	return &Service{channels: channels}
}

// Add delivers the notification to every channel, returning their joined
// errors. A failing channel never blocks the others, and callers treat
// notification errors as non-fatal (log-and-continue).
func (s *Service) Add(n domain.Notification) error {
	var errs []error
	for _, ch := range s.channels {
		if err := ch.Deliver(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
