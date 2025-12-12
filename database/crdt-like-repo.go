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

package database

import (
"github.com/Warp-net/warpnet/core/crdt"
"github.com/Warp-net/warpnet/database/local"
)

// CRDTLikeRepo manages like statistics using CRDT
type CRDTLikeRepo struct {
db        LikeStorer
crdtStore *crdt.CRDTStatsStore
}

// NewCRDTLikeRepo creates a new CRDT-enabled like statistics repository
func NewCRDTLikeRepo(db LikeStorer, crdtStore *crdt.CRDTStatsStore) *CRDTLikeRepo {
return &CRDTLikeRepo{
db:        db,
crdtStore: crdtStore,
}
}

// IncrementLikes increments the like counter for a tweet
func (repo *CRDTLikeRepo) IncrementLikes(tweetId string) error {
if tweetId == "" {
return local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeLikes)
return repo.crdtStore.Put(tweetId, crdt.StatTypeLikes, current+1)
}

return nil
}

// DecrementLikes decrements the like counter for a tweet
func (repo *CRDTLikeRepo) DecrementLikes(tweetId string) error {
if tweetId == "" {
return local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeLikes)
if current > 0 {
return repo.crdtStore.Put(tweetId, crdt.StatTypeLikes, current-1)
}
}

return nil
}

// GetLikesCount returns the aggregated like count from CRDT
func (repo *CRDTLikeRepo) GetLikesCount(tweetId string) (uint64, error) {
if tweetId == "" {
return 0, local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeLikes)
}

return 0, nil
}
