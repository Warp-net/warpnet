//nolint:all
package database

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"testing"

	"github.com/Warp-net/warpnet/core/media-meta"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type VideoRepoTestSuite struct {
	suite.Suite

	dir  string
	db   *local_store.DB
	repo *VideoRepo
}

func (s *VideoRepoTestSuite) SetupSuite() {
	var err error
	s.dir, err = os.MkdirTemp("", "video-repo")
	s.Require().NoError(err)

	s.db, err = local_store.New(s.dir, local_store.DefaultOptions())
	s.Require().NoError(err)

	err = s.db.Run("test", "test")
	s.Require().NoError(err)

	s.repo = NewVideoRepo(s.db)
}

func (s *VideoRepoTestSuite) TearDownSuite() {
	s.db.Close()
	_ = os.RemoveAll(s.dir)
}

func TestVideoRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(VideoRepoTestSuite))
}

func randomVideo(s *VideoRepoTestSuite, size int) ([]byte, domain.Base64Video) {
	raw := make([]byte, size)
	_, err := rand.Read(raw)
	s.Require().NoError(err)
	return raw, domain.Base64Video("data:video/mp4;base64," + base64.StdEncoding.EncodeToString(raw))
}

func (s *VideoRepoTestSuite) TestVideoRoundTripAcrossChunks() {
	alice := uuid.New().String()
	raw, video := randomVideo(s, 2<<20+512<<10)

	key, err := s.repo.SetVideo(alice, video)
	s.Require().NoError(err)

	manifest, err := s.repo.GetManifest(alice, string(key))
	s.Require().NoError(err)
	s.Equal(int64(len(raw)), manifest.Size)
	s.Equal("video/mp4", manifest.Mime)
	s.Equal(int64(1<<20), manifest.ChunkSize)
	s.Len(manifest.Hashes, 3)

	var joined []byte
	for idx := range manifest.Hashes {
		chunk, err := s.repo.GetChunk(alice, string(key), idx)
		s.Require().NoError(err)
		s.Equal(manifest.ChunkLen(idx), int64(len(chunk)))
		joined = append(joined, chunk...)
	}
	s.True(bytes.Equal(raw, joined), "the chunks put the file back together")

	whole, err := s.repo.GetVideo(alice, string(key))
	s.Require().NoError(err)
	s.Equal(video, whole, "the old routes still get the data URI they used to")

	_, err = s.repo.GetVideo(uuid.New().String(), string(key))
	s.ErrorIs(err, ErrMediaNotFound)
}

func (s *VideoRepoTestSuite) TestVideoKeyIsTheHashOfChunkHashes() {
	raw, video := randomVideo(s, 1<<20+1)

	key, err := s.repo.SetVideo(uuid.New().String(), video)
	s.Require().NoError(err)

	first := sha256.Sum256(raw[:1<<20])
	second := sha256.Sum256(raw[1<<20:])
	want := sha256.Sum256(append(first[:], second[:]...))
	s.Equal(hex.EncodeToString(want[:]), string(key))

	again, err := s.repo.SetVideo(uuid.New().String(), video)
	s.Require().NoError(err)
	s.Equal(key, again, "the same bytes get the same key")
}

func (s *VideoRepoTestSuite) TestForeignVideoIsReadableUnderTheGivenKey() {
	user := uuid.New().String()
	_, video := randomVideo(s, 1<<20)

	s.Require().NoError(s.repo.SetForeignVideoWithTTL(user, "remote-clip", video))

	got, err := s.repo.GetVideo(user, "remote-clip")
	s.Require().NoError(err)
	s.Equal(video, got)

	manifest, err := s.repo.GetManifest(user, "remote-clip")
	s.Require().NoError(err)
	s.Len(manifest.Hashes, 1)
}

func (s *VideoRepoTestSuite) TestPartlyCachedVideoIsNotWhole() {
	user := uuid.New().String()
	raw, _ := randomVideo(s, 2<<20)
	manifest := media_meta.BuildVideoManifest(raw, "video/mp4")
	key := manifest.Key()

	s.Require().NoError(s.repo.SetForeignManifestWithTTL(user, key, manifest))
	s.Require().NoError(s.repo.SetForeignChunkWithTTL(user, key, 0, raw[:1<<20]))

	chunk, err := s.repo.GetChunk(user, key, 0)
	s.Require().NoError(err)
	s.True(bytes.Equal(raw[:1<<20], chunk))

	_, err = s.repo.GetChunk(user, key, 1)
	s.ErrorIs(err, ErrMediaNotFound, "a missing chunk is fetched on demand")
	_, err = s.repo.GetVideo(user, key)
	s.ErrorIs(err, ErrMediaNotFound, "a partial cache is not a whole file")
}

func (s *VideoRepoTestSuite) TestVideoRejectsEmptyInput() {
	user := uuid.New().String()

	_, err := s.repo.SetVideo(user, "")
	s.Error(err)
	_, err = s.repo.SetVideo("", "data:video/mp4;base64,AAAA")
	s.Error(err)
	_, err = s.repo.SetVideo(user, "not a data uri")
	s.Error(err)
	_, err = s.repo.SetVideo(user, "data:video/mp4;base64,")
	s.Error(err)

	_, err = s.repo.GetVideo("", "k")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.repo.GetVideo(user, "")
	s.ErrorIs(err, ErrMediaNotFound)
	_, err = s.repo.GetChunk(user, "k", -1)
	s.ErrorIs(err, ErrMediaNotFound)

	s.Error(s.repo.SetForeignVideoWithTTL(user, "", "data:video/mp4;base64,AAAA"))
	s.Error(s.repo.SetForeignVideoWithTTL(user, "k", ""))
	s.Error(s.repo.SetForeignVideoWithTTL("", "k", "data:video/mp4;base64,AAAA"))
	s.Error(s.repo.SetForeignManifestWithTTL(user, "k", domain.VideoManifest{}))
	s.Error(s.repo.SetForeignChunkWithTTL(user, "k", 0, nil))
	s.Error(s.repo.SetForeignChunkWithTTL(user, "k", -1, []byte{1}))
}

func (s *VideoRepoTestSuite) TestNilVideoRepoNeverPanics() {
	var repo *VideoRepo

	_, err := repo.SetVideo("u", "data:video/mp4;base64,AAAA")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetVideo("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetManifest("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetChunk("u", "k", 0)
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetCopy("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignVideoWithTTL("u", "k", "data:video/mp4;base64,AAAA"), ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignManifestWithTTL("u", "k", domain.VideoManifest{Hashes: [][]byte{{1}}}), ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignChunkWithTTL("u", "k", 0, []byte{1}), ErrMediaRepoNotInit)
}

func (s *VideoRepoTestSuite) TestImageAndVideoNamespacesDoNotCollide() {
	user := uuid.New().String()
	images := NewImageRepo(s.db)
	_, video := randomVideo(s, 64)

	vidKey, err := s.repo.SetVideo(user, video)
	s.Require().NoError(err)
	s.Require().NoError(images.SetForeignImageWithTTL(user, string(vidKey), "data:image/png;base64,AAAA"))

	got, err := s.repo.GetVideo(user, string(vidKey))
	s.Require().NoError(err)
	s.Equal(video, got, "each namespace holds its own file")

	_, err = s.repo.GetVideo(user, "missing")
	s.ErrorIs(err, ErrMediaNotFound)
}

func (s *VideoRepoTestSuite) TestSponsoredVideoReadsCopiesTheImageRepoStored() {
	user := uuid.New().String()
	sold := domain.MediaCopy{OriginalKey: "original-video", BuyerId: "buyer", EncryptedOrder: []byte("sealed")}

	s.Require().NoError(NewSponsoredImageRepo(s.db).SetCopy(user, "copy-key", sold))

	got, err := NewSponsoredVideoRepo(s.db).GetCopy(user, "copy-key")
	s.Require().NoError(err)
	s.Equal(sold, got)

	_, err = NewVideoRepo(s.db).GetCopy(user, "copy-key")
	s.ErrorIs(err, ErrMediaNotFound, "copies live under the sponsored prefix only")
}

func (s *VideoRepoTestSuite) TestInMemoryStoreRefusesWholeChunks() {
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	s.Require().NoError(db.Run("test", "test"))
	defer db.Close()

	_, video := randomVideo(s, 1<<20)
	_, err = NewVideoRepo(db).SetVideo(uuid.New().String(), video)
	s.ErrorIs(err, local_store.ErrValueTooLarge, "an in-memory Badger panics on a value of exactly its threshold")

	_, small := randomVideo(s, 1024)
	_, err = NewVideoRepo(db).SetVideo(uuid.New().String(), small)
	s.NoError(err)
}
