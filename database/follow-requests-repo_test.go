//nolint:all
package database

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type FollowRequestsRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *FollowRequestsRepo
}

func (s *FollowRequestsRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
	s.repo = NewFollowRequestsRepo(s.db)
}

func (s *FollowRequestsRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *FollowRequestsRepoTestSuite) TestAddHasRemove() {
	target := uuid.New().String()
	f1 := uuid.New().String()
	f2 := uuid.New().String()

	has, err := s.repo.Has(target, f1)
	s.Require().NoError(err)
	s.False(has)

	s.Require().NoError(s.repo.Add(target, f1))
	s.Require().NoError(s.repo.Add(target, f2))

	has, err = s.repo.Has(target, f1)
	s.Require().NoError(err)
	s.True(has)

	limit := uint64(10)
	ids, _, err := s.repo.List(target, &limit, nil)
	s.Require().NoError(err)
	s.Len(ids, 2)

	s.Require().NoError(s.repo.Remove(target, f1))
	has, err = s.repo.Has(target, f1)
	s.Require().NoError(err)
	s.False(has)
}

func (s *FollowRequestsRepoTestSuite) TestEmptyValidation() {
	s.Error(s.repo.Add("", "f"))
	s.Error(s.repo.Add("u", ""))
	s.Error(s.repo.Remove("", "f"))
	s.Error(s.repo.Remove("u", ""))
	_, _, err := s.repo.List("", nil, nil)
	s.Error(err)

	// Has tolerates empty inputs (returns false, nil).
	has, err := s.repo.Has("", "f")
	s.NoError(err)
	s.False(has)
}

func (s *FollowRequestsRepoTestSuite) TestRemoveNonexistentIsNoop() {
	err := s.repo.Remove(uuid.New().String(), uuid.New().String())
	s.NoError(err)
}

func (s *FollowRequestsRepoTestSuite) TestIsolatedByTarget() {
	t1 := uuid.New().String()
	t2 := uuid.New().String()
	follower := uuid.New().String()

	s.Require().NoError(s.repo.Add(t1, follower))

	has, err := s.repo.Has(t1, follower)
	s.Require().NoError(err)
	s.True(has)

	has, err = s.repo.Has(t2, follower)
	s.Require().NoError(err)
	s.False(has, "t2 should not see t1's pending request")
}

func TestFollowRequestsRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(FollowRequestsRepoTestSuite))
}
