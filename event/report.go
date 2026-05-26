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

package event

import (
	"errors"
	"strings"

	"github.com/Warp-net/warpnet/domain"
)

// MaxReportReasonLen caps the free-form reason on the wire. Reports
// are gossiped network-wide, so an unbounded reason field is a cheap
// way to bloat the topic and spam logs.
const MaxReportReasonLen = 256

// Errors are exported so callers can match against them with
// errors.Is — useful both in the publishing handler (which returns
// 4xx-style errors to the UI) and in the moderator consumer (which
// drops bad messages silently).
var (
	ErrReportNoTargetUser = errors.New("report: empty target_user_id")
	ErrReportNoTargetNode = errors.New("report: empty target_node_id")
	ErrReportNoReason     = errors.New("report: empty reason")
	ErrReportReasonLong   = errors.New("report: reason too long")
	ErrReportNoObjectID   = errors.New("report: empty object_id for tweet")
	ErrReportBadType      = errors.New("report: unsupported moderation object type")
)

// SanitizeReport normalizes the user-typed fields of a ReportEvent so
// that Validate (and downstream logs) see a canonical value. Mutates
// in place. Idempotent.
func SanitizeReport(ev *ReportEvent) {
	if ev == nil {
		return
	}
	ev.Reason = strings.TrimSpace(ev.Reason)
	if ev.ObjectID != nil {
		trimmed := domain.ID(strings.TrimSpace(string(*ev.ObjectID)))
		ev.ObjectID = &trimmed
	}
}

// ValidateReport returns nil if the event is shape-acceptable. Run
// SanitizeReport first so whitespace-only fields are caught.
//
// Called both by the member-side publishing handler (to reject bad
// client input) and by the moderator-side consumer (because the
// reports topic is open — a signed message is still untrusted
// content). Keep the two paths in sync by sharing this function.
func ValidateReport(ev ReportEvent) error {
	if ev.TargetUserID == "" {
		return ErrReportNoTargetUser
	}
	if ev.TargetNodeID == "" {
		return ErrReportNoTargetNode
	}
	if ev.Reason == "" {
		return ErrReportNoReason
	}
	if len(ev.Reason) > MaxReportReasonLen {
		return ErrReportReasonLong
	}
	switch ev.Type {
	case domain.ModerationTweetType:
		if ev.ObjectID == nil || *ev.ObjectID == "" {
			return ErrReportNoObjectID
		}
	case domain.ModerationUserType:
		// object_id is optional / unused for user reports
	default:
		// Reply / image reports are not wired end-to-end yet.
		return ErrReportBadType
	}
	return nil
}
