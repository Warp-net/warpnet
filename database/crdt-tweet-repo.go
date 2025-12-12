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

// CRDTTweetRepo manages tweet statistics using CRDT (retweets and views)
type CRDTTweetRepo struct {
crdtStore *crdt.CRDTStatsStore
}

// NewCRDTTweetRepo creates a new CRDT-enabled tweet statistics repository
func NewCRDTTweetRepo(crdtStore *crdt.CRDTStatsStore) *CRDTTweetRepo {
return &CRDTTweetRepo{
crdtStore: crdtStore,
}
}

// IncrementRetweets increments the retweet counter for a tweet
func (repo *CRDTTweetRepo) IncrementRetweets(tweetId string) error {
if tweetId == "" {
return local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeRetweets)
return repo.crdtStore.Put(tweetId, crdt.StatTypeRetweets, current+1)
}

return nil
}

// DecrementRetweets decrements the retweet counter for a tweet
func (repo *CRDTTweetRepo) DecrementRetweets(tweetId string) error {
if tweetId == "" {
return local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeRetweets)
if current > 0 {
return repo.crdtStore.Put(tweetId, crdt.StatTypeRetweets, current-1)
}
}

return nil
}

// GetRetweetsCount returns the aggregated retweet count from CRDT
func (repo *CRDTTweetRepo) GetRetweetsCount(tweetId string) (uint64, error) {
if tweetId == "" {
return 0, local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeRetweets)
}

return 0, nil
}

// IncrementViews increments the view counter for a tweet
func (repo *CRDTTweetRepo) IncrementViews(tweetId string) error {
if tweetId == "" {
return local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeViews)
return repo.crdtStore.Put(tweetId, crdt.StatTypeViews, current+1)
}

return nil
}

// GetViewsCount returns the aggregated view count from CRDT
func (repo *CRDTTweetRepo) GetViewsCount(tweetId string) (uint64, error) {
if tweetId == "" {
return 0, local.DBError("empty tweet id")
}

if repo.crdtStore != nil {
return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeViews)
}

return 0, nil
}
