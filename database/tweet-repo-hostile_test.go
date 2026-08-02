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
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type TweetRepoHostileTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *TweetRepo
}

func (s *TweetRepoHostileTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	auth := NewAuthRepo(s.db, "test")
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewTweetRepo(s.db, nil)
}

func (s *TweetRepoHostileTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *TweetRepoHostileTestSuite) newTweet(userId, text string) domain.Tweet {
	s.T().Helper()
	tweet, err := s.repo.Create(userId, domain.Tweet{UserId: userId, Text: text})
	s.Require().NoError(err)
	return tweet
}

// ---------------------------------------------------------------------------
// Creation defaults — a tweet arriving over the wire is only half-filled.
// ---------------------------------------------------------------------------

func (s *TweetRepoHostileTestSuite) TestCreateFillsMissingFields() {
	userId := ulid.Make().String()

	tweet, err := s.repo.Create(userId, domain.Tweet{UserId: userId, Text: "bare minimum"})
	s.Require().NoError(err)

	s.NotEmpty(tweet.Id, "a tweet without an ID must get one")
	s.Equal(tweet.Id, tweet.RootId, "a top-level tweet roots itself")
	s.False(tweet.CreatedAt.IsZero(), "a tweet without a timestamp must get one")
	s.Equal("warpnet", tweet.Network, "network must default rather than stay blank")
}

// A completely empty payload is not a post — it must be rejected instead of
// creating a ghost row that shows up in someone's timeline.
func (s *TweetRepoHostileTestSuite) TestCreateRejectsEmptyTweet() {
	_, err := s.repo.Create(ulid.Make().String(), domain.Tweet{})
	s.Error(err)
}

// A caller-supplied ID must be honoured — gossip replays the same tweet from
// several peers and they all have to converge on one row, not N copies.
func (s *TweetRepoHostileTestSuite) TestCreateHonoursSuppliedIDAndIsIdempotentOnRead() {
	userId := ulid.Make().String()
	id := ulid.Make().String()

	created, err := s.repo.Create(userId, domain.Tweet{Id: id, UserId: userId, Text: "gossiped"})
	s.Require().NoError(err)
	s.Equal(id, created.Id)

	// Re-delivering the same tweet must not fan out into a second row.
	_, err = s.repo.Create(userId, domain.Tweet{Id: id, UserId: userId, Text: "gossiped"})
	s.Require().NoError(err)

	got, err := s.repo.Get(userId, id)
	s.Require().NoError(err)
	s.Equal(id, got.Id)
	s.Equal("gossiped", got.Text)
}

func (s *TweetRepoHostileTestSuite) TestCreatePreservesHostileText() {
	userId := ulid.Make().String()

	hostile := []string{
		strings.Repeat("я", 5000),
		"<script>alert(1)</script>",
		"line\nbreak\ttab\x00null",
		"🔥🙈👩‍👩‍👧‍👦 zero​width",
		"'; DROP TABLE tweets; --",
		"\\\"escaped\\\" json",
	}

	for _, text := range hostile {
		tweet, err := s.repo.Create(userId, domain.Tweet{UserId: userId, Text: text})
		s.Require().NoError(err)

		got, err := s.repo.Get(userId, tweet.Id)
		s.Require().NoError(err)
		s.Equal(text, got.Text, "text must round trip byte for byte")
	}
}

// ---------------------------------------------------------------------------
// Moderation blocklist.
// ---------------------------------------------------------------------------

func (s *TweetRepoHostileTestSuite) TestBlocklistIsPerTweetAndDefaultsOpen() {
	userId := ulid.Make().String()
	bad := s.newTweet(userId, "spam")
	good := s.newTweet(userId, "fine")

	s.False(s.repo.IsBlocklisted(bad.Id), "tweets start unmoderated")

	s.Require().NoError(s.repo.Blocklist(bad.Id))
	s.True(s.repo.IsBlocklisted(bad.Id))
	s.False(s.repo.IsBlocklisted(good.Id), "moderating one tweet must not censor another")

	// Blocklisting twice is idempotent.
	s.Require().NoError(s.repo.Blocklist(bad.Id))
	s.True(s.repo.IsBlocklisted(bad.Id))
}

func (s *TweetRepoHostileTestSuite) TestBlocklistIgnoresEmptyIDAndUnknownIsFree() {
	s.NoError(s.repo.Blocklist(""), "an empty ID must be a no-op, not a global block")
	s.False(s.repo.IsBlocklisted(""))
	s.False(s.repo.IsBlocklisted(ulid.Make().String()), "unknown tweets are not moderated")
}

// ---------------------------------------------------------------------------
// Update — the edit path.
// ---------------------------------------------------------------------------

func (s *TweetRepoHostileTestSuite) TestUpdateRejectsMissingIdentifiers() {
	s.Error(s.repo.Update(domain.Tweet{UserId: "u"}))
	s.Error(s.repo.Update(domain.Tweet{Id: "t"}))
	s.Error(s.repo.Update(domain.Tweet{}))
}

func (s *TweetRepoHostileTestSuite) TestUpdateNonexistentTweetFails() {
	err := s.repo.Update(domain.Tweet{
		Id:     ulid.Make().String(),
		UserId: ulid.Make().String(),
		Text:   "ghost edit",
	})
	s.Error(err, "editing a tweet that was never stored must not create it")
}

func (s *TweetRepoHostileTestSuite) TestUpdateRewritesTextAndStampsUpdatedAt() {
	userId := ulid.Make().String()
	tweet := s.newTweet(userId, "original")
	s.Nil(tweet.UpdatedAt)

	s.Require().NoError(s.repo.Update(domain.Tweet{
		Id: tweet.Id, UserId: userId, Text: "edited",
	}))

	got, err := s.repo.Get(userId, tweet.Id)
	s.Require().NoError(err)
	s.Equal("edited", got.Text)
	s.Require().NotNil(got.UpdatedAt, "an edit must be visibly marked as edited")
	s.WithinDuration(time.Now(), *got.UpdatedAt, time.Minute)
	s.Equal(tweet.CreatedAt.Unix(), got.CreatedAt.Unix(), "editing must not rewrite history")
}

// An update carrying no text must not blank the post — clients send partial
// payloads (e.g. moderation verdicts) and that must never erase content.
func (s *TweetRepoHostileTestSuite) TestUpdateWithEmptyTextKeepsOriginal() {
	userId := ulid.Make().String()
	tweet := s.newTweet(userId, "keep me")

	s.Require().NoError(s.repo.Update(domain.Tweet{Id: tweet.Id, UserId: userId, Text: ""}))

	got, err := s.repo.Get(userId, tweet.Id)
	s.Require().NoError(err)
	s.Equal("keep me", got.Text)
}

func (s *TweetRepoHostileTestSuite) TestUpdateAttachesModerationVerdictWithoutTouchingText() {
	userId := ulid.Make().String()
	tweet := s.newTweet(userId, "questionable content")

	reason := "hate speech"
	s.Require().NoError(s.repo.Update(domain.Tweet{
		Id:     tweet.Id,
		UserId: userId,
		Moderation: &domain.TweetModeration{
			ModeratorID: domain.ID("moderator-1"),
			Reason:      &reason,
			TimeAt:      time.Now(),
		},
	}))

	got, err := s.repo.Get(userId, tweet.Id)
	s.Require().NoError(err)
	s.Require().NotNil(got.Moderation)
	s.Equal(domain.ID("moderator-1"), got.Moderation.ModeratorID)
	s.Require().NotNil(got.Moderation.Reason)
	s.Equal(reason, *got.Moderation.Reason)
	s.Equal("questionable content", got.Text, "a verdict must not silently rewrite the post")
	s.True(got.IsModerated())
}

// One user must not be able to edit another user's tweet by guessing its ID.
func (s *TweetRepoHostileTestSuite) TestUpdateCannotCrossUserBoundary() {
	author := ulid.Make().String()
	attacker := ulid.Make().String()
	tweet := s.newTweet(author, "authored by victim")

	err := s.repo.Update(domain.Tweet{Id: tweet.Id, UserId: attacker, Text: "hijacked"})
	s.Error(err, "an attacker's user ID must not resolve the victim's tweet")

	got, err := s.repo.Get(author, tweet.Id)
	s.Require().NoError(err)
	s.Equal("authored by victim", got.Text)
}

// ---------------------------------------------------------------------------
// Edit history.
// ---------------------------------------------------------------------------

func (s *TweetRepoHostileTestSuite) TestAppendEditValidatesEveryRequiredField() {
	_, err := s.repo.AppendEdit(domain.TweetEdit{UserId: "u", Text: "t"})
	s.Error(err, "an edit must name the tweet it revises")

	_, err = s.repo.AppendEdit(domain.TweetEdit{OriginalTweetId: "t", Text: "t"})
	s.Error(err, "an edit must name its author")

	_, err = s.repo.AppendEdit(domain.TweetEdit{OriginalTweetId: "t", UserId: "u"})
	s.Error(err, "an empty revision is not an edit")
}

func (s *TweetRepoHostileTestSuite) TestAppendEditFillsIdentityAndTimestamp() {
	edit, err := s.repo.AppendEdit(domain.TweetEdit{
		OriginalTweetId: ulid.Make().String(),
		UserId:          ulid.Make().String(),
		Text:            "revision one",
	})
	s.Require().NoError(err)
	s.NotEmpty(edit.Id)
	s.False(edit.EditedAt.IsZero())
}

func (s *TweetRepoHostileTestSuite) TestAppendEditKeepsEveryRevision() {
	original := ulid.Make().String()
	userId := ulid.Make().String()

	ids := make(map[string]struct{})
	for i := 0; i < 5; i++ {
		edit, err := s.repo.AppendEdit(domain.TweetEdit{
			OriginalTweetId: original,
			UserId:          userId,
			Text:            "revision",
			EditedAt:        time.Now().Add(time.Duration(i) * time.Second),
		})
		s.Require().NoError(err)
		_, seen := ids[edit.Id]
		s.False(seen, "each revision must get a distinct ID")
		ids[edit.Id] = struct{}{}
	}
	s.Len(ids, 5)
}

// ---------------------------------------------------------------------------
// Retweets — counters are the classic place a social network goes wrong.
// ---------------------------------------------------------------------------

func (s *TweetRepoHostileTestSuite) TestUnRetweetRejectsEmptyIdentifiers() {
	s.Error(s.repo.UnRetweet("", "tweet", false))
	s.Error(s.repo.UnRetweet("user", "", false))
}

// Undoing a retweet must decrement exactly once and never underflow the counter
// into eighteen quintillion retweets.
func (s *TweetRepoHostileTestSuite) TestUnRetweetNeverUnderflowsCounter() {
	author := ulid.Make().String()
	tweet := s.newTweet(author, "boost me")

	boosters := []string{ulid.Make().String(), ulid.Make().String()}
	for _, b := range boosters {
		booster := b
		_, err := s.repo.NewRetweet(domain.Tweet{
			Id: tweet.Id, UserId: author, Text: tweet.Text, RetweetedBy: &booster,
		}, false)
		s.Require().NoError(err)
	}

	count, err := s.repo.RetweetsCount(tweet.Id)
	s.Require().NoError(err)
	s.Equal(uint64(2), count)

	for i, b := range boosters {
		s.Require().NoError(s.repo.UnRetweet(b, tweet.Id, false))
		count, err = s.repo.RetweetsCount(tweet.Id)
		s.Require().NoError(err)
		s.Equal(uint64(len(boosters)-i-1), count)
	}

	// A user who never boosted cannot drive the counter below zero. UnRetweet
	// reports that there was nothing to undo, and the count stays clamped.
	s.Error(s.repo.UnRetweet(ulid.Make().String(), tweet.Id, false))

	count, err = s.repo.RetweetsCount(tweet.Id)
	s.Require().NoError(err)
	s.Equal(uint64(0), count, "counter must clamp at zero, never wrap")
}

// UnRetweet is not idempotent by design: undoing something that was never done
// is reported rather than silently accepted, and it must not invent a counter.
func (s *TweetRepoHostileTestSuite) TestUnRetweetOfNeverRetweetedTweetIsRejected() {
	author := ulid.Make().String()
	tweet := s.newTweet(author, "nobody boosted this")

	s.Error(s.repo.UnRetweet(ulid.Make().String(), tweet.Id, false))

	_, err := s.repo.RetweetsCount(tweet.Id)
	s.ErrorIs(err, ErrTweetNotFound, "a never-retweeted tweet has no counter at all")
}

func (s *TweetRepoHostileTestSuite) TestRetweetsCountRejectsEmptyIDAndUnknownTweet() {
	_, err := s.repo.RetweetsCount("")
	s.Error(err)

	_, err = s.repo.RetweetsCount(ulid.Make().String())
	s.ErrorIs(err, ErrTweetNotFound)
}

func (s *TweetRepoHostileTestSuite) TestRetweetersRejectsEmptyIDAndListsDistinctBoosters() {
	_, _, err := s.repo.Retweeters("", nil, nil)
	s.Error(err)

	author := ulid.Make().String()
	tweet := s.newTweet(author, "popular")

	boosters := []string{ulid.Make().String(), ulid.Make().String(), ulid.Make().String()}
	for _, b := range boosters {
		booster := b
		_, err := s.repo.NewRetweet(domain.Tweet{
			Id: tweet.Id, UserId: author, Text: tweet.Text, RetweetedBy: &booster,
		}, false)
		s.Require().NoError(err)
	}

	got, _, err := s.repo.Retweeters(tweet.Id, nil, nil)
	s.Require().NoError(err)
	s.Len(got, len(boosters))
	for _, b := range boosters {
		s.Contains(got, b)
	}

	count, err := s.repo.RetweetsCount(tweet.Id)
	s.Require().NoError(err)
	s.Equal(uint64(len(boosters)), count)
}

// The same user boosting twice must not inflate the retweeter list.
func (s *TweetRepoHostileTestSuite) TestDoubleRetweetDoesNotDuplicateRetweeter() {
	author := ulid.Make().String()
	booster := ulid.Make().String()
	tweet := s.newTweet(author, "double boost")

	for i := 0; i < 2; i++ {
		_, err := s.repo.NewRetweet(domain.Tweet{
			Id: tweet.Id, UserId: author, Text: tweet.Text, RetweetedBy: &booster,
		}, false)
		s.Require().NoError(err)
	}

	got, _, err := s.repo.Retweeters(tweet.Id, nil, nil)
	s.Require().NoError(err)
	s.Len(got, 1, "one user is one retweeter no matter how often they click")
}

func (s *TweetRepoHostileTestSuite) TestRetweetWithoutRetweeterIsRejected() {
	author := ulid.Make().String()
	tweet := s.newTweet(author, "orphan boost")

	_, err := s.repo.NewRetweet(domain.Tweet{Id: tweet.Id, UserId: author, Text: tweet.Text}, false)
	s.Error(err, "a retweet with no retweeting user is malformed")
}

// ---------------------------------------------------------------------------
// Counts, deletion and reply threading.
// ---------------------------------------------------------------------------

func (s *TweetRepoHostileTestSuite) TestDeleteRemovesTweetAndReportsSecondAttempt() {
	userId := ulid.Make().String()
	tweet := s.newTweet(userId, "delete me")

	s.Require().NoError(s.repo.Delete(userId, tweet.Id))

	_, err := s.repo.Get(userId, tweet.Id)
	s.ErrorIs(err, ErrTweetNotFound)

	// Deleting again is reported rather than silently accepted — but it must
	// stay an error, never a panic or a resurrected row.
	s.Error(s.repo.Delete(userId, tweet.Id))

	_, err = s.repo.Get(userId, tweet.Id)
	s.ErrorIs(err, ErrTweetNotFound)
}

// A malformed request from a peer must produce an error, never a panic that
// takes the whole node down.
func (s *TweetRepoHostileTestSuite) TestGetRejectsEmptyIdentifiers() {
	_, err := s.repo.Get("", ulid.Make().String())
	s.Error(err)
	_, err = s.repo.Get(ulid.Make().String(), "")
	s.Error(err)
	_, err = s.repo.Get("", "")
	s.Error(err)
}

func (s *TweetRepoHostileTestSuite) TestRepliesCountAndGetReplyRejectBadInput() {
	_, err := s.repo.RepliesCount("")
	s.Error(err)

	_, err = s.repo.GetReply(ulid.Make().String(), ulid.Make().String())
	s.Error(err, "a reply that was never stored must not resolve")
}

func (s *TweetRepoHostileTestSuite) TestReplyToOwnTweetThreadsAndCounts() {
	author := ulid.Make().String()
	replier := ulid.Make().String()
	parent := s.newTweet(author, "thread root")

	reply, err := s.repo.AddReply(domain.Tweet{
		UserId:       replier,
		Text:         "first reply",
		ParentId:     &parent.Id,
		ParentUserId: &author,
	}, false)
	s.Require().NoError(err)
	s.True(reply.IsReply())

	count, err := s.repo.RepliesCount(parent.Id)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	got, err := s.repo.GetReply(parent.Id, reply.Id)
	s.Require().NoError(err)
	s.Equal("first reply", got.Text)

	// Deleting the reply must bring the counter back down, not leave it stuck.
	_, err = s.repo.DeleteReply(parent.Id, reply.Id, false)
	s.Require().NoError(err)

	count, err = s.repo.RepliesCount(parent.Id)
	s.Require().NoError(err)
	s.Equal(uint64(0), count)
}

func (s *TweetRepoHostileTestSuite) TestTweetsCountTracksCreateAndSurvivesUnknownUser() {
	userId := ulid.Make().String()

	count, err := s.repo.TweetsCount(userId)
	if err == nil {
		s.Equal(uint64(0), count, "a user with no tweets has zero, never garbage")
	}

	for i := 0; i < 3; i++ {
		s.newTweet(userId, "post")
	}

	count, err = s.repo.TweetsCount(userId)
	s.Require().NoError(err)
	s.Equal(uint64(3), count)
}

func (s *TweetRepoHostileTestSuite) TestListPaginatesNewestFirstWithoutRepeats() {
	userId := ulid.Make().String()

	total := 5
	for i := 0; i < total; i++ {
		_, err := s.repo.Create(userId, domain.Tweet{
			UserId:    userId,
			Text:      "post",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
		s.Require().NoError(err)
	}

	limit := uint64(2)
	seen := make(map[string]struct{})
	var cursor *string

	for page := 0; page < total; page++ {
		items, next, err := s.repo.List(userId, &limit, cursor)
		s.Require().NoError(err)
		if len(items) == 0 {
			break
		}
		for _, t := range items {
			_, dup := seen[t.Id]
			s.False(dup, "pagination must not replay tweet %s", t.Id)
			seen[t.Id] = struct{}{}
		}
		if next == "" {
			break
		}
		cursor = &next
	}

	s.Len(seen, total, "pagination must reach every tweet exactly once")
}

func TestTweetRepoHostileTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(TweetRepoHostileTestSuite))
}
