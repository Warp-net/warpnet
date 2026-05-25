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

package handler

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// ReportPublisher is the slice of the member pubsub provider this
// handler needs — published reports land on the moderator-facing topic.
type ReportPublisher interface {
	PublishReport(ev event.ReportEvent) error
}

// StreamReportHandler receives a PUBLIC_POST_REPORT call from a logged-in
// user (via the local Vue UI or warpdroid) and forwards it to the global
// reports gossip topic so any moderator node picks it up.
//
// The handler intentionally does not store reports locally — there's no
// audit log on the reporter's node. Trust comes from the libp2p
// signature on the envelope, which the auth middleware already verified
// before this code runs. Shape-level validation lives in
// event.ValidateReport so the moderator consumer can re-run the same
// check against the gossiped message.
func StreamReportHandler(publisher ReportPublisher) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.ReportEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		event.SanitizeReport(&ev)
		if err := event.ValidateReport(ev); err != nil {
			return nil, err
		}

		if err := publisher.PublishReport(ev); err != nil {
			log.Errorf("report: publish: %v", err)
			return nil, err
		}
		log.Infof("report: published type=%s target_user=%s reason=%q",
			ev.Type.String(), ev.TargetUserID, ev.Reason)
		return event.Accepted, nil
	}
}
