//nolint:all
package database

import (
	"strings"
	"testing"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
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

// ---------------------------------------------------------------------------
// Media storage — content-addressed blobs.
// ---------------------------------------------------------------------------

func (s *MediaRepoTestSuite) TestNilMediaRepoNeverPanics() {
	var repo *MediaRepo

	_, err := repo.GetImage("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.SetImage("u", "img")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetVideo("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.SetVideo("u", "vid")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetImageMeta("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)

	s.ErrorIs(repo.SetImageMeta("u", "k", MediaMeta{}), ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignImageWithTTL("u", "k", "img"), ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignVideoWithTTL("u", "k", "vid"), ErrMediaRepoNotInit)
}

// Media is content-addressed: the same bytes must always produce the same key,
// so re-uploading an image doesn't duplicate megabytes of storage.
func (s *MediaRepoTestSuite) TestImageKeyIsContentAddressed() {
	user := uuid.New().String()
	img := Base64Video("data:image/jpeg;base64,AAAA")

	first, err := s.repo.SetImage(user, Base64Image(img))
	s.Require().NoError(err)
	second, err := s.repo.SetImage(user, Base64Image(img))
	s.Require().NoError(err)
	s.Equal(first, second, "identical bytes must hash to the same key")

	other, err := s.repo.SetImage(user, "data:image/jpeg;base64,BBBB")
	s.Require().NoError(err)
	s.NotEqual(first, other, "different bytes must not collide")
}

func (s *MediaRepoTestSuite) TestImageRoundTripAndIsolation() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	key, err := s.repo.SetImage(alice, "data:image/jpeg;base64,ALICE")
	s.Require().NoError(err)

	got, err := s.repo.GetImage(alice, string(key))
	s.Require().NoError(err)
	s.Equal(Base64Image("data:image/jpeg;base64,ALICE"), got)

	// The same content key under another user must not resolve — media is
	// namespaced per owner.
	_, err = s.repo.GetImage(bob, string(key))
	s.ErrorIs(err, ErrMediaNotFound)
}

func (s *MediaRepoTestSuite) TestVideoRoundTripAndIsolation() {
	alice := uuid.New().String()
	bob := uuid.New().String()

	key, err := s.repo.SetVideo(alice, "data:video/mp4;base64,ALICE")
	s.Require().NoError(err)

	got, err := s.repo.GetVideo(alice, string(key))
	s.Require().NoError(err)
	s.Equal(Base64Video("data:video/mp4;base64,ALICE"), got)

	_, err = s.repo.GetVideo(bob, string(key))
	s.ErrorIs(err, ErrMediaNotFound)
}

func (s *MediaRepoTestSuite) TestMediaRejectsEmptyInput() {
	user := uuid.New().String()

	_, err := s.repo.SetImage(user, "")
	s.Error(err)
	_, err = s.repo.SetImage("", "data")
	s.Error(err)
	_, err = s.repo.SetVideo(user, "")
	s.Error(err)
	_, err = s.repo.SetVideo("", "data")
	s.Error(err)

	_, err = s.repo.GetImage("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.repo.GetImage(user, "")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.repo.GetVideo("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.repo.GetVideo(user, "")
	s.ErrorIs(err, ErrMediaNotFound)

	s.Error(s.repo.SetForeignImageWithTTL(user, "", "data"))
	s.Error(s.repo.SetForeignImageWithTTL(user, "k", ""))
	s.Error(s.repo.SetForeignImageWithTTL("", "k", "data"))
	s.Error(s.repo.SetForeignVideoWithTTL(user, "", "data"))
	s.Error(s.repo.SetForeignVideoWithTTL(user, "k", ""))
	s.Error(s.repo.SetForeignVideoWithTTL("", "k", "data"))
}

func (s *MediaRepoTestSuite) TestForeignMediaIsReadableUnderTheGivenKey() {
	user := uuid.New().String()

	s.Require().NoError(s.repo.SetForeignImageWithTTL(user, "remote-key", "data:image/jpeg;base64,REMOTE"))
	got, err := s.repo.GetImage(user, "remote-key")
	s.Require().NoError(err)
	s.Equal(Base64Image("data:image/jpeg;base64,REMOTE"), got)

	s.Require().NoError(s.repo.SetForeignVideoWithTTL(user, "remote-clip", "data:video/mp4;base64,REMOTE"))
	vid, err := s.repo.GetVideo(user, "remote-clip")
	s.Require().NoError(err)
	s.Equal(Base64Video("data:video/mp4;base64,REMOTE"), vid)
}

// Images and videos live in separate namespaces: a video key must never
// resolve as an image, even when the bytes happen to match.
func (s *MediaRepoTestSuite) TestImageAndVideoNamespacesDoNotCollide() {
	user := uuid.New().String()
	payload := "data:application/octet-stream;base64,SAME"

	imgKey, err := s.repo.SetImage(user, Base64Image(payload))
	s.Require().NoError(err)
	vidKey, err := s.repo.SetVideo(user, Base64Video(payload))
	s.Require().NoError(err)
	s.Equal(string(imgKey), string(vidKey), "the content hash is the same")

	_, err = s.repo.GetVideo(user, string(imgKey))
	s.Require().NoError(err, "each namespace holds its own copy")

	other := uuid.New().String()
	_, err = s.repo.GetImage(other, string(imgKey))
	s.ErrorIs(err, ErrMediaNotFound)
}

// ---------------------------------------------------------------------------
// Alt-text / focal point metadata.
// ---------------------------------------------------------------------------

// "Never described" and "described as empty" must be indistinguishable to the
// caller, so a missing record is a zero value rather than an error.
func (s *MediaRepoTestSuite) TestImageMetaDefaultsToZeroValue() {
	user := uuid.New().String()

	meta, err := s.repo.GetImageMeta(user, "never-described")
	s.Require().NoError(err)
	s.Equal(MediaMeta{}, meta)
}

func (s *MediaRepoTestSuite) TestImageMetaRoundTripAndOverwrite() {
	user := uuid.New().String()

	s.Require().NoError(s.repo.SetImageMeta(user, "key", MediaMeta{
		Description: "a cat on a keyboard",
		FocusX:      -0.5,
		FocusY:      0.25,
	}))

	meta, err := s.repo.GetImageMeta(user, "key")
	s.Require().NoError(err)
	s.Equal("a cat on a keyboard", meta.Description)
	s.InDelta(-0.5, meta.FocusX, 0.0001)
	s.InDelta(0.25, meta.FocusY, 0.0001)

	// Editing alt-text must replace, not merge.
	s.Require().NoError(s.repo.SetImageMeta(user, "key", MediaMeta{Description: "corrected"}))
	meta, err = s.repo.GetImageMeta(user, "key")
	s.Require().NoError(err)
	s.Equal("corrected", meta.Description)
	s.Zero(meta.FocusX)
	s.Zero(meta.FocusY)
}

func (s *MediaRepoTestSuite) TestImageMetaSurvivesExoticDescriptions() {
	user := uuid.New().String()

	inputs := []string{
		strings.Repeat("описание ", 500),
		`<img src=x onerror="alert(1)">`,
		"line\nbreak\ttab",
		"🔥🙈 emoji alt text",
		`{"json":"injection"}`,
	}

	for i, desc := range inputs {
		key := uuid.New().String()
		s.Require().NoErrorf(s.repo.SetImageMeta(user, key, MediaMeta{Description: desc}), "case %d", i)

		meta, err := s.repo.GetImageMeta(user, key)
		s.Require().NoError(err)
		s.Equalf(desc, meta.Description, "alt-text must round trip verbatim (case %d)", i)
	}
}

func (s *MediaRepoTestSuite) TestImageMetaRejectsEmptyIdentifiers() {
	s.Error(s.repo.SetImageMeta("", "k", MediaMeta{}))
	s.Error(s.repo.SetImageMeta("u", "", MediaMeta{}))

	meta, err := s.repo.GetImageMeta("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	s.Equal(MediaMeta{}, meta)

	meta, err = s.repo.GetImageMeta("u", "")
	s.ErrorIs(err, ErrMediaNotFound)
	s.Equal(MediaMeta{}, meta)
}

// Metadata lives beside the blob, so describing an image must not touch the
// (potentially megabyte-sized) payload.
func (s *MediaRepoTestSuite) TestImageMetaDoesNotDisturbTheBlob() {
	user := uuid.New().String()
	key, err := s.repo.SetImage(user, "data:image/jpeg;base64,PAYLOAD")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.SetImageMeta(user, string(key), MediaMeta{Description: "alt"}))

	img, err := s.repo.GetImage(user, string(key))
	s.Require().NoError(err)
	s.Equal(Base64Image("data:image/jpeg;base64,PAYLOAD"), img)
}
