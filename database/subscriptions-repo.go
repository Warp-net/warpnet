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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

const (
	// SubscriptionsRepoName holds the local watchlist: whose new tweets
	// I want notifications about.
	SubscriptionsRepoName = "/SUBSCRIPTIONS"

	// subscribedToSubName indexes per-subscriber the user ids they
	// subscribed to.
	subscribedToSubName = "SUBSCRIBED_TO"
	// subscribersSubName indexes per-target the subscribers of that
	// target — used when emitting notifications on a new tweet.
	subscribersSubName = "SUBSCRIBERS"
)

// SubscriptionsRepo persists the local "notify me about this user's new
// posts" watchlist.
type SubscriptionsRepo struct {
	db userRelationStorer
}

func NewSubscriptionsRepo(db userRelationStorer) *SubscriptionsRepo {
	return &SubscriptionsRepo{db: db}
}

func (repo *SubscriptionsRepo) Subscribe(selfId, targetUserId string) error {
	return addUserRelation(repo.db, SubscriptionsRepoName, subscribedToSubName, subscribersSubName, selfId, targetUserId)
}

func (repo *SubscriptionsRepo) Unsubscribe(selfId, targetUserId string) error {
	return removeUserRelation(repo.db, SubscriptionsRepoName, subscribedToSubName, subscribersSubName, selfId, targetUserId)
}

func (repo *SubscriptionsRepo) IsSubscribed(selfId, targetUserId string) (bool, error) {
	return hasUserRelation(repo.db, SubscriptionsRepoName, subscribedToSubName, selfId, targetUserId)
}
