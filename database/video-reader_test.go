//nolint:all
package database

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const videoAuthorId = "author"

var videoAuthorKey, videoAuthorNodeId = mustVideoAuthorKey()

func mustVideoAuthorKey() (ed25519.PrivateKey, warpnet.WarpPeerID) {
	priv, err := security.GenerateKeyFromSeed([]byte("video-reader-test-seed"))
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

	manifest, err := a.repo.GetManifest(videoAuthorId, key)
	if err != nil {
		return nil, err
	}
	signer := media_meta.Metadata{PrivKey: videoAuthorKey, NodeId: videoAuthorNodeId.String(), OwnerId: videoAuthorId}
	manifest, err = signer.SignManifest(key, manifest)
	if err != nil {
		return nil, err
	}

	client, server := stream.NewLoopbackStream("", "", "/public/get/video/1.0.0")
	go func() {
		_ = a.repo.WriteVideo(server, videoAuthorId, key, manifest, offset)
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

func playForeignVideo(t *testing.T, viewer *VideoRepo, a *authorNode, offset int64) (domain.VideoManifest, []byte, error) {
	t.Helper()

	video, err := viewer.OpenForeignVideo(a, a.user, a.key, offset)
	if err != nil {
		return domain.VideoManifest{}, nil, err
	}
	defer video.Close()

	client, server := stream.NewLoopbackStream("", "", "/private/get/video/0.0.0")
	go func() {
		_ = video.WriteVideo(server)
		_ = server.Close()
	}()

	r := bufio.NewReaderSize(client, media_meta.MaxManifestSize)
	manifest, err := media_meta.ReadVideoManifest(r)
	require.NoError(t, err)
	body, _ := io.ReadAll(r)
	return manifest, body, nil
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

func TestOpenForeignVideo_OneStreamFromTheAuthorAndThenTheCache(t *testing.T) {
	a := newAuthorNode(t, 3*media_meta.VideoChunkSize+300)
	viewer := newDiskVideoRepo(t)

	manifest, body, err := playForeignVideo(t, viewer, a, 0)
	require.NoError(t, err)
	assert.Equal(t, a.key, manifest.Key())
	assert.True(t, bytes.Equal(a.video, body))
	assert.Equal(t, []int64{0}, a.chunkFetches, "one stream from the author carries the whole video")

	_, body, err = playForeignVideo(t, viewer, a, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video, body))
	assert.Len(t, a.chunkFetches, 1, "a cached video plays without the author")
}

func TestOpenForeignVideo_MissingChunksComeFromTheFirstOneMissing(t *testing.T) {
	a := newAuthorNode(t, 3*media_meta.VideoChunkSize)
	viewer := newDiskVideoRepo(t)
	seek := int64(2*media_meta.VideoChunkSize + 7)

	_, body, err := playForeignVideo(t, viewer, a, seek)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video[2*media_meta.VideoChunkSize:], body), "the chunk holding the offset comes whole")
	_, err = viewer.getChunk(videoAuthorId, a.key, 1)
	assert.ErrorIs(t, err, ErrMediaNotFound, "chunks nobody watched stay with the author")

	_, body, err = playForeignVideo(t, viewer, a, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video, body))
	assert.Equal(t, []int64{seek, 0}, a.chunkFetches)
}

func TestOpenForeignVideo_ForgedChunkStopsTheVideo(t *testing.T) {
	a := newAuthorNode(t, 2*media_meta.VideoChunkSize)
	a.forgeChunk(t, 1)
	viewer := newDiskVideoRepo(t)

	_, body, err := playForeignVideo(t, viewer, a, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a.video[:media_meta.VideoChunkSize], body), "only the verified chunk reaches the player")

	_, err = viewer.getChunk(videoAuthorId, a.key, 0)
	assert.NoError(t, err)
	_, err = viewer.getChunk(videoAuthorId, a.key, 1)
	assert.ErrorIs(t, err, ErrMediaNotFound)
}

func TestOpenForeignVideo_RefusesTheManifestOfAnotherSigner(t *testing.T) {
	a := newAuthorNode(t, media_meta.VideoChunkSize)
	a.user.NodeId = "12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU"
	viewer := newDiskVideoRepo(t)

	_, _, err := playForeignVideo(t, viewer, a, 0)
	assert.ErrorIs(t, err, media_meta.ErrForgedManifest)

	_, err = viewer.GetManifest(videoAuthorId, a.key)
	assert.ErrorIs(t, err, ErrMediaNotFound, "nothing unverified is cached")
}

func TestOpenForeignVideo_OlderAuthorNodeSendsTheWholeFile(t *testing.T) {
	a := newAuthorNode(t, 10)
	a.file = signedVideoFile(t)
	a.key = media_meta.BuildContentKey(string(a.file))
	a.err = fmt.Errorf("stream: new: %w", warpnet.ErrProtocolNotSupported)
	viewer := newDiskVideoRepo(t)
	_, raw, err := media_meta.SplitDataURI(string(a.file))
	require.NoError(t, err)

	_, body, err := playForeignVideo(t, viewer, a, 0)
	require.NoError(t, err)
	assert.Equal(t, raw, body)

	_, body, err = playForeignVideo(t, viewer, a, 4)
	require.NoError(t, err)
	assert.Equal(t, raw, body)
	assert.Equal(t, 1, a.videoFetches, "the whole file is fetched once and played from chunks after")
}

func TestOpenForeignVideo_WholeFileMustMatchTheKey(t *testing.T) {
	a := newAuthorNode(t, 10)
	a.file = "data:video/mp4;base64,AAAA"
	a.err = warpnet.ErrProtocolNotSupported

	_, _, err := playForeignVideo(t, newDiskVideoRepo(t), a, 0)
	assert.ErrorIs(t, err, media_meta.ErrMediaKeyMismatch)
}

func TestOpenForeignVideo_OfflineAuthor(t *testing.T) {
	a := newAuthorNode(t, 100)
	a.err = warpnet.ErrNodeIsOffline

	_, _, err := playForeignVideo(t, newDiskVideoRepo(t), a, 0)
	assert.ErrorIs(t, err, warpnet.ErrNodeIsOffline)
}
