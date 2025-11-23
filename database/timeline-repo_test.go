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

	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type TimelineRepoTestSuite struct {
	suite.Suite

	db   *local.DB
	repo *TimelineRepo
}

func (s *TimelineRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local.New("", local.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	auth := NewAuthRepo(s.db)
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewTimelineRepo(s.db)
}

func (s *TimelineRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *TimelineRepoTestSuite) TestAddAndGetTimeline() {
	userID := ulid.Make().String()

	tweet := domain.Tweet{
		Id:        ulid.Make().String(),
		UserId:    userID,
		Text:      "Hello timeline",
		CreatedAt: time.Now(),
	}

	err := s.repo.AddTweetToTimeline(userID, tweet)
	s.Require().NoError(err)

	limit := uint64(10)
	timeline, cursor, err := s.repo.GetTimeline(userID, &limit, nil)
	s.Require().NoError(err)
	s.Len(timeline, 1)
	s.Equal(cursor, "end")
	s.Equal(tweet.Text, timeline[0].Text)
	s.Equal(tweet.Id, timeline[0].Id)
}

func (s *TimelineRepoTestSuite) TestDeleteTweetFromTimeline() {
	userID := ulid.Make().String()

	tweet := domain.Tweet{
		Id:        ulid.Make().String(),
		UserId:    userID,
		Text:      "To be deleted",
		CreatedAt: time.Now(),
	}

	err := s.repo.AddTweetToTimeline(userID, tweet)
	s.Require().NoError(err)

	err = s.repo.DeleteTweetFromTimeline(userID, tweet.Id)
	s.Require().NoError(err)

	limit := uint64(10)
	timeline, _, err := s.repo.GetTimeline(userID, &limit, nil)
	s.Require().NoError(err)
	s.Len(timeline, 0)
}

func (s *TimelineRepoTestSuite) TestMultipleTweetsOrder() {
	userID := ulid.Make().String()

	for i := 0; i < 3; i++ {
		t := domain.Tweet{
			Id:        ulid.Make().String(),
			UserId:    userID,
			Text:      "tweet",
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Second),
		}
		s.Require().NoError(s.repo.AddTweetToTimeline(userID, t))
	}

	limit := uint64(10)
	timeline, _, err := s.repo.GetTimeline(userID, &limit, nil)
	s.Require().NoError(err)
	s.Len(timeline, 3)

	// ensure order: newest to oldest
	s.True(timeline[0].CreatedAt.After(timeline[1].CreatedAt) || timeline[0].CreatedAt.Equal(timeline[1].CreatedAt))
	s.True(timeline[1].CreatedAt.After(timeline[2].CreatedAt) || timeline[1].CreatedAt.Equal(timeline[2].CreatedAt))
}

func TestTimelineRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(TimelineRepoTestSuite))
}
