//nolint:all
package database

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type FollowerRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *FollowRepo
}

func (s *FollowerRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	err = s.db.Run("test", "test")
	s.Require().NoError(err)

	s.repo = NewFollowRepo(s.db)
}

func (s *FollowerRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *FollowerRepoTestSuite) TestFollow_Success() {
	err := s.repo.Follow("user1", "user2")
	assert.NoError(s.T(), err)
}

func (s *FollowerRepoTestSuite) TestFollow_EmptyParams() {
	err := s.repo.Follow("", "user2")
	assert.Error(s.T(), err)

	err = s.repo.Follow("user1", "")
	assert.Error(s.T(), err)
}

func (s *FollowerRepoTestSuite) TestFollow_Self() {
	err := s.repo.Follow("userX", "userX")
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "cannot follow yourself")
}

func (s *FollowerRepoTestSuite) TestFollow_AlreadyFollowed() {
	err := s.repo.Follow("follower1", "following1")
	assert.NoError(s.T(), err)

	err = s.repo.Follow("follower1", "following1")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrAlreadyFollowed, err)
}

func (s *FollowerRepoTestSuite) TestIsFollowing() {
	err := s.repo.Follow("a", "b")
	assert.NoError(s.T(), err)

	assert.True(s.T(), s.repo.IsFollowing("a", "b"))
	assert.False(s.T(), s.repo.IsFollowing("b", "a"))
	assert.False(s.T(), s.repo.IsFollowing("nonexistent", "b"))
}

func (s *FollowerRepoTestSuite) TestIsFollower() {
	err := s.repo.Follow("c", "d")
	assert.NoError(s.T(), err)

	assert.True(s.T(), s.repo.IsFollower("d", "c"))
	assert.False(s.T(), s.repo.IsFollower("c", "d"))
}

func (s *FollowerRepoTestSuite) TestUnfollow() {
	err := s.repo.Follow("e", "f")
	assert.NoError(s.T(), err)
	assert.True(s.T(), s.repo.IsFollowing("e", "f"))

	err = s.repo.Unfollow("e", "f")
	assert.NoError(s.T(), err)
	assert.False(s.T(), s.repo.IsFollowing("e", "f"))
}

func (s *FollowerRepoTestSuite) TestGetFollowersCount() {
	err := s.repo.Follow("g1", "target1")
	assert.NoError(s.T(), err)
	err = s.repo.Follow("g2", "target1")
	assert.NoError(s.T(), err)

	count, err := s.repo.GetFollowersCount("target1")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), uint64(2), count)
}

func (s *FollowerRepoTestSuite) TestGetFollowersCount_Empty() {
	count, err := s.repo.GetFollowersCount("nobody")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), uint64(0), count)
}

func (s *FollowerRepoTestSuite) TestGetFollowersCount_EmptyUserID() {
	_, err := s.repo.GetFollowersCount("")
	assert.Error(s.T(), err)
}

func (s *FollowerRepoTestSuite) TestGetFollowingsCount() {
	err := s.repo.Follow("h1", "target2")
	assert.NoError(s.T(), err)
	err = s.repo.Follow("h1", "target3")
	assert.NoError(s.T(), err)

	count, err := s.repo.GetFollowingsCount("h1")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), uint64(2), count)
}

func (s *FollowerRepoTestSuite) TestGetFollowingsCount_EmptyUserID() {
	_, err := s.repo.GetFollowingsCount("")
	assert.Error(s.T(), err)
}

func (s *FollowerRepoTestSuite) TestGetFollowers() {
	err := s.repo.Follow("i1", "target4")
	assert.NoError(s.T(), err)
	err = s.repo.Follow("i2", "target4")
	assert.NoError(s.T(), err)

	followers, cursor, err := s.repo.GetFollowers("target4", nil, nil)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), followers, 2)
	assert.NotEmpty(s.T(), cursor)
}

func (s *FollowerRepoTestSuite) TestGetFollowings() {
	err := s.repo.Follow("j1", "target5")
	assert.NoError(s.T(), err)
	err = s.repo.Follow("j1", "target6")
	assert.NoError(s.T(), err)

	followings, cursor, err := s.repo.GetFollowings("j1", nil, nil)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), followings, 2)
	assert.NotEmpty(s.T(), cursor)
}

func TestFollowerRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(FollowerRepoTestSuite))
}
