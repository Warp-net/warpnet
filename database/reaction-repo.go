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
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

const (
	ReactionRepoName    = "/REACTIONS"
	IncrSubNamespace    = "INCR"
	ReactorSubNamespace = "REACTOR"
	ReactedSubNamespace = "REACTED" // per-user index of reacted tweet refs
	EmojiSubNamespace   = "EMOJI"   // per-emoji counters, one key per reaction

)

var ErrReactionsNotFound = local_store.DBError("reaction not found")

type ReactionStorer interface {
	Get(key local_store.DatabaseKey) ([]byte, error)
	NewTxn() (local_store.WarpTransactioner, error)
}

type ReactionStatsStorer interface {
	GetAggregatedStat(key ds.Key) (uint64, error)
	Increment(key ds.Key) error
	Decrement(key ds.Key) error
}

type ReactionRepo struct {
	db      ReactionStorer
	statsDb ReactionStatsStorer
}

func NewReactionRepo(db ReactionStorer, statsDb ReactionStatsStorer) *ReactionRepo {
	return &ReactionRepo{db: db, statsDb: statsDb}
}

func (repo *ReactionRepo) React(tweetId, userId, emoji string, isTransitive bool) (reactionsCount uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local_store.DBError("empty user id")
	}

	reactionKey := reactionsCountKey(tweetId)
	reactorKey := reactorKey(tweetId, userId)

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	prev, err := txn.Get(reactorKey)
	switch {
	case err == nil:
		return repo.switchReaction(txn, reactorKey, tweetId, storedReaction(prev, userId), emoji)
	case local_store.IsNotFoundError(err):
		return repo.addReaction(txn, reactorKey, reactionKey, tweetId, emoji, isTransitive)
	default:
		return 0, err
	}
}

func (repo *ReactionRepo) addReaction(
	txn local_store.WarpTransactioner,
	reactorKey, reactionKey local_store.DatabaseKey,
	tweetId, emoji string,
	isTransitive bool,
) (reactionsCount uint64, err error) {
	if err = txn.Set(reactorKey, []byte(emoji)); err != nil {
		return 0, err
	}
	reactionsCount, err = txn.Increment(reactionKey)
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
		return reactionsCount, nil
	}
	if err := repo.statsDb.Increment(reactionKey.DatastoreKey()); err != nil {
		log.Warnf("react: stats db increment: %v", err)
	}
	return reactionsCount, nil
}

func (repo *ReactionRepo) switchReaction(
	txn local_store.WarpTransactioner,
	reactorKey local_store.DatabaseKey,
	tweetId, prevEmoji, emoji string,
) (reactionsCount uint64, err error) {
	if prevEmoji == emoji { // nothing to move
		_ = txn.Commit()
		return repo.ReactionsCount(tweetId)
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
	return repo.ReactionsCount(tweetId)
}

func (repo *ReactionRepo) Unreact(tweetId, userId string, isTransitive bool) (reactionsCount uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local_store.DBError("empty user id")
	}

	unreactionKey := reactionsCountKey(tweetId)
	unreactorKey := reactorKey(tweetId, userId)

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	prev, err := txn.Get(unreactorKey)
	if local_store.IsNotFoundError(err) { // already unreacted
		_ = txn.Commit()
		return repo.ReactionsCount(tweetId)
	}
	if err != nil {
		return 0, err
	}
	emoji := storedReaction(prev, userId)
	if err = txn.Delete(unreactorKey); err != nil {
		return 0, err
	}
	reactionsCount, err = txn.Decrement(unreactionKey)
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
		return reactionsCount, nil
	}

	if err := repo.statsDb.Decrement(unreactionKey.DatastoreKey()); err != nil {
		log.Warnf("unreact: stats db decrement: %v", err)
	}

	return reactionsCount, nil
}

const (
	maxReactionKinds = uint64(128)
	defaultReaction  = "❤️"
)

func (repo *ReactionRepo) Reactions(tweetId string) (map[string]uint64, error) {
	if tweetId == "" {
		return nil, local_store.DBError("empty tweet id")
	}

	prefix := local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(EmojiSubNamespace).
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
		count := decodeCount(item.Value)
		if emoji == "" || count == 0 {
			continue
		}
		reactions[emoji] = count
		named += count
	}
	if uint64(len(items)) >= limit { // page truncated, remainder is unknowable
		return reactions, nil
	}

	total, err := repo.ReactionsCount(tweetId)
	if err != nil && !errors.Is(err, ErrReactionsNotFound) {
		return nil, err
	}
	if total > named {
		reactions[defaultReaction] += total - named
	}
	return reactions, nil
}

func (repo *ReactionRepo) Reaction(tweetId, userId string) (string, error) {
	if tweetId == "" {
		return "", local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return "", local_store.DBError("empty user id")
	}

	bt, err := repo.db.Get(reactorKey(tweetId, userId))
	if local_store.IsNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return storedReaction(bt, userId), nil
}

func (repo *ReactionRepo) ReactionsCount(tweetId string) (reactionsNum uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	reactionKey := local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()

	var networkTotal uint64
	if repo.statsDb != nil {
		total, statErr := repo.statsDb.GetAggregatedStat(reactionKey.DatastoreKey())
		if statErr != nil {
			log.Warnf("get reactions stat: %v", statErr)
		} else {
			networkTotal = total
		}
	}

	bt, err := repo.db.Get(reactionKey)
	if local_store.IsNotFoundError(err) {
		if networkTotal == 0 {
			return 0, ErrReactionsNotFound
		}
		return networkTotal, nil
	}
	if err != nil {
		return 0, err
	}
	localTotal := binary.BigEndian.Uint64(bt)
	if networkTotal > localTotal {
		return networkTotal, nil
	}
	return localTotal, nil
}

type reactorIDs = []string

func (repo *ReactionRepo) Reactors(tweetId string, limit *uint64, cursor *string) (_ reactorIDs, cur string, err error) {
	if tweetId == "" {
		return nil, "", local_store.DBError("empty tweet id")
	}

	reactorPrefix := local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(ReactorSubNamespace).
		AddRootID(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(reactorPrefix, limit, cursor)
	if local_store.IsNotFoundError(err) {
		return nil, "", ErrReactionsNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	reactors := make(reactorIDs, 0, len(items))
	for _, item := range items {
		// The value holds the reaction emoji, so the reactor comes from the
		// key — which is also where pre-reaction rows carry it.
		userId := keyID(item.Key)
		if userId == "" {
			continue
		}
		reactors = append(reactors, userId)
	}
	return reactors, cur, nil
}

func (repo *ReactionRepo) SetReacted(userId, tweetId, ownerUserId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	if ownerUserId == "" {
		return local_store.DBError("empty owner user id")
	}

	lt := domain.ReactedTweet{
		UserId:      userId,
		TweetId:     tweetId,
		OwnerUserId: ownerUserId,
		CreatedAt:   time.Now(),
	}

	fixedKey := local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(ReactedSubNamespace).
		AddRootID(userId).
		AddRange(local_store.FixedRangeKey).
		AddParentId(tweetId).
		Build()

	sortableKey := local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(ReactedSubNamespace).
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

func (repo *ReactionRepo) RemoveReacted(userId, tweetId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}

	fixedKey := local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(ReactedSubNamespace).
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

func (repo *ReactionRepo) Reacted(userId string, limit *uint64, cursor *string) ([]domain.ReactedTweet, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("empty user id")
	}

	prefix := local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(ReactedSubNamespace).
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

	reacted := make([]domain.ReactedTweet, 0, len(items))
	for _, item := range items {
		var lt domain.ReactedTweet
		if err := json.Unmarshal(item.Value, &lt); err != nil {
			return nil, "", err
		}
		reacted = append(reacted, lt)
	}
	return reacted, cur, nil
}

func reactionsCountKey(tweetId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(IncrSubNamespace).
		AddRootID(tweetId).
		Build()
}

func reactorKey(tweetId, userId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(ReactorSubNamespace).
		AddRootID(tweetId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(userId).
		Build()
}

func reactionCountKey(tweetId, emoji string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(ReactionRepoName).
		AddSubPrefix(EmojiSubNamespace).
		AddRootID(tweetId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(emoji).
		Build()
}

func keyID(key string) string {
	return key[strings.LastIndex(key, local_store.Delimeter)+1:]
}

func storedReaction(value []byte, userId string) string {
	emoji := string(value)
	if emoji == "" || emoji == userId {
		return defaultReaction
	}
	return emoji
}

func decodeCount(value []byte) uint64 {
	if len(value) < 8 { //nolint:mnd
		return 0
	}
	return binary.BigEndian.Uint64(value)
}
