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

// RatingReader is the read side of the rating store, declared here and
// kept to the two methods these handlers need — the same handler-local
// interface style every other handler in this package uses.
type RatingReader interface {
	Public(subject warpnet.WarpPeerID) (domain.NodeRating, error)
	Own() (domain.NodeRating, error)
}

// StreamGetOwnRatingHandler serves the owner their own node's standing.
//
// What comes back is the public aggregate, assembled entirely from
// records other nodes wrote about this one. A node holds no opinion of
// itself by construction, so there is nothing else it could report —
// which is the point: the user sees themselves as the network sees
// them, and the offence tallies say what to fix.
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

// StreamGetRatingHandler serves this node's view of some other node.
//
// It is a convenience for clients that hold no CRDT replica of their
// own — a paired phone, mostly. Full nodes read the CRDT directly, so
// nothing in the rating mechanism depends on this route being
// answered.
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
