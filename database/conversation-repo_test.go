//nolint:all
package database

import (
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type ConversationsRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *ConversationsRepo
}

func (s *ConversationsRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
	s.repo = NewConversationsRepo(s.db)
}

func (s *ConversationsRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *ConversationsRepoTestSuite) TestTouchAndList() {
	user := uuid.New().String()
	t1 := uuid.New().String()
	t2 := uuid.New().String()

	s.Require().NoError(s.repo.Touch(user, t1, time.Now().Add(-2*time.Second)))
	s.Require().NoError(s.repo.Touch(user, t2, time.Now().Add(-1*time.Second)))

	limit := uint64(10)
	ids, _, err := s.repo.List(user, &limit, nil)
	s.Require().NoError(err)
	s.Len(ids, 2)
	// Newest first.
	s.Equal(t2, ids[0])
	s.Equal(t1, ids[1])
}

func (s *ConversationsRepoTestSuite) TestTouchIdempotentReplaces() {
	user := uuid.New().String()
	tweet := uuid.New().String()

	s.Require().NoError(s.repo.Touch(user, tweet, time.Now().Add(-2*time.Second)))
	// Touching again with a fresher timestamp should keep exactly one
	// list entry, not two.
	s.Require().NoError(s.repo.Touch(user, tweet, time.Now()))

	limit := uint64(10)
	ids, _, err := s.repo.List(user, &limit, nil)
	s.Require().NoError(err)
	s.Len(ids, 1)
}

func (s *ConversationsRepoTestSuite) TestHide() {
	user := uuid.New().String()
	tweet := uuid.New().String()
	s.Require().NoError(s.repo.Touch(user, tweet, time.Now()))

	s.Require().NoError(s.repo.Hide(user, tweet))

	limit := uint64(10)
	ids, _, err := s.repo.List(user, &limit, nil)
	s.Require().NoError(err)
	s.Empty(ids)
}

func (s *ConversationsRepoTestSuite) TestHideNonexistentIsNoop() {
	err := s.repo.Hide(uuid.New().String(), uuid.New().String())
	s.NoError(err)
}

func (s *ConversationsRepoTestSuite) TestValidation() {
	s.Error(s.repo.Touch("", "t", time.Now()))
	s.Error(s.repo.Touch("u", "", time.Now()))
	s.Error(s.repo.Hide("", "t"))
	s.Error(s.repo.Hide("u", ""))
	_, _, err := s.repo.List("", nil, nil)
	s.Error(err)
}

func (s *ConversationsRepoTestSuite) TestIsolatedByUser() {
	u1 := uuid.New().String()
	u2 := uuid.New().String()
	tweet := uuid.New().String()

	s.Require().NoError(s.repo.Touch(u1, tweet, time.Now()))

	limit := uint64(10)
	u1ids, _, err := s.repo.List(u1, &limit, nil)
	s.Require().NoError(err)
	s.Len(u1ids, 1)

	u2ids, _, err := s.repo.List(u2, &limit, nil)
	s.Require().NoError(err)
	s.Empty(u2ids)
}

func TestConversationsRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(ConversationsRepoTestSuite))
}
