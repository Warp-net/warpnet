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

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package vote

import "github.com/Warp-net/warpnet/domain"

// Package vote is the moderator-to-moderator vote contract. It lives
// outside the shared event package because votes never leave the moderator
// fleet — member nodes publish reports and consume verdicts, but never send
// or read a vote — and outside the moderator package itself so the gossip
// layer that carries votes and the logic that produces them can both depend
// on it without an import cycle.

// Topic carries one Event per moderator per report round. Voter
// authenticity rides on the envelope (event.Message) signature, so the vote
// payload itself is unsigned.
const Topic = "/warpnet/moderation/votes/1.0.0"

// Event is a single moderator's verdict on one report round. Every
// moderator that assessed the report publishes one; the round's chair
// aggregates them into the final event.ModerationVerdictEvent.
type Event struct {
	ReportID string                      `json:"report_id"`
	Type     domain.ModerationObjectType `json:"type"`
	Result   domain.ModerationResult     `json:"result"`
	Reason   *string                     `json:"reason,omitempty"`
	UserID   domain.ID                   `json:"user_id"`
	ObjectID *domain.ID                  `json:"object_id,omitempty"`
	// ModeratorID is overwritten by the subscriber from the verified
	// envelope NodeId; the payload value is never trusted.
	ModeratorID domain.ID `json:"moderator_id,omitempty"`
	// Final marks the chair's (or a backup's) announcement that the round
	// was finalized. It cancels the deterministic takeover chain on every
	// other voter and is never counted as a vote.
	Final bool `json:"final,omitempty"`
}
