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
	"strings"
	"time"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	LikeRepoName      = "/LIKES"
	IncrSubNamespace  = "INCR"
	LikerSubNamespace = "LIKER"
	LikedSubNamespace = "LIKED" // per-user index of liked tweet refs
	ReactSubNamespace = "REACT" // per-emoji counters, one key per reaction
)

// maxReactionKinds bounds one page of the per-emoji tally. Beyond it the
// breakdown is reported as-is instead of folding the remainder into
// DefaultReaction, which would inflate hearts.
const maxReactionKinds = uint64(128)

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

func (repo *LikeRepo) Like(tweetId, userId, emoji string, isTransitive bool) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local_store.DBError("empty user id")
	}
	emoji, err = domain.NormalizeReaction(emoji)
	if err != nil {
		return 0, err
	}

	likeKey := likesCountKey(tweetId)
	reactorKey := likerKey(tweetId, userId)

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	prev, err := txn.Get(reactorKey)
	switch {
	case err == nil:
		return repo.switchReaction(txn, reactorKey, tweetId, storedReaction(prev, userId), emoji, isTransitive)
	case local_store.IsNotFoundError(err):
		return repo.addReaction(txn, reactorKey, likeKey, tweetId, emoji, isTransitive)
	default:
		return 0, err
	}
}

// addReaction records a user's first reaction on a tweet: the total counter
// and the emoji's own counter both go up by one. Commits txn.
func (repo *LikeRepo) addReaction(
	txn local_store.WarpTransactioner,
	reactorKey, likeKey local_store.DatabaseKey,
	tweetId, emoji string,
	isTransitive bool,
) (likesCount uint64, err error) {
	if err = txn.Set(reactorKey, []byte(emoji)); err != nil {
		return 0, err
	}
	likesCount, err = txn.Increment(likeKey)
	if err != nil {
		return 0, err
	}
	if _, err = txn.Increment(reactionCountKey(tweetId, emoji)); err != nil {
		return 0, err
	}
	if err = txn.Commit(); err != nil {
		return 0, err
	}
	if repo.statsDb == nil || !isTransitive {
		return likesCount, nil
	}
	if err := repo.statsDb.Increment(likeKey.DatastoreKey()); err != nil {
		log.Warnf("like: stats db increment: %v", err)
	}
	if err := repo.statsDb.Increment(reactionCountKey(tweetId, emoji).DatastoreKey()); err != nil {
		log.Warnf("like: stats db reaction increment: %v", err)
	}
	return likesCount, nil
}

// switchReaction moves an existing reaction to a different emoji. The like
// itself stays, so only the per-emoji tallies move and the total counter is
// left alone. Commits txn.
func (repo *LikeRepo) switchReaction(
	txn local_store.WarpTransactioner,
	reactorKey local_store.DatabaseKey,
	tweetId, prevEmoji, emoji string,
	isTransitive bool,
) (likesCount uint64, err error) {
	if prevEmoji == emoji { // nothing to move
		_ = txn.Commit()
		return repo.LikesCount(tweetId)
	}
	if err = txn.Set(reactorKey, []byte(emoji)); err != nil {
		return 0, err
	}
	if _, err = txn.Decrement(reactionCountKey(tweetId, prevEmoji)); err != nil {
		return 0, err
	}
	if _, err = txn.Increment(reactionCountKey(tweetId, emoji)); err != nil {
		return 0, err
	}
	if err = txn.Commit(); err != nil {
		return 0, err
	}
	repo.moveReactionStat(tweetId, prevEmoji, emoji, isTransitive)
	return repo.LikesCount(tweetId)
}

// moveReactionStat mirrors a switch into the network-wide (CRDT) per-emoji
// counters. Best effort: the local counters are already committed and back
// the read-time fallback.
func (repo *LikeRepo) moveReactionStat(tweetId, from, to string, isTransitive bool) {
	if repo.statsDb == nil || !isTransitive {
		return
	}
	if err := repo.statsDb.Decrement(reactionCountKey(tweetId, from).DatastoreKey()); err != nil {
		log.Warnf("like: stats db reaction decrement: %v", err)
	}
	if err := repo.statsDb.Increment(reactionCountKey(tweetId, to).DatastoreKey()); err != nil {
		log.Warnf("like: stats db reaction increment: %v", err)
	}
}

func (repo *LikeRepo) Unlike(tweetId, userId string, isTransitive bool) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local_store.DBError("empty user id")
	}

	unlikeKey := likesCountKey(tweetId)
	unlikerKey := likerKey(tweetId, userId)

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	prev, err := txn.Get(unlikerKey)
	if local_store.IsNotFoundError(err) { // already unliked
		_ = txn.Commit()
		return repo.LikesCount(tweetId)
	}
	if err != nil {
		return 0, err
	}
	emoji := storedReaction(prev, userId)
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
	if _, err = txn.Decrement(reactionCountKey(tweetId, emoji)); err != nil {
		return 0, err
	}
	if err := txn.Commit(); err != nil {
		return 0, err
	}
	if repo.statsDb == nil || !isTransitive {
		return likesCount, nil
	}

	if err := repo.statsDb.Decrement(unlikeKey.DatastoreKey()); err != nil {
		log.Warnf("unlike: stats db decrement: %v", err)
	}
	if err := repo.statsDb.Decrement(reactionCountKey(tweetId, emoji).DatastoreKey()); err != nil {
		log.Warnf("unlike: stats db reaction decrement: %v", err)
	}

	return likesCount, nil
}

// Reactions returns the per-emoji tally for a tweet. Likes stored before
// reactions existed carry no per-emoji counter, so whatever the total
// counter holds beyond the sum of the named emoji is attributed to
// DefaultReaction — old hearts stay hearts.
func (repo *LikeRepo) Reactions(tweetId string) (map[string]uint64, error) {
	if tweetId == "" {
		return nil, local_store.DBError("empty tweet id")
	}

	prefix := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(ReactSubNamespace).
		AddRootID(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, err
	}
	defer txn.Rollback()

	limit := maxReactionKinds
	items, _, err := txn.List(prefix, &limit, nil)
	if err != nil && !local_store.IsNotFoundError(err) {
		return nil, err
	}
	if err = txn.Commit(); err != nil {
		return nil, err
	}

	var (
		reactions = make(map[string]uint64, len(items))
		named     uint64
	)
	for _, item := range items {
		emoji := keyID(item.Key)
		if emoji == "" {
			continue
		}
		count := decodeCount(item.Value)
		if repo.statsDb != nil {
			if total, statErr := repo.statsDb.GetAggregatedStat(
				local_store.DatabaseKey(item.Key).DatastoreKey(),
			); statErr == nil {
				count = total
			}
		}
		if count == 0 {
			continue
		}
		reactions[emoji] = count
		named += count
	}
	if uint64(len(items)) >= limit { // page truncated, remainder is unknowable
		return reactions, nil
	}

	total, err := repo.LikesCount(tweetId)
	if err != nil && !errors.Is(err, ErrLikesNotFound) {
		return nil, err
	}
	if total > named {
		reactions[domain.DefaultReaction] += total - named
	}
	return reactions, nil
}

// Reaction reports the emoji this user put on the tweet, or an empty
// string when they haven't reacted to it.
func (repo *LikeRepo) Reaction(tweetId, userId string) (string, error) {
	if tweetId == "" {
		return "", local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return "", local_store.DBError("empty user id")
	}

	bt, err := repo.db.Get(likerKey(tweetId, userId))
	if local_store.IsNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return storedReaction(bt, userId), nil
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
		// The value holds the reaction emoji, so the liker comes from the
		// key — which is also where pre-reaction rows carry it.
		userId := keyID(item.Key)
		if userId == "" {
			continue
		}
		likers = append(likers, userId)
	}
	return likers, cur, nil
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

	lt := domain.LikedTweet{
		UserId:      userId,
		TweetId:     tweetId,
		OwnerUserId: ownerUserId,
		CreatedAt:   time.Now(),
	}

	// Same fixed/sortable key pair as the chat message repo: the fixed key
	// gives deterministic lookup for unlike and is skipped by iteration,
	// the sortable key orders the list newest-liked-first.
	fixedKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikedSubNamespace).
		AddRootID(userId).
		AddRange(local_store.FixedRangeKey).
		AddParentId(tweetId).
		Build()

	sortableKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikedSubNamespace).
		AddRootID(userId).
		AddReversedTimestamp(lt.CreatedAt).
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

	if existing, err := txn.Get(fixedKey); err == nil && len(existing) != 0 {
		return txn.Commit() // already indexed, no-op
	}
	if err = txn.Set(fixedKey, sortableKey.Bytes()); err != nil {
		return err
	}
	if err = txn.Set(sortableKey, bt); err != nil {
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

	fixedKey := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikedSubNamespace).
		AddRootID(userId).
		AddRange(local_store.FixedRangeKey).
		AddParentId(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(fixedKey)
	if err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	if len(sortableKey) == 0 {
		return txn.Commit() // not indexed, no-op
	}
	if err = txn.Delete(fixedKey); err != nil {
		return err
	}
	if err = txn.Delete(local_store.DatabaseKey(sortableKey)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *LikeRepo) Liked(userId string, limit *uint64, cursor *string) ([]domain.LikedTweet, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("empty user id")
	}

	prefix := local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikedSubNamespace).
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

	liked := make([]domain.LikedTweet, 0, len(items))
	for _, item := range items {
		var lt domain.LikedTweet
		if err := json.Unmarshal(item.Value, &lt); err != nil {
			return nil, "", err
		}
		liked = append(liked, lt)
	}
	return liked, cur, nil
}

func likesCountKey(tweetId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()
}

func likerKey(tweetId, userId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(userId).
		Build()
}

func reactionCountKey(tweetId, emoji string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(ReactSubNamespace).
		AddRootID(tweetId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(emoji).
		Build()
}

// keyID returns the last segment of a database key — the liker's user id
// for a LIKER key, the emoji for a REACT key.
func keyID(key string) string {
	return key[strings.LastIndex(key, local_store.Delimeter)+1:]
}

// storedReaction decodes the emoji a liker row carries. Rows written before
// reactions existed stored the liker's own id as the value, so they read
// back as hearts.
func storedReaction(value []byte, userId string) string {
	emoji := string(value)
	if emoji == "" || emoji == userId {
		return domain.DefaultReaction
	}
	return emoji
}

// isTransitive tells whether this action should propagate to the network-wide
// (CRDT) counter, which is replicated ("transits") across nodes. The caller
// (handler) sets it true only on the acting user's own node, so an action
// observed on more than one node is counted once. The local per-node counter
// is always updated (it backs the read-time fallback).
//
// A user holds at most one reaction per tweet: reacting again with a
// different emoji moves the per-emoji tallies but leaves the like itself
// (and therefore the total counter) alone.

// decodeCount reads a counter value written by the store's Increment,
// tolerating a short or missing value.
func decodeCount(value []byte) uint64 {
	if len(value) < 8 { //nolint:mnd
		return 0
	}
	return binary.BigEndian.Uint64(value)
}
