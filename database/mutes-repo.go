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
	MutesRepoName = "/MUTES"

	// muteesSubName indexes per-muter the user ids they have muted.
	muteesSubName = "MUTEES"
	// mutersSubName indexes per-mutee the user ids who muted them.
	mutersSubName = "MUTERS"
)

// MutesRepo persists the set of user ids the local owner has muted.
type MutesRepo struct {
	db userRelationStorer
}

func NewMutesRepo(db userRelationStorer) *MutesRepo { return &MutesRepo{db: db} }

func (repo *MutesRepo) Mute(muterId, muteeId string) error {
	return addUserRelation(repo.db, MutesRepoName, muteesSubName, mutersSubName, muterId, muteeId)
}

func (repo *MutesRepo) Unmute(muterId, muteeId string) error {
	return removeUserRelation(repo.db, MutesRepoName, muteesSubName, mutersSubName, muterId, muteeId)
}

func (repo *MutesRepo) IsMuted(muterId, muteeId string) (bool, error) {
	return hasUserRelation(repo.db, MutesRepoName, muteesSubName, muterId, muteeId)
}

func (repo *MutesRepo) List(muterId string, limit *uint64, cursor *string) ([]string, string, error) {
	return listUserRelations(repo.db, MutesRepoName, muteesSubName, muterId, limit, cursor)
}
