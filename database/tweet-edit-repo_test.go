//nolint:all
package database

import (
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type TweetEditsRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *TweetEditsRepo
}

func (s *TweetEditsRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
	s.repo = NewTweetEditsRepo(s.db)
}

func (s *TweetEditsRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *TweetEditsRepoTestSuite) TestAppendAndList() {
	tweetId := uuid.New().String()
	userId := uuid.New().String()

	e1, err := s.repo.Append(domain.TweetEdit{
		OriginalTweetId: tweetId,
		UserId:          userId,
		Text:            "v1",
		EditedAt:        time.Now().Add(-2 * time.Second),
	})
	s.Require().NoError(err)
	s.NotEmpty(e1.Id, "id should be auto-generated")

	_, err = s.repo.Append(domain.TweetEdit{
		OriginalTweetId: tweetId,
		UserId:          userId,
		Text:            "v2",
		EditedAt:        time.Now().Add(-1 * time.Second),
	})
	s.Require().NoError(err)

	limit := uint64(10)
	edits, _, err := s.repo.List(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Len(edits, 2)
	// Stored reverse-chronological — newest first.
	s.Equal("v2", edits[0].Text)
	s.Equal("v1", edits[1].Text)
}

func (s *TweetEditsRepoTestSuite) TestAppendValidation() {
	_, err := s.repo.Append(domain.TweetEdit{UserId: "u", Text: "x"})
	s.Error(err)
	_, err = s.repo.Append(domain.TweetEdit{OriginalTweetId: "t", Text: "x"})
	s.Error(err)
	_, err = s.repo.Append(domain.TweetEdit{OriginalTweetId: "t", UserId: "u"})
	s.Error(err)
}

func (s *TweetEditsRepoTestSuite) TestListEmpty() {
	limit := uint64(10)
	edits, _, err := s.repo.List(uuid.New().String(), &limit, nil)
	s.Require().NoError(err)
	s.Empty(edits)
}

func (s *TweetEditsRepoTestSuite) TestListValidation() {
	_, _, err := s.repo.List("", nil, nil)
	s.Error(err)
}

func (s *TweetEditsRepoTestSuite) TestIsolatedByTweet() {
	t1 := uuid.New().String()
	t2 := uuid.New().String()
	user := uuid.New().String()

	_, err := s.repo.Append(domain.TweetEdit{OriginalTweetId: t1, UserId: user, Text: "for t1"})
	s.Require().NoError(err)
	_, err = s.repo.Append(domain.TweetEdit{OriginalTweetId: t2, UserId: user, Text: "for t2"})
	s.Require().NoError(err)

	limit := uint64(10)
	t1Edits, _, err := s.repo.List(t1, &limit, nil)
	s.Require().NoError(err)
	s.Len(t1Edits, 1)
	s.Equal("for t1", t1Edits[0].Text)
}

func TestTweetEditsRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(TweetEditsRepoTestSuite))
}
