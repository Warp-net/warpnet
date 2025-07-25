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

import (
	"encoding/binary"
	"errors"
	"github.com/Warp-net/warpnet/database/local"
)

const (
	LikeRepoName      = "/LIKE"
	IncrSubNamespace  = "INCR"
	LikerSubNamespace = "LIKER"
)

var ErrLikesNotFound = local.DBError("like not found")

type LikeStorer interface {
	Get(key local.DatabaseKey) ([]byte, error)
	NewTxn() (local.WarpTransactioner, error)
}

type LikeRepo struct {
	db LikeStorer
}

func NewLikeRepo(db LikeStorer) *LikeRepo {
	return &LikeRepo{db: db}
}

func (repo *LikeRepo) Like(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local.DBError("empty user id")
	}

	likeKey := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()

	likerKey := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		AddRange(local.NoneRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	_, err = txn.Get(likerKey)
	if !errors.Is(err, local.ErrKeyNotFound) {
		_ = txn.Commit()
		return repo.LikesCount(tweetId) // like exists
	}

	if err = txn.Set(likerKey, []byte(userId)); err != nil {
		return 0, err
	}
	likesCount, err = txn.Increment(likeKey)
	if err != nil {
		return 0, err
	}
	return likesCount, txn.Commit()
}

func (repo *LikeRepo) Unlike(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local.DBError("empty user id")
	}

	likeKey := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()

	likerKey := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		AddRange(local.NoneRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	_, err = txn.Get(likerKey)
	if errors.Is(err, local.ErrKeyNotFound) { // already unliked
		_ = txn.Commit()
		return repo.LikesCount(tweetId)
	}
	if err = txn.Delete(likerKey); err != nil {
		return 0, err
	}
	likesCount, err = txn.Decrement(likeKey)
	if errors.Is(err, local.ErrKeyNotFound) {
		return 0, txn.Commit()
	}
	if err != nil {
		return 0, err
	}
	return likesCount, txn.Commit()
}

func (repo *LikeRepo) LikesCount(tweetId string) (likesNum uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	likeKey := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()

	bt, err := repo.db.Get(likeKey)
	if errors.Is(err, local.ErrKeyNotFound) {
		return 0, ErrLikesNotFound
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bt), nil
}

type likedUserIDs = []string

func (repo *LikeRepo) Likers(tweetId string, limit *uint64, cursor *string) (_ likedUserIDs, cur string, err error) {
	if tweetId == "" {
		return nil, "", local.DBError("empty tweet id")
	}

	likePrefix := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(likePrefix, limit, cursor)
	if errors.Is(err, local.ErrKeyNotFound) {
		return nil, "", ErrLikesNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	likers := make(likedUserIDs, 0, len(items))
	for _, item := range items {
		userId := string(item.Value)
		likers = append(likers, userId)
	}
	return likers, cur, nil
}
