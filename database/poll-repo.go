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
	"strconv"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	log "github.com/sirupsen/logrus"
)

const (
	PollRepoName = "/POLLS"

	// Poll-owned key segments. They deliberately duplicate the values likes
	// use rather than borrowing ReactionRepoName's constants: the two keyspaces
	// are independent, and sharing a constant would silently move the poll
	// keys if likes ever renamed theirs.
	VotesSubNamespace     = "VOTES" // per-option vote counters
	VotesIncrSubNamespace = "INCR"
	VotesDecrSubNamespace = "DECR"
	VoterSubNamespace     = "VOTER" // per-voter record of the option they picked
)

var ErrPollAlreadyVoted = local_store.DBError("poll already voted")

type PollStorer interface {
	Get(key local_store.DatabaseKey) ([]byte, error)
	NewTxn() (local_store.WarpTransactioner, error)
}

type PollStatsStorer interface {
	GetAggregatedStat(key ds.Key) (uint64, error)
	Increment(key ds.Key) error
}

type PollRepo struct {
	db      PollStorer
	statsDb PollStatsStorer
}

func NewPollRepo(db PollStorer, statsDb PollStatsStorer) *PollRepo {
	return &PollRepo{db: db, statsDb: statsDb}
}

// pollVotesKey addresses one option's counter. direction is
// VotesIncrSubNamespace or VotesDecrSubNamespace, mirroring the PN-counter
// split the CRDT stats store uses.
func pollVotesKey(tweetId string, option int, direction string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(PollRepoName).
		AddSubPrefix(VotesSubNamespace).
		AddSubPrefix(direction).
		AddRootID(tweetId).
		AddParentId(strconv.Itoa(option)).
		Build()
}

func pollVoterKey(tweetId, userId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(PollRepoName).
		AddSubPrefix(VoterSubNamespace).
		AddRootID(tweetId).
		AddRange(local_store.NoneRangeKey).
		AddParentId(userId).
		Build()
}

// Vote records userId's choice on the poll attached to tweetId. A vote is
// final: a second call for the same voter returns ErrPollAlreadyVoted and
// changes nothing, so a replayed or propagated event can't inflate a count.
//
// isTransitive carries the same meaning as in ReactionRepo: the network-wide
// (CRDT) counter is bumped only on the voter's own node, so a vote stored on
// both the voter's and the author's node is counted once.
func (repo *PollRepo) Vote(tweetId, userId string, option int, isTransitive bool) error {
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if option < 0 {
		return local_store.DBError("negative poll option")
	}

	votesKey := pollVotesKey(tweetId, option, VotesIncrSubNamespace)
	voterKey := pollVoterKey(tweetId, userId)

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	_, err = txn.Get(voterKey)
	if !local_store.IsNotFoundError(err) {
		_ = txn.Commit()
		return ErrPollAlreadyVoted
	}

	if err = txn.Set(voterKey, []byte(strconv.Itoa(option))); err != nil {
		return err
	}
	if _, err = txn.Increment(votesKey); err != nil {
		return err
	}
	if err = txn.Commit(); err != nil {
		return err
	}
	if repo.statsDb == nil || !isTransitive {
		return nil
	}
	if err := repo.statsDb.Increment(votesKey.DatastoreKey()); err != nil {
		log.Warnf("poll: stats db increment: %v", err)
	}
	return nil
}

// Voted returns the option userId picked on tweetId's poll. ok is false when
// they haven't voted.
func (repo *PollRepo) Voted(tweetId, userId string) (option int, ok bool, err error) {
	if tweetId == "" {
		return 0, false, local_store.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, false, local_store.DBError("empty user id")
	}

	bt, err := repo.db.Get(pollVoterKey(tweetId, userId))
	if local_store.IsNotFoundError(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	option, err = strconv.Atoi(string(bt))
	if err != nil {
		return 0, false, err
	}
	return option, true, nil
}

// Results returns the vote count for each of the poll's optionsNum options,
// in option order. Like the other engagement counters it prefers the
// network-wide (CRDT) total and falls back to this node's own counter.
func (repo *PollRepo) Results(tweetId string, optionsNum int) (votes []uint64, err error) {
	if tweetId == "" {
		return nil, local_store.DBError("empty tweet id")
	}
	if optionsNum <= 0 {
		return nil, local_store.DBError("empty poll options")
	}

	votes = make([]uint64, optionsNum)
	for i := range votes {
		votes[i], err = repo.optionVotes(tweetId, i)
		if err != nil {
			return nil, err
		}
	}
	return votes, nil
}

// optionVotes reads one option's tally. It prefers the network-wide (CRDT)
// total and falls back to this node's own counters.
//
// The CRDT store keeps its own positive/negative split for whichever key it
// is handed, so only the INCR key goes to it — same as likes, where an
// unlike decrements that very key. The local fallback is the plain
// difference of the two counters.
func (repo *PollRepo) optionVotes(tweetId string, option int) (uint64, error) {
	incrKey := pollVotesKey(tweetId, option, VotesIncrSubNamespace)

	if repo.statsDb != nil {
		total, err := repo.statsDb.GetAggregatedStat(incrKey.DatastoreKey())
		if err == nil {
			return total, nil
		}
		log.Warnf("crdt poll votes not found for %s option %d - %s", tweetId, option, err)
	}

	incr, err := repo.localCounter(incrKey)
	if err != nil {
		return 0, err
	}
	decr, err := repo.localCounter(pollVotesKey(tweetId, option, VotesDecrSubNamespace))
	if err != nil {
		return 0, err
	}
	if decr >= incr {
		return 0, nil
	}
	return incr - decr, nil
}

// localCounter reads a counter this node keeps. A missing key means nothing
// has been counted there yet, which is zero rather than an error.
func (repo *PollRepo) localCounter(key local_store.DatabaseKey) (uint64, error) {
	bt, err := repo.db.Get(key)
	if local_store.IsNotFoundError(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bt), nil
}
