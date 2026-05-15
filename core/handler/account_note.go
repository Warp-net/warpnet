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

// AccountNoteStorer is the slice of UserNoteRepo the per-target-note
// handlers need.
type AccountNoteStorer interface {
	SetNote(selfId, targetId, note string) error
	GetNote(selfId, targetId string) (string, error)
}

func StreamUpdateAccountNoteHandler(repo AccountNoteStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UpdateAccountNoteEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.SelfId == "" {
			return nil, warpnet.WarpError("account note: empty self id")
		}
		if ev.TargetUserId == "" {
			return nil, warpnet.WarpError("account note: empty target user id")
		}
		// Note may be empty — that's how a user clears a previously-set note.
		if err := repo.SetNote(ev.SelfId, ev.TargetUserId, ev.Note); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamGetAccountNoteHandler(repo AccountNoteStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAccountNoteEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.SelfId == "" {
			return nil, warpnet.WarpError("account note: empty self id")
		}
		if ev.TargetUserId == "" {
			return nil, warpnet.WarpError("account note: empty target user id")
		}
		note, err := repo.GetNote(ev.SelfId, ev.TargetUserId)
		if err != nil {
			return nil, err
		}
		return event.GetAccountNoteResponse{Note: note}, nil
	}
}
