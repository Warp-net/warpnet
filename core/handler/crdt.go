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
	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
)

// CRDTStatsProvider provides access to CRDT statistics
type CRDTStatsProvider interface {
	GetTweetStats(tweetID string) (*crdt.TweetStats, error)
}

// CRDTStatsRequest represents a request for tweet statistics
type CRDTStatsRequest struct {
	TweetID string `json:"tweet_id"`
}

// StreamGetCRDTStatsHandler returns a handler for retrieving CRDT-based tweet statistics
func StreamGetCRDTStatsHandler(statsProvider CRDTStatsProvider) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var req CRDTStatsRequest
		if err := json.Unmarshal(buf, &req); err != nil {
			return nil, err
		}

		if req.TweetID == "" {
			return nil, warpnet.WarpError("empty tweet id")
		}

		stats, err := statsProvider.GetTweetStats(req.TweetID)
		if err != nil {
			return nil, err
		}

		return stats, nil
	}
}
