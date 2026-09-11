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

// RatingReader is the rating as these handlers read it: the public view
// of a peer, and what the network says about this node.
type RatingReader interface {
	View(peerID warpnet.WarpPeerID) (domain.NodeRating, error)
	Own() (domain.NodeRating, error)
}

// StreamGetOwnRatingHandler answers with what the network says about this
// node. A node holds no opinion of itself, so all of it was written by
// others.
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

// StreamGetRatingHandler answers with this node's public view of a peer:
// the unweighted median of what its observers concluded, which is not the
// view this node enforces on.
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
			return StreamGetOwnRatingHandler(reader)(buf, nil)
		}

		peerID := warpnet.FromStringToPeerID(ev.NodeId)
		if peerID == "" {
			return nil, warpnet.ErrMalformedNodeId
		}
		view, err := reader.View(peerID)
		if err != nil {
			log.Errorf("rating handler: reading standing of %s: %v", ev.NodeId, err)
			return nil, err
		}
		return event.GetRatingResponse(view), nil
	}
}
