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

// NodeRating is what a node reports about a peer's standing, or about
// its own. A node's own rating is assembled entirely from records
// written by others — it has no opinion of itself to report.
type NodeRating struct {
	NodeID     string            `json:"node_id"`
	Overall    int32             `json:"overall"`
	Band       string            `json:"band"`
	Dimensions []DimensionRating `json:"dimensions"`
	Observers  int               `json:"observers"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type DimensionRating struct {
	Name   string         `json:"name"`
	Score  int32          `json:"score"`
	Band   string         `json:"band"`
	Recent []OffenceTally `json:"recent"`
}

// OffenceTally is a raw, undecayed count: it answers "what is my node
// being marked for", which is what a user needs to fix it.
type OffenceTally struct {
	Kind   string    `json:"kind"`
	Count  uint32    `json:"count"`
	LastAt time.Time `json:"last_at"`
}
