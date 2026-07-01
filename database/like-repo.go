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
	"time"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	LikeRepoName      = "/LIKES"
	IncrSubNamespace  = "INCR"
	LikerSubNamespace = "LIKER"
	likedListSub      = "LIKED"     // forward index: per-user cursor of liked tweet refs
	likedItemSub      = "LIKEDITEM" // stable lookup by tweet id for unlike
)

var ErrLikesNotFound = local_store.DBError("like not found")

type LikeStorer interface {
	Get(key local_store.DatabaseKey) ([]byte, error)
	NewTxn() (local_store.WarpTransactioner, error)
}

type LikeStatsStorer interface {
	GetAggregatedStat(key ds.Key) (uint64, error)
	Increment(key ds.Key) error
	Decrement(key ds.Key) error
}

type LikeRepo struct {
	db      LikeStorer
	statsDb LikeStatsStorer
}

func NewLikeRepo(db LikeStorer, statsDb LikeStatsStorer) *LikeRepo {
	return &LikeRepo{db: db, statsDb: statsDb}
}

func (repo *LikeRepo) Like(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local_store.DBError("empty user id")
	}

	likeKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()

	likerKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	_, err = txn.Get(likerKey)
	if !local_store.IsNotFoundError(err) {
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
	if err = txn.Commit(); err != nil {
		return 0, err
	}
	if repo.statsDb == nil {
		return likesCount, nil
	}
	if err := repo.statsDb.Increment(likeKey.DatastoreKey()); err != nil {
		log.Warnf("like: stats db increment: %v", err)
	}
	return likesCount, nil
}

func (repo *LikeRepo) Unlike(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local_store.DBError("empty user id")
	}

	unlikeKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()

	unlikerKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	_, err = txn.Get(unlikerKey)
	if local_store.IsNotFoundError(err) { // already unliked
		_ = txn.Commit()
		return repo.LikesCount(tweetId)
	}
	if err = txn.Delete(unlikerKey); err != nil {
		return 0, err
	}
	likesCount, err = txn.Decrement(unlikeKey)
	if local_store.IsNotFoundError(err) {
		return 0, txn.Commit()
	}
	if err != nil {
		return 0, err
	}
	if err := txn.Commit(); err != nil {
		return 0, err
	}
	if repo.statsDb == nil {
		return likesCount, nil
	}

	if err := repo.statsDb.Decrement(unlikeKey.DatastoreKey()); err != nil {
		log.Warnf("unlike: stats db decrement: %v", err)
	}

	return likesCount, nil
}

func (repo *LikeRepo) LikesCount(tweetId string) (likesNum uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	likeKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()

	if repo.statsDb != nil {
		total, err := repo.statsDb.GetAggregatedStat(likeKey.DatastoreKey())
		if err == nil {
			return total, nil
		}
		log.Warnf("get likes stat: %v", err)
	}

	bt, err := repo.db.Get(likeKey)
	if local_store.IsNotFoundError(err) {
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
		return nil, "", local_store.DBError("empty tweet id")
	}

	likePrefix := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(likePrefix, limit, cursor)
	if local_store.IsNotFoundError(err) {
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

// LikedTweet is one entry of a user's "tweets I liked" index. The tweet
// author's id is stored alongside so the client can fetch the tweet without
// an extra resolution round-trip (same trick as database.Bookmark).
type LikedTweet struct {
	UserId      string    `json:"user_id"`
	TweetId     string    `json:"tweet_id"`
	OwnerUserId string    `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func (repo *LikeRepo) SetLiked(userId, tweetId, ownerUserId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	if ownerUserId == "" {
		return local_store.DBError("empty owner user id")
	}

	lt := LikedTweet{
		UserId:      userId,
		TweetId:     tweetId,
		OwnerUserId: ownerUserId,
		CreatedAt:   time.Now(),
	}

	// list-key: ordered by creation time (reverse so newest comes first
	// when iterating); item-key: stable lookup by tweet id for unlike.
	listKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(likedListSub).
		AddRootID(userId).
		AddReversedTimestamp(lt.CreatedAt).
		AddParentId(tweetId).
		Build()

	itemKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(likedItemSub).
		AddRootID(userId).
		AddParentId(tweetId).
		Build()

	bt, err := json.Marshal(lt)
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if existing, err := txn.Get(itemKey); err == nil && len(existing) != 0 {
		return txn.Commit() // already indexed, no-op
	}
	if err = txn.Set(itemKey, []byte(listKey)); err != nil {
		return err
	}
	if err = txn.Set(listKey, bt); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *LikeRepo) RemoveLiked(userId, tweetId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}

	itemKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(likedItemSub).
		AddRootID(userId).
		AddParentId(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	listKeyBytes, err := txn.Get(itemKey)
	if local_store.IsNotFoundError(err) {
		return txn.Commit()
	}
	if err != nil {
		return err
	}
	if err = txn.Delete(itemKey); err != nil {
		return err
	}
	if len(listKeyBytes) != 0 {
		if err = txn.Delete(local_store.DatabaseKey(listKeyBytes)); err != nil {
			return err
		}
	}
	return txn.Commit()
}

func (repo *LikeRepo) Liked(userId string, limit *uint64, cursor *string) ([]LikedTweet, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("empty user id")
	}

	prefix := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(likedListSub).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	liked := make([]LikedTweet, 0, len(items))
	for _, item := range items {
		var lt LikedTweet
		if err := json.Unmarshal(item.Value, &lt); err != nil {
			return nil, "", err
		}
		liked = append(liked, lt)
	}
	return liked, cur, nil
}
