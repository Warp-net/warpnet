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
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

// SubscriptionStorer is the slice of UserSetRepo the subscribe handlers
// need. Subscribe = "tell me about new tweets from this user"; the local
// node uses this watchlist to elevate the corresponding incoming tweet
// to a notification.
type SubscriptionStorer interface {
	Add(ownerId, targetId string) error
	Remove(ownerId, targetId string) error
}

func StreamSubscribeUserHandler(repo SubscriptionStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.SubscribeUserEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.SelfId == "" {
			return nil, warpnet.WarpError("subscribe: empty self id")
		}
		if ev.TargetId == "" {
			return nil, warpnet.WarpError("subscribe: empty target id")
		}
		if ev.SelfId == ev.TargetId {
			return nil, warpnet.WarpError("subscribe: cannot subscribe to yourself")
		}
		if err := repo.Add(ev.SelfId, ev.TargetId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamUnsubscribeUserHandler(repo SubscriptionStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnsubscribeUserEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.SelfId == "" {
			return nil, warpnet.WarpError("unsubscribe: empty self id")
		}
		if ev.TargetId == "" {
			return nil, warpnet.WarpError("unsubscribe: empty target id")
		}
		if err := repo.Remove(ev.SelfId, ev.TargetId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}
