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

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type PollRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *PollRepo
}

func (s *PollRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewPollRepo(s.db, nil)
}

func (s *PollRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *PollRepoTestSuite) TestVoteAndResults() {
	tweetId := ulid.Make().String()
	first := ulid.Make().String()
	second := ulid.Make().String()

	s.Require().NoError(s.repo.Vote(tweetId, first, 1, true))
	s.Require().NoError(s.repo.Vote(tweetId, second, 1, true))

	votes, err := s.repo.Results(tweetId, 3)
	s.Require().NoError(err)
	s.Equal([]uint64{0, 2, 0}, votes)

	option, ok, err := s.repo.Voted(tweetId, first)
	s.Require().NoError(err)
	s.True(ok)
	s.Equal(1, option)
}

func (s *PollRepoTestSuite) TestResults_SubtractsDecrements() {
	tweetId := ulid.Make().String()
	s.Require().NoError(s.repo.Vote(tweetId, ulid.Make().String(), 0, true))
	s.Require().NoError(s.repo.Vote(tweetId, ulid.Make().String(), 0, true))

	// Nothing retracts a vote today, so bump the DECR counter directly: it
	// proves INCR and DECR address different keys and that the read composes
	// both halves instead of ignoring one.
	txn, err := s.db.NewTxn()
	s.Require().NoError(err)
	_, err = txn.Increment(pollVotesKey(tweetId, 0, VotesDecrSubNamespace))
	s.Require().NoError(err)
	s.Require().NoError(txn.Commit())

	votes, err := s.repo.Results(tweetId, 2)
	s.Require().NoError(err)
	s.Equal([]uint64{1, 0}, votes)
}

func (s *PollRepoTestSuite) TestVote_IsFinal() {
	tweetId := ulid.Make().String()
	userId := ulid.Make().String()

	s.Require().NoError(s.repo.Vote(tweetId, userId, 0, true))

	// A second vote — a replay, or the same event arriving through the
	// author's node — must not move any counter.
	err := s.repo.Vote(tweetId, userId, 1, true)
	s.EqualError(err, ErrPollAlreadyVoted.Error())

	votes, err := s.repo.Results(tweetId, 2)
	s.Require().NoError(err)
	s.Equal([]uint64{1, 0}, votes)

	option, ok, err := s.repo.Voted(tweetId, userId)
	s.Require().NoError(err)
	s.True(ok)
	s.Equal(0, option)
}

func (s *PollRepoTestSuite) TestVoted_NoVote() {
	option, ok, err := s.repo.Voted(ulid.Make().String(), ulid.Make().String())
	s.Require().NoError(err)
	s.False(ok)
	s.Zero(option)
}

func (s *PollRepoTestSuite) TestResults_NoVotes() {
	votes, err := s.repo.Results(ulid.Make().String(), 2)
	s.Require().NoError(err)
	s.Equal([]uint64{0, 0}, votes)
}

func (s *PollRepoTestSuite) TestInvalidParams() {
	tweetId := ulid.Make().String()
	userId := ulid.Make().String()

	s.Error(s.repo.Vote("", userId, 0, true))
	s.Error(s.repo.Vote(tweetId, "", 0, true))
	s.Error(s.repo.Vote(tweetId, userId, -1, true))

	_, _, err := s.repo.Voted("", userId)
	s.Error(err)
	_, _, err = s.repo.Voted(tweetId, "")
	s.Error(err)

	_, err = s.repo.Results("", 2)
	s.Error(err)
	_, err = s.repo.Results(tweetId, 0)
	s.Error(err)
}

func TestPollRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(PollRepoTestSuite))
}
