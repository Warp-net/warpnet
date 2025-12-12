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

// CRDTReplyRepo manages reply statistics using CRDT
type CRDTReplyRepo struct {
crdtStore *crdt.CRDTStatsStore
}

// NewCRDTReplyRepo creates a new CRDT-enabled reply statistics repository
func NewCRDTReplyRepo(crdtStore *crdt.CRDTStatsStore) *CRDTReplyRepo {
return &CRDTReplyRepo{
crdtStore: crdtStore,
}
}

// IncrementReplies increments the reply counter for a tweet
func (repo *CRDTReplyRepo) IncrementReplies(tweetId string) error {
if tweetId == "" {
return local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeReplies)
return repo.crdtStore.Put(tweetId, crdt.StatTypeReplies, current+1)
}

return nil
}

// DecrementReplies decrements the reply counter for a tweet
func (repo *CRDTReplyRepo) DecrementReplies(tweetId string) error {
if tweetId == "" {
return local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeReplies)
if current > 0 {
return repo.crdtStore.Put(tweetId, crdt.StatTypeReplies, current-1)
}
}

return nil
}

// GetRepliesCount returns the aggregated reply count from CRDT
func (repo *CRDTReplyRepo) GetRepliesCount(tweetId string) (uint64, error) {
if tweetId == "" {
return 0, local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeReplies)
}

return 0, nil
}
