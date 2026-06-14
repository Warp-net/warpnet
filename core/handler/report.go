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
	"errors"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"strings"
)

const MaxReportReasonLen = 256

var (
	ErrReportNoTargetUser = errors.New("report: empty target_user_id")
	ErrReportNoTargetNode = errors.New("report: empty target_node_id")
	ErrReportNoReason     = errors.New("report: empty reason")
	ErrReportReasonLong   = errors.New("report: reason too long")
	ErrReportNoObjectID   = errors.New("report: empty object_id for tweet")
	ErrReportBadType      = errors.New("report: unsupported moderation object type")
)

type ReportPublisher interface {
	PublishReport(ev event.ReportEvent) error
}

func StreamReportHandler(publisher ReportPublisher) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.ReportEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}

		sanitizeReport(&ev)

		if err := validateReport(ev); err != nil {
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

func sanitizeReport(ev *event.ReportEvent) {
	if ev == nil {
		return
	}
	ev.Reason = strings.TrimSpace(ev.Reason)
	if ev.ObjectID != nil {
		trimmed := strings.TrimSpace(*ev.ObjectID)
		ev.ObjectID = &trimmed
	}
}

func validateReport(ev event.ReportEvent) error {
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
