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
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const ErrRatingUnavailable = warpnet.WarpError("rating is not available on this node")

type RatingReader interface {
	Public(subject warpnet.WarpPeerID) (domain.NodeRating, error)
	Own() (domain.NodeRating, error)
}

func StreamGetOwnRatingHandler(reader RatingReader) warpnet.WarpHandlerFunc {
	return func(_ []byte, _ warpnet.WarpStream) (any, error) {
		if reader == nil {
			return nil, ErrRatingUnavailable
		}
		own, err := reader.Own()
		if err != nil {
			log.Errorf("rating handler: reading own standing: %v", err)
			return nil, err
		}
		return event.GetRatingResponse(own), nil
	}
}

func StreamGetRatingHandler(reader RatingReader) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		if reader == nil {
			return nil, ErrRatingUnavailable
		}

		var ev event.GetRatingEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.NodeId == "" {
			own, err := reader.Own()
			if err != nil {
				log.Errorf("rating handler: reading own standing: %v", err)
				return nil, err
			}
			return event.GetRatingResponse(own), nil
		}

		subject := warpnet.FromStringToPeerID(ev.NodeId)
		if subject == "" {
			return nil, warpnet.ErrMalformedNodeId
		}
		public, err := reader.Public(subject)
		if err != nil {
			log.Errorf("rating handler: reading standing of %s: %v", ev.NodeId, err)
			return nil, err
		}
		return event.GetRatingResponse(public), nil
	}
}
