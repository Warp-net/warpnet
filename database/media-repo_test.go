//nolint:all
package database

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type MediaRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *MediaRepo
}

func (s *MediaRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	err = s.db.Run("test", "test")
	s.Require().NoError(err)

	s.repo = NewMediaRepo(s.db)
}

func (s *MediaRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *MediaRepoTestSuite) TestSetImage_Success() {
	key, err := s.repo.SetImage("user1", Base64Image("data:image/png;base64,iVBOR..."))
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), key)
}

func (s *MediaRepoTestSuite) TestSetImage_EmptyImage() {
	_, err := s.repo.SetImage("user1", "")
	assert.Error(s.T(), err)
}

func (s *MediaRepoTestSuite) TestSetImage_EmptyUserId() {
	_, err := s.repo.SetImage("", Base64Image("somedata"))
	assert.Error(s.T(), err)
}

func (s *MediaRepoTestSuite) TestGetImage_Success() {
	img := Base64Image("data:image/png;base64,testdata123")
	key, err := s.repo.SetImage("user2", img)
	assert.NoError(s.T(), err)

	retrieved, err := s.repo.GetImage("user2", string(key))
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), img, retrieved)
}

func (s *MediaRepoTestSuite) TestGetImage_NotFound() {
	_, err := s.repo.GetImage("user2", "nonexistent-key")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaNotFound, err)
}

func (s *MediaRepoTestSuite) TestGetImage_EmptyKey() {
	_, err := s.repo.GetImage("user2", "")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaNotFound, err)
}

func (s *MediaRepoTestSuite) TestGetImage_EmptyUserId() {
	_, err := s.repo.GetImage("", "somekey")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaNotFound, err)
}

func (s *MediaRepoTestSuite) TestSetImage_DeterministicKey() {
	img := Base64Image("deterministic-content")
	k1, _ := s.repo.SetImage("user3", img)
	k2, _ := s.repo.SetImage("user3", img)
	assert.Equal(s.T(), k1, k2, "same content should produce same key")
}

func (s *MediaRepoTestSuite) TestSetForeignImageWithTTL_Success() {
	err := s.repo.SetForeignImageWithTTL("user4", "foreign-key", Base64Image("foreign-data"))
	assert.NoError(s.T(), err)

	retrieved, err := s.repo.GetImage("user4", "foreign-key")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), Base64Image("foreign-data"), retrieved)
}

func (s *MediaRepoTestSuite) TestSetForeignImageWithTTL_EmptyImage() {
	err := s.repo.SetForeignImageWithTTL("user4", "key", "")
	assert.Error(s.T(), err)
}

func (s *MediaRepoTestSuite) TestSetForeignImageWithTTL_EmptyUserId() {
	err := s.repo.SetForeignImageWithTTL("", "key", Base64Image("data"))
	assert.Error(s.T(), err)
}

func (s *MediaRepoTestSuite) TestSetForeignImageWithTTL_EmptyKey() {
	err := s.repo.SetForeignImageWithTTL("user4", "", Base64Image("data"))
	assert.Error(s.T(), err)
}

func (s *MediaRepoTestSuite) TestNilRepo() {
	var repo *MediaRepo
	_, err := repo.GetImage("user", "key")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaRepoNotInit, err)

	_, err = repo.SetImage("user", Base64Image("data"))
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaRepoNotInit, err)

	err = repo.SetForeignImageWithTTL("user", "key", Base64Image("data"))
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaRepoNotInit, err)
}

func TestMediaRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(MediaRepoTestSuite))
}
