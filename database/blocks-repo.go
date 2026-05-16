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

// NOT to be confused with NodeRepo.Blocklist (database/node-repo.go),
// which is a *peer*-level anti-abuse mechanism keyed by libp2p PeerID
// with escalating TTLs — that one bans misbehaving nodes from the
// network. BlocksRepo below is the *social* block: permanent,
// user-scoped "hide this account from me" keyed by domain user id.

const (
	BlocksRepoName = "/BLOCKS"

	// blockeesSubName indexes per-blocker the user ids they have blocked.
	blockeesSubName = "BLOCKEES"
	// blockersSubName indexes per-blockee the user ids who blocked them
	// (used when we need to evaluate inbound traffic).
	blockersSubName = "BLOCKERS"
)

// BlocksRepo persists the set of user ids the local owner has blocked.
type BlocksRepo struct {
	db userRelationStorer
}

func NewBlocksRepo(db userRelationStorer) *BlocksRepo { return &BlocksRepo{db: db} }

// Block records that blockerId has blocked blockeeId.
func (repo *BlocksRepo) Block(blockerId, blockeeId string) error {
	return addUserRelation(repo.db, BlocksRepoName, blockeesSubName, blockersSubName, blockerId, blockeeId)
}

// Unblock removes a previously-recorded block.
func (repo *BlocksRepo) Unblock(blockerId, blockeeId string) error {
	return removeUserRelation(repo.db, BlocksRepoName, blockeesSubName, blockersSubName, blockerId, blockeeId)
}

func (repo *BlocksRepo) IsBlocked(blockerId, blockeeId string) (bool, error) {
	return hasUserRelation(repo.db, BlocksRepoName, blockeesSubName, blockerId, blockeeId)
}

func (repo *BlocksRepo) List(blockerId string, limit *uint64, cursor *string) ([]string, string, error) {
	return listUserRelations(repo.db, BlocksRepoName, blockeesSubName, blockerId, limit, cursor)
}
