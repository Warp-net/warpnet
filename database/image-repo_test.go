//nolint:all
package database

import (
	"testing"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type ImageRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *ImageRepo
}

func (s *ImageRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	err = s.db.Run("test", "test")
	s.Require().NoError(err)

	s.repo = NewImageRepo(s.db)
}

func (s *ImageRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *ImageRepoTestSuite) TestSetImage_Success() {
	key, err := s.repo.SetImage("user1", domain.Base64Image("data:image/png;base64,iVBOR..."))
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), key)
}

func (s *ImageRepoTestSuite) TestSetImage_EmptyImage() {
	_, err := s.repo.SetImage("user1", "")
	assert.Error(s.T(), err)
}

func (s *ImageRepoTestSuite) TestSetImage_EmptyUserId() {
	_, err := s.repo.SetImage("", domain.Base64Image("somedata"))
	assert.Error(s.T(), err)
}

func (s *ImageRepoTestSuite) TestGetImage_Success() {
	img := domain.Base64Image("data:image/png;base64,testdata123")
	key, err := s.repo.SetImage("user2", img)
	assert.NoError(s.T(), err)

	retrieved, err := s.repo.GetImage("user2", string(key))
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), img, retrieved)
}

func (s *ImageRepoTestSuite) TestGetImage_NotFound() {
	_, err := s.repo.GetImage("user2", "nonexistent-key")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaNotFound, err)
}

func (s *ImageRepoTestSuite) TestGetImage_EmptyKey() {
	_, err := s.repo.GetImage("user2", "")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaNotFound, err)
}

func (s *ImageRepoTestSuite) TestGetImage_EmptyUserId() {
	_, err := s.repo.GetImage("", "somekey")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaNotFound, err)
}

func (s *ImageRepoTestSuite) TestSetImage_DeterministicKey() {
	img := domain.Base64Image("deterministic-content")
	k1, _ := s.repo.SetImage("user3", img)
	k2, _ := s.repo.SetImage("user3", img)
	assert.Equal(s.T(), k1, k2, "same content should produce same key")
}

func (s *ImageRepoTestSuite) TestSetForeignImageWithTTL_Success() {
	err := s.repo.SetForeignImageWithTTL("user4", "foreign-key", domain.Base64Image("foreign-data"))
	assert.NoError(s.T(), err)

	retrieved, err := s.repo.GetImage("user4", "foreign-key")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), domain.Base64Image("foreign-data"), retrieved)
}

func (s *ImageRepoTestSuite) TestSetForeignImageWithTTL_EmptyImage() {
	err := s.repo.SetForeignImageWithTTL("user4", "key", "")
	assert.Error(s.T(), err)
}

func (s *ImageRepoTestSuite) TestSetForeignImageWithTTL_EmptyUserId() {
	err := s.repo.SetForeignImageWithTTL("", "key", domain.Base64Image("data"))
	assert.Error(s.T(), err)
}

func (s *ImageRepoTestSuite) TestSetForeignImageWithTTL_EmptyKey() {
	err := s.repo.SetForeignImageWithTTL("user4", "", domain.Base64Image("data"))
	assert.Error(s.T(), err)
}

func (s *ImageRepoTestSuite) TestNilRepo() {
	var repo *ImageRepo
	_, err := repo.GetImage("user", "key")
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaRepoNotInit, err)

	_, err = repo.SetImage("user", domain.Base64Image("data"))
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaRepoNotInit, err)

	err = repo.SetForeignImageWithTTL("user", "key", domain.Base64Image("data"))
	assert.Error(s.T(), err)
	assert.Equal(s.T(), ErrMediaRepoNotInit, err)
}

func TestImageRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(ImageRepoTestSuite))
}

func (s *ImageRepoTestSuite) TestNilImageRepoNeverPanics() {
	var repo *ImageRepo

	_, err := repo.GetImage("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.SetImage("u", "img")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignImageWithTTL("u", "k", "img"), ErrMediaRepoNotInit)
}

func (s *ImageRepoTestSuite) TestImageKeyIsContentAddressed() {
	user := uuid.New().String()
	img := domain.Base64Video("data:image/jpeg;base64,AAAA")

	first, err := s.repo.SetImage(user, domain.Base64Image(img))
	s.Require().NoError(err)
	second, err := s.repo.SetImage(user, domain.Base64Image(img))
	s.Require().NoError(err)
	s.Equal(first, second, "identical bytes must hash to the same key")

	other, err := s.repo.SetImage(user, "data:image/jpeg;base64,BBBB")
	s.Require().NoError(err)
	s.NotEqual(first, other, "different bytes must not collide")
}

func (s *ImageRepoTestSuite) TestImageRoundTripAndIsolation() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	key, err := s.repo.SetImage(alice, "data:image/jpeg;base64,ALICE")
	s.Require().NoError(err)

	got, err := s.repo.GetImage(alice, string(key))
	s.Require().NoError(err)
	s.Equal(domain.Base64Image("data:image/jpeg;base64,ALICE"), got)

	_, err = s.repo.GetImage(bob, string(key))
	s.ErrorIs(err, ErrMediaNotFound)
}

func (s *ImageRepoTestSuite) TestImageRejectsEmptyInput() {
	user := uuid.New().String()

	_, err := s.repo.SetImage(user, "")
	s.Error(err)
	_, err = s.repo.SetImage("", "data")
	s.Error(err)

	_, err = s.repo.GetImage("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.repo.GetImage(user, "")
	s.ErrorIs(err, ErrMediaNotFound)

	s.Error(s.repo.SetForeignImageWithTTL(user, "", "data"))
	s.Error(s.repo.SetForeignImageWithTTL(user, "k", ""))
	s.Error(s.repo.SetForeignImageWithTTL("", "k", "data"))
}

func (s *ImageRepoTestSuite) TestForeignMediaIsReadableUnderTheGivenKey() {
	user := uuid.New().String()

	s.Require().NoError(s.repo.SetForeignImageWithTTL(user, "remote-key", "data:image/jpeg;base64,REMOTE"))
	got, err := s.repo.GetImage(user, "remote-key")
	s.Require().NoError(err)
	s.Equal(domain.Base64Image("data:image/jpeg;base64,REMOTE"), got)
}

func (s *ImageRepoTestSuite) TestCopyRoundTripAndIsolation() {
	alice := uuid.New().String()
	bob := uuid.New().String()
	sold := domain.MediaCopy{OriginalKey: "original-key", BuyerId: "buyer", EncryptedOrder: []byte("sealed")}

	s.Require().NoError(s.repo.SetCopy(alice, "copy-key", sold))

	got, err := s.repo.GetCopy(alice, "copy-key")
	s.Require().NoError(err)
	s.Equal(sold, got)

	_, err = s.repo.GetCopy(bob, "copy-key")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.repo.GetImage(alice, "copy-key")
	s.ErrorIs(err, ErrMediaNotFound, "a copy is a record, not a file")

	s.Error(s.repo.SetCopy(alice, "", sold))
	s.Error(s.repo.SetCopy(alice, "copy-key", domain.MediaCopy{}))
	s.Error(s.repo.SetCopy(alice, "copy-key", domain.MediaCopy{OriginalKey: "original-key"}), "a copy names its buyer")

	var repo *ImageRepo
	s.ErrorIs(repo.SetCopy(alice, "copy-key", sold), ErrMediaRepoNotInit)
	_, err = repo.GetCopy(alice, "copy-key")
	s.ErrorIs(err, ErrMediaRepoNotInit)
}

func (s *ImageRepoTestSuite) TestListCopies() {
	alice := uuid.New().String()
	longerAlice := alice + "-2"
	first := domain.MediaCopy{OriginalKey: "original-a", BuyerId: "buyer-1", IsPixelMarked: true}
	second := domain.MediaCopy{OriginalKey: "original-a", BuyerId: "buyer-2"}
	s.Require().NoError(s.repo.SetCopy(alice, "copy-1", first))
	s.Require().NoError(s.repo.SetCopy(alice, "copy-2", second))
	s.Require().NoError(s.repo.SetCopy(alice, "copy-3", domain.MediaCopy{OriginalKey: "original-b", BuyerId: "buyer-1"}))
	s.Require().NoError(s.repo.SetCopy(longerAlice, "copy-4", domain.MediaCopy{OriginalKey: "original-a", BuyerId: "buyer-3"}))

	copies, err := s.repo.ListCopies(alice, "original-a")
	s.Require().NoError(err)
	s.ElementsMatch([]domain.MediaCopy{first, second}, copies, "only this user's copies of this original")

	copies, err = s.repo.ListCopies(uuid.New().String(), "original-a")
	s.Require().NoError(err)
	s.Empty(copies)

	var repo *ImageRepo
	_, err = repo.ListCopies(alice, "original-a")
	s.ErrorIs(err, ErrMediaRepoNotInit)
}
