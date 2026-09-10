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

package domain

import "time"

// RatingRecord is one node's signed count of another node's offences
// within one hour bucket. Only ObserverId ever writes it.
type RatingRecord struct {
	PeerId     string         `json:"peer_id"`
	ObserverId string         `json:"observer_id"`
	Dimension  string         `json:"dimension"`
	Bucket     int64          `json:"bucket"` // unix hour
	Generation string         `json:"generation"`
	Offences   []OffenceCount `json:"offences"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Signature  string         `json:"signature"`
}

type OffenceCount struct {
	Kind  string `json:"kind"`
	Count uint32 `json:"count"`
}

type NodeRating struct {
	NodeId     string            `json:"node_id"`
	Overall    int32             `json:"overall"`
	Tier       string            `json:"tier"`
	Dimensions []DimensionRating `json:"dimensions"`
	Observers  int               `json:"observers"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type DimensionRating struct {
	Name   string         `json:"name"`
	Score  int32          `json:"score"`
	Tier   string         `json:"tier"`
	Recent []OffenceTally `json:"recent"`
}

type OffenceTally struct {
	Kind   string    `json:"kind"`
	Count  uint32    `json:"count"`
	LastAt time.Time `json:"last_at"`
}
