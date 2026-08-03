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

//nolint:all
package database

import (
	"testing"
	"time"

	"go.uber.org/goleak"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type ReactionRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *ReactionRepo
}

func (s *ReactionRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewReactionRepo(s.db, nil)
}

func (s *ReactionRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *ReactionRepoTestSuite) TestReactAndUnreact() {
	userId := ulid.Make().String()
	tweetId := ulid.Make().String()

	// React with the default
	likes, err := s.repo.React(tweetId, userId, "", true)
	s.Require().NoError(err)
	s.Equal(uint64(1), likes)

	// React again (must not increment)
	likes, err = s.repo.React(tweetId, userId, "", true)
	s.Require().NoError(err)
	s.Equal(uint64(1), likes)

	// Check count directly
	count, err := s.repo.ReactionsCount(tweetId)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	// Check reactors
	limit := uint64(10)
	reactors, cur, err := s.repo.Reactors(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Len(reactors, 1)
	s.Equal(cur, "end")
	s.Equal(userId, reactors[0])

	// Take the reaction back
	likes, err = s.repo.Unreact(tweetId, userId, true)
	s.Require().NoError(err)
	s.Equal(uint64(0), likes)

	// Take it back again (must not fail)
	likes, err = s.repo.Unreact(tweetId, userId, true)
	s.Require().NoError(err)
	s.Equal(uint64(0), likes)

	// Check reactors now
	reactors, _, err = s.repo.Reactors(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Len(reactors, 0)
}

func (s *ReactionRepoTestSuite) TestReact_InvalidParams() {
	tweetId := ulid.Make().String()
	userId := ulid.Make().String()

	_, err := s.repo.React("", userId, "", true)
	s.Error(err)

	_, err = s.repo.React(tweetId, "", "", true)
	s.Error(err)

	_, err = s.repo.Unreact("", userId, true)
	s.Error(err)

	_, err = s.repo.Unreact(tweetId, "", true)
	s.Error(err)

	_, err = s.repo.ReactionsCount("")
	s.Error(err)

	_, _, err = s.repo.Reactors("", nil, nil)
	s.Error(err)
}

func (s *ReactionRepoTestSuite) TestReactionsCount_NotFound() {
	id := ulid.Make().String()
	_, err := s.repo.ReactionsCount(id)
	s.EqualError(err, ErrReactionsNotFound.Error())
}

func (s *ReactionRepoTestSuite) TestReactors_Empty() {
	tweetId := ulid.Make().String()
	limit := uint64(10)
	reactors, cur, err := s.repo.Reactors(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(reactors)
	s.Equal(cur, "end")
}

func TestReactionRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(ReactionRepoTestSuite))
}

func (s *ReactionRepoTestSuite) TestReactedIndex() {
	userId := ulid.Make().String()
	ownerId := ulid.Make().String()
	tweetId := ulid.Make().String()

	// Empty before anything is reacted to.
	limit := uint64(10)
	reacted, cur, err := s.repo.Reacted(userId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(reacted)
	s.Equal("end", cur)

	// Index a reacted tweet.
	err = s.repo.SetReacted(userId, tweetId, ownerId)
	s.Require().NoError(err)

	// Indexing again is a no-op, not a duplicate.
	err = s.repo.SetReacted(userId, tweetId, ownerId)
	s.Require().NoError(err)

	reacted, cur, err = s.repo.Reacted(userId, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(reacted, 1)
	s.Equal("end", cur)
	s.Equal(userId, reacted[0].UserId)
	s.Equal(tweetId, reacted[0].TweetId)
	s.Equal(ownerId, reacted[0].OwnerUserId)

	// A later reaction must come back first (newest-first ordering).
	laterTweetId := ulid.Make().String()
	time.Sleep(2 * time.Millisecond)
	err = s.repo.SetReacted(userId, laterTweetId, ownerId)
	s.Require().NoError(err)

	reacted, _, err = s.repo.Reacted(userId, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(reacted, 2)
	s.Equal(laterTweetId, reacted[0].TweetId)
	s.Equal(tweetId, reacted[1].TweetId)

	err = s.repo.RemoveReacted(userId, laterTweetId)
	s.Require().NoError(err)

	// Remove and verify the index is empty again.
	err = s.repo.RemoveReacted(userId, tweetId)
	s.Require().NoError(err)

	// Removing again should not fail.
	err = s.repo.RemoveReacted(userId, tweetId)
	s.Require().NoError(err)

	reacted, _, err = s.repo.Reacted(userId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(reacted)
}

func (s *ReactionRepoTestSuite) TestReactedIndex_InvalidParams() {
	id := ulid.Make().String()

	s.Error(s.repo.SetReacted("", id, id))
	s.Error(s.repo.SetReacted(id, "", id))
	s.Error(s.repo.SetReacted(id, id, ""))
	s.Error(s.repo.RemoveReacted("", id))
	s.Error(s.repo.RemoveReacted(id, ""))
	_, _, err := s.repo.Reacted("", nil, nil)
	s.Error(err)
}

func (s *ReactionRepoTestSuite) TestReactors_Multiple() {
	tweetId := ulid.Make().String()
	user1 := ulid.Make().String()
	user2 := ulid.Make().String()

	_, err := s.repo.React(tweetId, user1, "", true)
	s.Require().NoError(err)
	_, err = s.repo.React(tweetId, user2, "", true)
	s.Require().NoError(err)

	limit := uint64(10)
	reactors, _, err := s.repo.Reactors(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(reactors, 2)
	s.ElementsMatch([]string{user1, user2}, reactors)
}

func (s *ReactionRepoTestSuite) TestReactions_SwitchKeepsTotal() {
	tweetId := ulid.Make().String()
	user1 := ulid.Make().String()
	user2 := ulid.Make().String()

	_, err := s.repo.React(tweetId, user1, "🔥", true)
	s.Require().NoError(err)
	total, err := s.repo.React(tweetId, user2, "", true) // no emoji named -> heart
	s.Require().NoError(err)
	s.Equal(uint64(2), total)

	reactions, err := s.repo.Reactions(tweetId)
	s.Require().NoError(err)
	s.Equal(map[string]uint64{"🔥": 1, "❤️": 1}, reactions)

	// Switching moves the per-emoji tallies but not the reaction itself.
	total, err = s.repo.React(tweetId, user1, "👍", true)
	s.Require().NoError(err)
	s.Equal(uint64(2), total)

	reactions, err = s.repo.Reactions(tweetId)
	s.Require().NoError(err)
	s.Equal(map[string]uint64{"👍": 1, "❤️": 1}, reactions)

	emoji, err := s.repo.Reaction(tweetId, user1)
	s.Require().NoError(err)
	s.Equal("👍", emoji)

	// Unliking drops both the total and the emoji it was counted under.
	total, err = s.repo.Unreact(tweetId, user1, true)
	s.Require().NoError(err)
	s.Equal(uint64(1), total)

	reactions, err = s.repo.Reactions(tweetId)
	s.Require().NoError(err)
	s.Equal(map[string]uint64{"❤️": 1}, reactions)

	emoji, err = s.repo.Reaction(tweetId, user1)
	s.Require().NoError(err)
	s.Empty(emoji)
}

func (s *ReactionRepoTestSuite) TestReactions_LegacyReactionIsAHeart() {
	tweetId := ulid.Make().String()
	legacyUser := ulid.Make().String()
	newUser := ulid.Make().String()

	// A like written before reactions existed: the reactor's own id as the
	// value and no per-emoji counter at all.
	txn, err := s.db.NewTxn()
	s.Require().NoError(err)
	s.Require().NoError(txn.Set(reactorKey(tweetId, legacyUser), []byte(legacyUser)))
	_, err = txn.Increment(reactionsCountKey(tweetId))
	s.Require().NoError(err)
	s.Require().NoError(txn.Commit())

	emoji, err := s.repo.Reaction(tweetId, legacyUser)
	s.Require().NoError(err)
	s.Equal("❤️", emoji)

	_, err = s.repo.React(tweetId, newUser, "🔥", true)
	s.Require().NoError(err)

	// The total counter knows about the legacy like, the per-emoji keys
	// don't — the remainder is attributed to hearts.
	reactions, err := s.repo.Reactions(tweetId)
	s.Require().NoError(err)
	s.Equal(map[string]uint64{"🔥": 1, "❤️": 1}, reactions)

	limit := uint64(10)
	reactors, _, err := s.repo.Reactors(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.ElementsMatch([]string{legacyUser, newUser}, reactors)
}

func (s *ReactionRepoTestSuite) TestReact_RejectsMalformedEmoji() {
	tweetId := ulid.Make().String()
	userId := ulid.Make().String()

	_, err := s.repo.React(tweetId, userId, "🔥/💧", true) // key delimiter
	s.Error(err)

	_, err = s.repo.React(tweetId, userId, "not an emoji at all", true)
	s.Error(err)

	reactors, _, err := s.repo.Reactors(tweetId, nil, nil)
	s.Require().NoError(err)
	s.Empty(reactors) // nothing was stored
}

func (s *ReactionRepoTestSuite) TestReactions_InvalidParams() {
	_, err := s.repo.Reactions("")
	s.Error(err)

	_, err = s.repo.Reaction("", ulid.Make().String())
	s.Error(err)

	_, err = s.repo.Reaction(ulid.Make().String(), "")
	s.Error(err)
}

// crdtSkew is a stats store whose per-key values can be nudged out of step
// with each other, the way independently merging CRDT deltas are in a real
// cluster.
type crdtSkew struct {
	vals map[string]uint64
}

func (c *crdtSkew) GetAggregatedStat(key ds.Key) (uint64, error) { return c.vals[key.String()], nil }

func (c *crdtSkew) Increment(key ds.Key) error {
	c.vals[key.String()]++
	return nil
}

func (c *crdtSkew) Decrement(key ds.Key) error {
	if c.vals[key.String()] > 0 {
		c.vals[key.String()]--
	}
	return nil
}

// A switch, then a stale per-emoji CRDT view: the breakdown must follow the
// reaction that is actually stored, never the emoji the CRDT still carries.
func (s *ReactionRepoTestSuite) TestReactions_SurviveSkewedCRDT() {
	skew := &crdtSkew{vals: map[string]uint64{}}
	repo := NewReactionRepo(s.db, skew)

	tweetId := ulid.Make().String()
	userId := ulid.Make().String()

	_, err := repo.React(tweetId, userId, "❤️", true)
	s.Require().NoError(err)
	_, err = repo.React(tweetId, userId, "🔥", true) // switch
	s.Require().NoError(err)

	// The heart's delta hasn't been retracted and the fire's hasn't landed.
	skew.vals[reactionCountKey(tweetId, "❤️").DatastoreKey().String()] = 1
	skew.vals[reactionCountKey(tweetId, "🔥").DatastoreKey().String()] = 0

	reactions, err := repo.Reactions(tweetId)
	s.Require().NoError(err)
	s.Equal(map[string]uint64{"🔥": 1}, reactions)

	emoji, err := repo.Reaction(tweetId, userId)
	s.Require().NoError(err)
	s.Equal("🔥", emoji, "the chip and the reactor's own emoji must agree")

	total, err := repo.ReactionsCount(tweetId)
	s.Require().NoError(err)
	s.Equal(total, sumCounts(reactions), "the breakdown must add up to the total")
}

// Taking a reaction back must clear the chip even while the per-emoji CRDT
// delta is still in flight — the counter and the breakdown reached zero
// together on the screenshot that started this.
func (s *ReactionRepoTestSuite) TestReactions_UnreactClearsSkewedChip() {
	skew := &crdtSkew{vals: map[string]uint64{}}
	repo := NewReactionRepo(s.db, skew)

	tweetId := ulid.Make().String()
	userId := ulid.Make().String()

	_, err := repo.React(tweetId, userId, "❤️", true)
	s.Require().NoError(err)
	total, err := repo.Unreact(tweetId, userId, true)
	s.Require().NoError(err)
	s.Equal(uint64(0), total)

	skew.vals[reactionCountKey(tweetId, "❤️").DatastoreKey().String()] = 1 // stale

	reactions, err := repo.Reactions(tweetId)
	s.Require().NoError(err)
	s.Empty(reactions)
}

func sumCounts(m map[string]uint64) uint64 {
	var sum uint64
	for _, v := range m {
		sum += v
	}
	return sum
}
