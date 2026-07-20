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
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/suite"
)

type UserRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *UserRepo
}

func (s *UserRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewUserRepo(s.db)
}

func (s *UserRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *UserRepoTestSuite) TestCreateAndGetUser() {
	user := domain.User{
		Id:        "user1",
		Username:  "testuser",
		CreatedAt: time.Now(),
	}
	created, err := s.repo.Create(user)
	s.Require().NoError(err)
	s.Equal(user.Id, created.Id)

	fetched, err := s.repo.Get(user.Id)
	s.Require().NoError(err)
	s.Equal(user.Id, fetched.Id)
	s.Equal(user.Username, fetched.Username)
}

type stubNewUserNotifier struct {
	added []domain.Notification
}

func (s *stubNewUserNotifier) Add(n domain.Notification) error {
	s.added = append(s.added, n)
	return nil
}

func (s *UserRepoTestSuite) TestNotifiesOnNewUser() {
	notifier := &stubNewUserNotifier{}
	repo := NewUserRepoNotifying(s.db, notifier, "owner-notify")

	_, err := repo.Create(domain.User{Id: "newbie-1", Username: "newbie"})
	s.Require().NoError(err)
	s.Require().Len(notifier.added, 1)
	s.Equal(domain.NotificationNewUserType, notifier.added[0].Type)
	s.Equal("owner-notify", notifier.added[0].UserId)
	s.Equal("newbie joined Warpnet", notifier.added[0].Text)

	// A duplicate must not notify.
	_, err = repo.Create(domain.User{Id: "newbie-1", Username: "newbie"})
	s.Require().ErrorIs(err, ErrUserAlreadyExists)
	s.Len(notifier.added, 1)

	// The owner's own record must not notify.
	_, err = repo.Create(domain.User{Id: "owner-notify", Username: "me"})
	s.Require().NoError(err)
	s.Len(notifier.added, 1)

	// Username-less user falls back to id in the text.
	_, err = repo.Create(domain.User{Id: "newbie-2"})
	s.Require().NoError(err)
	s.Require().Len(notifier.added, 2)
	s.Equal("newbie-2 joined Warpnet", notifier.added[1].Text)
}

func (s *UserRepoTestSuite) TestPlainRepoDoesNotNotify() {
	// A plain repo has a nil notifier: Create must not panic.
	_, err := NewUserRepo(s.db).Create(domain.User{Id: "plain-user", Username: "plain"})
	s.Require().NoError(err)
}

func (s *UserRepoTestSuite) TestUpdateUser() {
	user := domain.User{
		Id:        "user2",
		Username:  "initial",
		CreatedAt: time.Now(),
	}
	_, err := s.repo.Create(user)
	s.Require().NoError(err)

	updated, err := s.repo.Update(user.Id, domain.User{
		Username: "updated",
	})
	s.Require().NoError(err)
	s.Equal("updated", updated.Username)
}

func (s *UserRepoTestSuite) TestDeleteUser() {
	user := domain.User{
		Id:        "user3",
		Username:  "todelete",
		CreatedAt: time.Now(),
	}
	_, err := s.repo.Create(user)
	s.Require().NoError(err)

	err = s.repo.Delete(user.Id)
	s.Require().NoError(err)

	_, err = s.repo.Get(user.Id)
	s.Equal(ErrUserNotFound, err)
}

func (s *UserRepoTestSuite) TestGetByNodeID() {
	user := domain.User{
		Id:        "user4",
		NodeId:    "node123",
		Username:  "nodeuser",
		CreatedAt: time.Now(),
	}
	_, err := s.repo.Create(user)
	s.Require().NoError(err)

	found, err := s.repo.GetByNodeID("node123")
	s.Require().NoError(err)
	s.Equal("user4", found.Id)
}

func (s *UserRepoTestSuite) TestListAndGetBatch() {
	users := []domain.User{
		{Id: "list1", Username: "a"},
		{Id: "list2", Username: "b"},
		{Id: "list3", Username: "c"},
	}
	for _, u := range users {
		u.CreatedAt = time.Now()
		_, err := s.repo.Create(u)
		s.Require().NoError(err)
	}

	limit := uint64(10)
	all, _, err := s.repo.List(&limit, nil)
	s.Require().NoError(err)
	s.GreaterOrEqual(len(all), 3)

	usersBatch, err := s.repo.GetBatch("list1", "list2")
	s.Require().NoError(err)
	s.Len(usersBatch, 2)
}

func (s *UserRepoTestSuite) TestSearch() {
	u1 := domain.User{Id: "u-alice", Username: "alice", Bio: "loves cats"}
	u2 := domain.User{Id: "u-bob", Username: "bob", Bio: "rust enthusiast"}
	u3 := domain.User{Id: "u-carol", Username: "carol", Bio: "alice's friend"}

	_, err := s.repo.Create(u1)
	s.Require().NoError(err)
	_, err = s.repo.Create(u2)
	s.Require().NoError(err)
	_, err = s.repo.Create(u3)
	s.Require().NoError(err)

	limit := uint64(50)
	hits, _, err := s.repo.Search("alice", &limit, nil)
	s.Require().NoError(err)
	// Matches by username (u1) and by bio (u3).
	ids := map[string]bool{}
	for _, u := range hits {
		ids[u.Id] = true
	}
	s.True(ids["u-alice"], "should match by username")
	s.True(ids["u-carol"], "should match by bio substring")
	s.False(ids["u-bob"], "should not match unrelated user")
}

func (s *UserRepoTestSuite) TestSearch_CaseInsensitive() {
	_, err := s.repo.Create(domain.User{Id: "u-mixed", Username: "MixedCase"})
	s.Require().NoError(err)

	hits, _, err := s.repo.Search("MIXED", nil, nil)
	s.Require().NoError(err)
	found := false
	for _, u := range hits {
		if u.Id == "u-mixed" {
			found = true
			break
		}
	}
	s.True(found, "search should be case-insensitive")
}

func (s *UserRepoTestSuite) TestSearch_EmptyQuery() {
	_, _, err := s.repo.Search("", nil, nil)
	s.Error(err)
}

// TestSearch_FindsUserBeyondFirstPage guards the regression where Search
// only scanned a single `limit`-sized page: a discovered user that sorts
// after that page (many non-matching users ahead of it) became invisible to
// search and the new-message picker. Search must scan the whole prefix.
func (s *UserRepoTestSuite) TestSearch_FindsUserBeyondFirstPage() {
	// Enough non-matching users to push the target well past a 20-wide page.
	for i := 0; i < 40; i++ {
		_, err := s.repo.Create(domain.User{
			Id:       fmt.Sprintf("beyond-noise-%03d", i),
			Username: fmt.Sprintf("beyondnoise%03d", i),
		})
		s.Require().NoError(err)
	}
	// Id sorts last within the shared RTT range, so it lands after the noise.
	_, err := s.repo.Create(domain.User{Id: "zzzz-beyond-target", Username: "beyondtargetuser"})
	s.Require().NoError(err)

	limit := uint64(20)
	hits, _, err := s.repo.Search("beyondtargetuser", &limit, nil)
	s.Require().NoError(err)

	found := false
	for _, u := range hits {
		if u.Id == "zzzz-beyond-target" {
			found = true
			break
		}
	}
	s.True(found, "Search must scan past the first page to find a discovered user")
}

// TestWhoToFollow_SurfacesBuriedNativePeer guards two regressions at once:
//   - a warpnet-native (ULID) peer that sorts behind a wall of other accounts
//     (as Mastodon accounts do in production) must still be recommended, not
//     dropped because it fell outside the first page;
//   - a peer with an avatar but no tweets yet must not be filtered out.
func (s *UserRepoTestSuite) TestWhoToFollow_SurfacesBuriedNativePeer() {
	// 30 non-native users (no avatar, no tweets) whose ids sort BEFORE the
	// native ULID ("0000-*" < "01ARZ..."), pushing the native past a single
	// 20-wide page. None are gated out for missing avatar/tweets.
	for i := 0; i < 30; i++ {
		_, err := s.repo.Create(domain.User{
			Id:       fmt.Sprintf("0000-wtf-noise-%03d", i),
			Username: fmt.Sprintf("wtfnoise%03d", i),
		})
		s.Require().NoError(err)
	}
	// Native peer with an avatar but zero tweets.
	const nativeID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	_, err := s.repo.Create(domain.User{
		Id:          nativeID,
		Username:    "freshpeer",
		AvatarKey:   "image/jpeg,BBBB",
		TweetsCount: 0,
	})
	s.Require().NoError(err)

	// Native peer with neither avatar nor tweets — a just-joined peer must
	// still be recommendable (warpnet peers aren't gated on avatar).
	const barePeerID = "01ARZ3NDEKTSV4RRFFQ69G5FB0"
	_, err = s.repo.Create(domain.User{Id: barePeerID, Username: "barepeer"})
	s.Require().NoError(err)

	limit := uint64(20)
	recs, _, err := s.repo.WhoToFollow(&limit, nil)
	s.Require().NoError(err)

	got := map[string]bool{}
	nonNativeNoAvatar := false
	for _, u := range recs {
		got[u.Id] = true
		if strings.HasPrefix(u.Id, "0000-wtf-noise-") {
			nonNativeNoAvatar = true
		}
	}
	s.True(got[nativeID], "WhoToFollow must surface a native peer past the first page with no tweets")
	s.True(got[barePeerID], "WhoToFollow must surface a native peer with no avatar and no tweets")
	s.True(nonNativeNoAvatar, "WhoToFollow must not gate non-native users on missing avatar/tweets")
}

func TestUserRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(UserRepoTestSuite))
}

func (s *UserRepoTestSuite) TestList_NoFixedKeyLeak() {
	fresh := []domain.User{
		{Id: "leakcheck1", Username: "x", CreatedAt: time.Now()},
		{Id: "leakcheck2", Username: "y", CreatedAt: time.Now()},
		{Id: "leakcheck3", Username: "z", CreatedAt: time.Now()},
	}
	for _, u := range fresh {
		_, err := s.repo.Create(u)
		s.Require().NoError(err)
	}

	limit := uint64(100)
	all, _, err := s.repo.List(&limit, nil)
	s.Require().NoError(err)

	seen := map[string]int{}
	for _, u := range all {
		seen[u.Id]++
	}
	// Each user appears exactly once: the fixed lookup keys written by
	// Create are skipped by List, so no duplicates and no decode leaks.
	s.Equal(1, seen["leakcheck1"])
	s.Equal(1, seen["leakcheck2"])
	s.Equal(1, seen["leakcheck3"])
}
