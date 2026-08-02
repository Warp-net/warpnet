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
	"fmt"
	"testing"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type FollowRepoHostileTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *FollowRepo
}

func (s *FollowRepoHostileTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	auth := NewAuthRepo(s.db, "test")
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewFollowRepo(s.db)
}

func (s *FollowRepoHostileTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *FollowRepoHostileTestSuite) counts(userId string) (followers, followings uint64) {
	s.T().Helper()
	var err error
	followers, err = s.repo.GetFollowersCount(userId)
	s.Require().NoError(err)
	followings, err = s.repo.GetFollowingsCount(userId)
	s.Require().NoError(err)
	return followers, followings
}

// ---------------------------------------------------------------------------
// Direction — mixing up follower and following is the classic social bug.
// ---------------------------------------------------------------------------

func (s *FollowRepoHostileTestSuite) TestFollowIsDirectedNotMutual() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	s.Require().NoError(s.repo.Follow(alice, bob))

	s.True(s.repo.IsFollowing(alice, bob), "alice follows bob")
	s.False(s.repo.IsFollowing(bob, alice), "bob must not be dragged into following back")

	s.True(s.repo.IsFollower(bob, alice), "alice is a follower of bob")
	s.False(s.repo.IsFollower(alice, bob), "bob is not a follower of alice")

	aliceFollowers, aliceFollowings := s.counts(alice)
	s.Equal(uint64(0), aliceFollowers)
	s.Equal(uint64(1), aliceFollowings)

	bobFollowers, bobFollowings := s.counts(bob)
	s.Equal(uint64(1), bobFollowers)
	s.Equal(uint64(0), bobFollowings)

	followers, _, err := s.repo.GetFollowers(bob, nil, nil)
	s.Require().NoError(err)
	s.Equal([]string{alice}, followers, "bob's followers list is exactly [alice]")

	followings, _, err := s.repo.GetFollowings(alice, nil, nil)
	s.Require().NoError(err)
	s.Equal([]string{bob}, followings, "alice's followings list is exactly [bob]")
}

func (s *FollowRepoHostileTestSuite) TestMutualFollowKeepsBothSidesConsistent() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	s.Require().NoError(s.repo.Follow(alice, bob))
	s.Require().NoError(s.repo.Follow(bob, alice))

	s.True(s.repo.IsFollowing(alice, bob))
	s.True(s.repo.IsFollowing(bob, alice))

	aliceFollowers, aliceFollowings := s.counts(alice)
	s.Equal(uint64(1), aliceFollowers)
	s.Equal(uint64(1), aliceFollowings)

	bobFollowers, bobFollowings := s.counts(bob)
	s.Equal(uint64(1), bobFollowers)
	s.Equal(uint64(1), bobFollowings)

	// Unfollowing one direction must leave the other intact.
	s.Require().NoError(s.repo.Unfollow(alice, bob))
	s.False(s.repo.IsFollowing(alice, bob))
	s.True(s.repo.IsFollowing(bob, alice), "bob still follows alice")
}

// ---------------------------------------------------------------------------
// Counter integrity under repeated / bogus operations.
// ---------------------------------------------------------------------------

func (s *FollowRepoHostileTestSuite) TestDoubleFollowDoesNotInflateCounters() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	s.Require().NoError(s.repo.Follow(alice, bob))
	s.ErrorIs(s.repo.Follow(alice, bob), ErrAlreadyFollowed)
	s.ErrorIs(s.repo.Follow(alice, bob), ErrAlreadyFollowed)

	bobFollowers, _ := s.counts(bob)
	_, aliceFollowings := s.counts(alice)
	s.Equal(uint64(1), bobFollowers, "spamming follow must not inflate the follower count")
	s.Equal(uint64(1), aliceFollowings)

	followers, _, err := s.repo.GetFollowers(bob, nil, nil)
	s.Require().NoError(err)
	s.Len(followers, 1)
}

// Unfollowing someone you never followed must not push the counters below zero.
func (s *FollowRepoHostileTestSuite) TestUnfollowWithoutFollowNeverUnderflows() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	s.Require().NoError(s.repo.Unfollow(alice, bob))

	bobFollowers, _ := s.counts(bob)
	_, aliceFollowings := s.counts(alice)
	s.Equal(uint64(0), bobFollowers, "counter must clamp at zero, never wrap to 2^64-1")
	s.Equal(uint64(0), aliceFollowings)

	s.False(s.repo.IsFollowing(alice, bob))
}

func (s *FollowRepoHostileTestSuite) TestRepeatedUnfollowStaysAtZero() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	s.Require().NoError(s.repo.Follow(alice, bob))
	for i := 0; i < 4; i++ {
		s.Require().NoError(s.repo.Unfollow(alice, bob), "unfollow %d", i)
		bobFollowers, _ := s.counts(bob)
		_, aliceFollowings := s.counts(alice)
		s.Equal(uint64(0), bobFollowers, "after unfollow %d", i)
		s.Equal(uint64(0), aliceFollowings, "after unfollow %d", i)
	}
}

// Follow → unfollow → follow must settle at exactly one, not two.
func (s *FollowRepoHostileTestSuite) TestRefollowAfterUnfollowSettlesAtOne() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	s.Require().NoError(s.repo.Follow(alice, bob))
	s.Require().NoError(s.repo.Unfollow(alice, bob))
	s.Require().NoError(s.repo.Follow(alice, bob))

	bobFollowers, _ := s.counts(bob)
	_, aliceFollowings := s.counts(alice)
	s.Equal(uint64(1), bobFollowers)
	s.Equal(uint64(1), aliceFollowings)

	followers, _, err := s.repo.GetFollowers(bob, nil, nil)
	s.Require().NoError(err)
	s.Len(followers, 1, "re-following must not leave a stale duplicate row")
}

func (s *FollowRepoHostileTestSuite) TestManyFollowersCountMatchesListing() {
	star := uuid.New().String()

	const n = 12
	fans := make([]string, 0, n)
	for i := 0; i < n; i++ {
		fan := uuid.New().String()
		fans = append(fans, fan)
		s.Require().NoError(s.repo.Follow(fan, star))
	}

	count, _ := s.counts(star)
	s.Equal(uint64(n), count)

	limit := uint64(5)
	seen := make(map[string]struct{})
	var cursor *string
	for page := 0; page < n; page++ {
		batch, next, err := s.repo.GetFollowers(star, &limit, cursor)
		s.Require().NoError(err)
		if len(batch) == 0 {
			break
		}
		for _, f := range batch {
			_, dup := seen[f]
			s.False(dup, "pagination replayed follower %s", f)
			seen[f] = struct{}{}
		}
		if next == "" {
			break
		}
		cursor = &next
	}
	s.Len(seen, n, "paging must enumerate every follower exactly once")
	for _, fan := range fans {
		s.Contains(seen, fan)
	}
}

// ---------------------------------------------------------------------------
// Malformed input must never panic the node.
// ---------------------------------------------------------------------------

func (s *FollowRepoHostileTestSuite) TestSelfFollowIsRejected() {
	alice := uuid.New().String()
	s.Error(s.repo.Follow(alice, alice))

	followers, followings := s.counts(alice)
	s.Equal(uint64(0), followers)
	s.Equal(uint64(0), followings)
}

func (s *FollowRepoHostileTestSuite) TestEmptyIdentifiersAreRejectedNotPanicking() {
	s.Error(s.repo.Follow("", "b"))
	s.Error(s.repo.Follow("a", ""))
	s.Error(s.repo.Unfollow("", "b"))
	s.Error(s.repo.Unfollow("a", ""))
	s.Error(s.repo.Unfollow("", ""))

	_, err := s.repo.GetFollowersCount("")
	s.Error(err)
	_, err = s.repo.GetFollowingsCount("")
	s.Error(err)
}

func (s *FollowRepoHostileTestSuite) TestUnknownUserHasEmptyGraphNotAnError() {
	stranger := uuid.New().String()

	followers, followings := s.counts(stranger)
	s.Equal(uint64(0), followers)
	s.Equal(uint64(0), followings)

	list, _, err := s.repo.GetFollowers(stranger, nil, nil)
	s.Require().NoError(err)
	s.Empty(list)

	list, _, err = s.repo.GetFollowings(stranger, nil, nil)
	s.Require().NoError(err)
	s.Empty(list)

	s.False(s.repo.IsFollowing(stranger, uuid.New().String()))
	s.False(s.repo.IsFollower(stranger, uuid.New().String()))
}

// ---------------------------------------------------------------------------
// Follow requests — the locked-account flow.
// ---------------------------------------------------------------------------

func (s *FollowRepoHostileTestSuite) TestFollowRequestLifecycle() {
	target := uuid.New().String()
	follower := uuid.New().String()

	has, err := s.repo.HasFollowRequest(target, follower)
	s.Require().NoError(err)
	s.False(has, "no request exists until one is made")

	s.Require().NoError(s.repo.AddFollowRequest(target, follower))

	has, err = s.repo.HasFollowRequest(target, follower)
	s.Require().NoError(err)
	s.True(has)

	pending, _, err := s.repo.ListFollowRequests(target, nil, nil)
	s.Require().NoError(err)
	s.Equal([]string{follower}, pending)

	// Approval path: the request is cleared and a real follow edge is created.
	s.Require().NoError(s.repo.RemoveFollowRequest(target, follower))
	s.Require().NoError(s.repo.Follow(follower, target))

	has, err = s.repo.HasFollowRequest(target, follower)
	s.Require().NoError(err)
	s.False(has, "an approved request must not linger in the pending list")
	s.True(s.repo.IsFollowing(follower, target))

	pending, _, err = s.repo.ListFollowRequests(target, nil, nil)
	s.Require().NoError(err)
	s.Empty(pending)
}

func (s *FollowRepoHostileTestSuite) TestRepeatedFollowRequestDoesNotDuplicate() {
	target := uuid.New().String()
	follower := uuid.New().String()

	for i := 0; i < 3; i++ {
		s.Require().NoError(s.repo.AddFollowRequest(target, follower))
	}

	pending, _, err := s.repo.ListFollowRequests(target, nil, nil)
	s.Require().NoError(err)
	s.Len(pending, 1, "spamming the follow button must queue one request, not three")
}

func (s *FollowRepoHostileTestSuite) TestRemoveFollowRequestIsIdempotent() {
	target := uuid.New().String()
	follower := uuid.New().String()

	// Rejecting a request that was never made is a harmless no-op.
	s.Require().NoError(s.repo.RemoveFollowRequest(target, follower))

	s.Require().NoError(s.repo.AddFollowRequest(target, follower))
	s.Require().NoError(s.repo.RemoveFollowRequest(target, follower))
	s.Require().NoError(s.repo.RemoveFollowRequest(target, follower))

	has, err := s.repo.HasFollowRequest(target, follower)
	s.Require().NoError(err)
	s.False(has)
}

// A pending request is not a follow — it must not leak into the graph or counts.
func (s *FollowRepoHostileTestSuite) TestPendingRequestIsNotAFollow() {
	target := uuid.New().String()
	follower := uuid.New().String()

	s.Require().NoError(s.repo.AddFollowRequest(target, follower))

	s.False(s.repo.IsFollowing(follower, target))
	s.False(s.repo.IsFollower(target, follower))

	targetFollowers, _ := s.counts(target)
	s.Equal(uint64(0), targetFollowers, "a pending request must not count as a follower")
}

func (s *FollowRepoHostileTestSuite) TestFollowRequestsAreScopedPerTarget() {
	targetA := uuid.New().String()
	targetB := uuid.New().String()
	follower := uuid.New().String()

	s.Require().NoError(s.repo.AddFollowRequest(targetA, follower))

	hasA, err := s.repo.HasFollowRequest(targetA, follower)
	s.Require().NoError(err)
	s.True(hasA)

	hasB, err := s.repo.HasFollowRequest(targetB, follower)
	s.Require().NoError(err)
	s.False(hasB, "a request to one account must not appear on another")

	pendingB, _, err := s.repo.ListFollowRequests(targetB, nil, nil)
	s.Require().NoError(err)
	s.Empty(pendingB)
}

func (s *FollowRepoHostileTestSuite) TestFollowRequestRejectsEmptyIdentifiers() {
	s.Error(s.repo.AddFollowRequest("", "f"))
	s.Error(s.repo.AddFollowRequest("t", ""))
	s.Error(s.repo.RemoveFollowRequest("", "f"))
	s.Error(s.repo.RemoveFollowRequest("t", ""))

	_, _, err := s.repo.ListFollowRequests("", nil, nil)
	s.Error(err)

	// Reads with missing identifiers answer "no" instead of blowing up.
	has, err := s.repo.HasFollowRequest("", "f")
	s.NoError(err)
	s.False(has)
	has, err = s.repo.HasFollowRequest("t", "")
	s.NoError(err)
	s.False(has)
}

func (s *FollowRepoHostileTestSuite) TestListFollowRequestsPaginates() {
	target := uuid.New().String()

	const n = 7
	for i := 0; i < n; i++ {
		s.Require().NoError(s.repo.AddFollowRequest(target, fmt.Sprintf("follower-%02d", i)))
	}

	limit := uint64(3)
	seen := make(map[string]struct{})
	var cursor *string
	for page := 0; page < n; page++ {
		batch, next, err := s.repo.ListFollowRequests(target, &limit, cursor)
		s.Require().NoError(err)
		if len(batch) == 0 {
			break
		}
		for _, id := range batch {
			_, dup := seen[id]
			s.False(dup, "pagination replayed request %s", id)
			seen[id] = struct{}{}
		}
		if next == "" {
			break
		}
		cursor = &next
	}
	s.Len(seen, n)
}

func TestFollowRepoHostileTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(FollowRepoHostileTestSuite))
}
