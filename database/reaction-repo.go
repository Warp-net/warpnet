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
	ReactionRepoName    = "/REACTIONS"
	IncrSubNamespace    = "INCR"
	ReactorSubNamespace = "REACTOR"
	ReactedSubNamespace = "REACTED" // per-user index of reacted tweet refs
	EmojiSubNamespace   = "EMOJI"   // per-emoji counters, one key per reaction
)

// maxReactionKinds bounds one page of the per-emoji tally. Beyond it the
// breakdown is reported as-is instead of folding the remainder into
// DefaultReaction, which would inflate hearts.
const maxReactionKinds = uint64(128)

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
	emoji, err = domain.NormalizeReaction(emoji)
	if err != nil {
		return 0, err
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

// addReaction records a user's first reaction on a tweet: the total counter
// and the emoji's own counter both go up by one. Commits txn.
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

// switchReaction moves an existing reaction to a different emoji. The
// reaction itself stays, so only this node's per-emoji tallies move: the
// total is untouched and there is nothing to mirror into the CRDT.
// Commits txn.
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

// Reactions returns the per-emoji tally for a tweet. Likes stored before
// reactions existed carry no per-emoji counter, so whatever the total
// counter holds beyond the sum of the named emoji is attributed to
// DefaultReaction — old hearts stay hearts.
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
	// Local counters only, deliberately. The author's node is where every
	// reaction is propagated, so its own counters are the whole picture, and
	// the per-emoji CRDT keys merge independently of each other and of the
	// total — reading them here let the breakdown contradict both the total
	// and the reactor's own emoji while deltas were still in flight.
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
		reactions[domain.DefaultReaction] += total - named
	}
	return reactions, nil
}

// Reaction reports the emoji this user put on the tweet, or an empty
// string when they haven't reacted to it.
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

	if repo.statsDb != nil {
		total, err := repo.statsDb.GetAggregatedStat(reactionKey.DatastoreKey())
		if err == nil {
			return total, nil
		}
		log.Warnf("get reactions stat: %v", err)
	}

	bt, err := repo.db.Get(reactionKey)
	if local_store.IsNotFoundError(err) {
		return 0, ErrReactionsNotFound
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bt), nil
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

	// Same fixed/sortable key pair as the chat message repo: the fixed key
	// gives deterministic lookup for unlike and is skipped by iteration,
	// the sortable key orders the list newest-reacted-first.
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

// keyID returns the last segment of a database key — the reactor's user id
// for a LIKER key, the emoji for a REACT key.
func keyID(key string) string {
	return key[strings.LastIndex(key, local_store.Delimeter)+1:]
}

// storedReaction decodes the emoji a reactor row carries. Rows written before
// reactions existed stored the reactor's own id as the value, so they read
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
// different emoji moves the per-emoji tallies but leaves the reaction itself
// (and therefore the total counter) alone.

// decodeCount reads a counter value written by the store's Increment,
// tolerating a short or missing value.
func decodeCount(value []byte) uint64 {
	if len(value) < 8 { //nolint:mnd
		return 0
	}
	return binary.BigEndian.Uint64(value)
}
