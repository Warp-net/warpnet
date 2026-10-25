//nolint:all
package database

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	manifest, err := s.repo.getManifest(alice, string(key))
	s.Require().NoError(err)
	s.Equal(int64(len(raw)), manifest.Size)
	s.Equal("video/mp4", manifest.Mime)
	s.Equal(int64(1<<20), manifest.ChunkSize)
	s.Len(manifest.Hashes, 3)

	var joined []byte
	for idx := range manifest.Hashes {
		chunk, err := s.repo.getChunk(alice, string(key), idx)
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

	manifest, err := s.repo.getManifest(user, "remote-clip")
	s.Require().NoError(err)
	s.Len(manifest.Hashes, 1)
}

func (s *VideoRepoTestSuite) TestPartlyCachedVideoIsNotWhole() {
	user := uuid.New().String()
	raw, _ := randomVideo(s, 2<<20)
	manifest := buildManifest(raw, "video/mp4")
	key := manifest.Key()

	s.Require().NoError(s.repo.setForeignManifest(user, key, manifest))
	s.Require().NoError(s.repo.setForeignChunk(user, key, 0, raw[:1<<20]))

	chunk, err := s.repo.getChunk(user, key, 0)
	s.Require().NoError(err)
	s.True(bytes.Equal(raw[:1<<20], chunk))

	_, err = s.repo.getChunk(user, key, 1)
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
	_, err = s.repo.getChunk(user, "k", -1)
	s.ErrorIs(err, ErrMediaNotFound)

	s.Error(s.repo.SetForeignVideoWithTTL(user, "", "data:video/mp4;base64,AAAA"))
	s.Error(s.repo.SetForeignVideoWithTTL(user, "k", ""))
	s.Error(s.repo.SetForeignVideoWithTTL("", "k", "data:video/mp4;base64,AAAA"))
	s.Error(s.repo.setForeignManifest(user, "k", domain.VideoManifest{}))
	s.Error(s.repo.setForeignChunk(user, "k", 0, nil))
	s.Error(s.repo.setForeignChunk(user, "k", -1, []byte{1}))
}

func (s *VideoRepoTestSuite) TestNilVideoRepoNeverPanics() {
	var repo *VideoRepo

	_, err := repo.SetVideo("u", "data:video/mp4;base64,AAAA")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetVideo("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetVideoStream("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetForeignVideoStream(nil, domain.User{Id: "u"}, "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	_, err = repo.GetCopy("u", "k")
	s.ErrorIs(err, ErrMediaRepoNotInit)
	s.ErrorIs(repo.SetForeignVideoWithTTL("u", "k", "data:video/mp4;base64,AAAA"), ErrMediaRepoNotInit)
}

func (s *VideoRepoTestSuite) TestVideoStreamReadsFromTheOffset() {
	user := uuid.New().String()
	raw, file := randomVideo(s, 2<<20+300)
	key, err := s.repo.SetVideo(user, file)
	s.Require().NoError(err)

	video, err := s.repo.GetVideoStream(user, string(key))
	s.Require().NoError(err)
	defer video.Close()

	manifest, err := video.Manifest()
	s.Require().NoError(err)
	s.Equal(int64(len(raw)), manifest.Size)

	read := func(offset int64, whence int) []byte {
		_, err := video.Seek(offset, whence)
		s.Require().NoError(err)
		body, err := io.ReadAll(video)
		s.Require().NoError(err)
		return body
	}

	s.True(bytes.Equal(raw, read(0, io.SeekStart)))
	s.True(bytes.Equal(raw[1<<20+5:], read(1<<20+5, io.SeekStart)), "a stream starts at any byte")
	s.True(bytes.Equal(raw[len(raw)-10:], read(-10, io.SeekEnd)))
	s.Empty(read(int64(len(raw))+1, io.SeekStart))

	_, err = video.Seek(-1, io.SeekStart)
	s.ErrorIs(err, ErrInvalidSeek)

	_, err = s.repo.GetVideoStream(user, "missing")
	s.ErrorIs(err, ErrMediaNotFound)
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

const videoAuthorId = "author"

var videoAuthorKey, videoAuthorNodeId = mustVideoAuthorKey()

func mustVideoAuthorKey() (ed25519.PrivateKey, warpnet.WarpPeerID) {
	priv, err := security.GenerateKeyFromSeed([]byte("video-repo-test-seed"))
	if err != nil {
		panic(err)
	}
	id, err := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		panic(err)
	}
	return priv, id
}

func newDiskVideoRepo(t *testing.T) *VideoRepo {
	t.Helper()
	db, err := local_store.New(t.TempDir(), local_store.DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("test", "test"))
	t.Cleanup(db.Close)
	return NewVideoRepo(db)
}

type authorNode struct {
	repo  *VideoRepo
	user  domain.User
	video []byte
	key   string
	err   error
	file  domain.Base64Video

	mx           sync.Mutex
	chunkFetches []int64
	videoFetches int
}

func newAuthorNode(t *testing.T, size int) *authorNode {
	t.Helper()
	video := make([]byte, size)
	for i := range video {
		video[i] = byte(i % 251)
	}

	a := &authorNode{
		repo:  newDiskVideoRepo(t),
		user:  domain.User{Id: videoAuthorId, NodeId: videoAuthorNodeId.String()},
		video: video,
	}
	key, err := a.repo.SetVideo(videoAuthorId, domain.Base64Video("data:video/mp4;base64,"+base64.StdEncoding.EncodeToString(video)))
	require.NoError(t, err)
	a.key = string(key)
	return a
}

func (a *authorNode) FetchVideoChunks(_ domain.User, key string, offset int64) (warpnet.WarpStream, error) {
	a.mx.Lock()
	a.chunkFetches = append(a.chunkFetches, offset)
	a.mx.Unlock()
	if a.err != nil {
		return nil, a.err
	}

	video, err := a.repo.GetVideoStream(videoAuthorId, key)
	if err != nil {
		return nil, err
	}
	manifest, err := video.Manifest()
	if err != nil {
		return nil, err
	}
	if _, err := video.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	client, server := stream.NewLoopbackStream("", "", "/public/get/video/1.0.0")
	go func() {
		defer func() { _ = video.Close() }()
		line, _ := json.Marshal(manifest)
		if _, err := server.Write(append(line, '\n')); err == nil {
			_, _ = io.Copy(server, video)
		}
		_ = server.Close()
	}()
	return client, nil
}

func (a *authorNode) FetchVideo(domain.User, string) (domain.Base64Video, error) {
	a.mx.Lock()
	defer a.mx.Unlock()
	a.videoFetches++
	return a.file, nil
}

func (a *authorNode) forgeChunk(t *testing.T, idx int) {
	t.Helper()
	chunk, err := a.repo.getChunk(videoAuthorId, a.key, idx)
	require.NoError(t, err)
	forged := bytes.Clone(chunk)
	forged[0] ^= 0xFF
	require.NoError(t, a.repo.db.SetWithTTL(a.repo.buildChunkKey(videoAuthorId, a.key, idx), forged, local_store.PermanentTTL))
}

func playForeignVideo(viewer *VideoRepo, a *authorNode, offset int64) ([]byte, error) {
	video, err := viewer.GetForeignVideoStream(a, a.user, a.key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = video.Close() }()

	if _, err := video.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := video.Manifest(); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	_, err = io.Copy(&body, video)
	return body.Bytes(), err
}

func signedVideoFile(t *testing.T) domain.Base64Video {
	t.Helper()
	raw := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0x00, 0x00, 0x02, 0x00, 'i', 's', 'o', 'm', 'm', 'p', '4', '1'}
	signer := media_meta.Metadata{
		PrivKey:       videoAuthorKey,
		NodeId:        videoAuthorNodeId.String(),
		OwnerId:       videoAuthorId,
		EncryptedMeta: []byte("encrypted forensic payload"),
	}
	metadata, err := signer.Sign(security.ConvertToSHA256(raw))
	require.NoError(t, err)
	signed, err := media_meta.EmbedInVideo(raw, metadata)
	require.NoError(t, err)
	return domain.Base64Video("data:video/mp4;base64," + base64.StdEncoding.EncodeToString(signed))
}

func TestForeignVideoStream_OneStreamFromTheAuthorAndThenTheCache(t *testing.T) {
	a := newAuthorNode(t, 3*videoChunkSize+300)
	viewer := newDiskVideoRepo(t)

	body, err := playForeignVideo(viewer, a, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video, body))
	assert.Equal(t, []int64{0}, a.chunkFetches, "one stream from the author carries the whole video")

	body, err = playForeignVideo(viewer, a, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video, body))
	assert.Len(t, a.chunkFetches, 1, "a cached video plays without the author")
}

func TestForeignVideoStream_MissingChunksComeFromTheFirstOneMissing(t *testing.T) {
	a := newAuthorNode(t, 3*videoChunkSize)
	viewer := newDiskVideoRepo(t)
	seek := int64(2*videoChunkSize + 7)

	body, err := playForeignVideo(viewer, a, seek)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video[seek:], body))
	_, err = viewer.getChunk(videoAuthorId, a.key, 1)
	assert.ErrorIs(t, err, ErrMediaNotFound, "chunks nobody watched stay with the author")

	body, err = playForeignVideo(viewer, a, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video, body))
	assert.Equal(t, []int64{2 * videoChunkSize, 0}, a.chunkFetches, "the author is asked from a chunk boundary")
}

func TestForeignVideoStream_ForgedChunkStopsTheVideo(t *testing.T) {
	a := newAuthorNode(t, 2*videoChunkSize)
	a.forgeChunk(t, 1)
	viewer := newDiskVideoRepo(t)

	body, err := playForeignVideo(viewer, a, 0)
	assert.ErrorIs(t, err, ErrForgedVideo)
	assert.True(t, bytes.Equal(a.video[:videoChunkSize], body), "only the verified chunk is read")

	_, err = viewer.getChunk(videoAuthorId, a.key, 0)
	assert.NoError(t, err)
	_, err = viewer.getChunk(videoAuthorId, a.key, 1)
	assert.ErrorIs(t, err, ErrMediaNotFound)
}

func TestForeignVideoStream_RefusesAManifestForAnotherKey(t *testing.T) {
	a := newAuthorNode(t, videoChunkSize)
	other := newAuthorNode(t, videoChunkSize+5)
	a.key = other.key
	viewer := newDiskVideoRepo(t)

	_, err := playForeignVideo(viewer, a, 0)
	assert.Error(t, err)

	_, err = viewer.getManifest(videoAuthorId, a.key)
	assert.ErrorIs(t, err, ErrMediaNotFound, "nothing unverified is cached")
}

func TestForeignVideoStream_OlderAuthorNodeSendsTheWholeFile(t *testing.T) {
	a := newAuthorNode(t, 10)
	a.file = signedVideoFile(t)
	a.key = media_meta.BuildContentKey(string(a.file))
	a.err = fmt.Errorf("stream: new: %w", warpnet.ErrProtocolNotSupported)
	viewer := newDiskVideoRepo(t)
	_, raw, err := media_meta.SplitDataURI(string(a.file))
	require.NoError(t, err)

	body, err := playForeignVideo(viewer, a, 0)
	require.NoError(t, err)
	assert.Equal(t, raw, body)

	body, err = playForeignVideo(viewer, a, 4)
	require.NoError(t, err)
	assert.Equal(t, raw[4:], body)
	assert.Equal(t, 1, a.videoFetches, "the whole file is fetched once and read from chunks after")
}

func TestForeignVideoStream_WholeFileMustMatchTheKey(t *testing.T) {
	a := newAuthorNode(t, 10)
	a.file = "data:video/mp4;base64,AAAA"
	a.err = warpnet.ErrProtocolNotSupported

	_, err := playForeignVideo(newDiskVideoRepo(t), a, 0)
	assert.ErrorIs(t, err, media_meta.ErrMediaKeyMismatch)
}

func TestForeignVideoStream_OfflineAuthor(t *testing.T) {
	a := newAuthorNode(t, 100)
	a.err = warpnet.ErrNodeIsOffline

	_, err := playForeignVideo(newDiskVideoRepo(t), a, 0)
	assert.ErrorIs(t, err, warpnet.ErrNodeIsOffline)
}
