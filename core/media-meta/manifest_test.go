// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package media_meta

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chunkRepo map[int][]byte

func (r chunkRepo) GetChunk(_, _ string, idx int) ([]byte, error) {
	chunk, ok := r[idx]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return chunk, nil
}

func randomVideo(t *testing.T, size int) []byte {
	t.Helper()
	video := make([]byte, size)
	_, err := rand.Read(video)
	require.NoError(t, err)
	return video
}

func chunksOf(video []byte) chunkRepo {
	chunks := chunkRepo{}
	for idx := 0; idx*VideoChunkSize < len(video); idx++ {
		chunks[idx] = video[idx*VideoChunkSize : min((idx+1)*VideoChunkSize, len(video))]
	}
	return chunks
}

func signedManifest(t *testing.T, video []byte, ownerId string) (domain.VideoManifest, string) {
	t.Helper()
	manifest := BuildVideoManifest(video, "video/mp4")
	key := manifest.Key()
	signed, err := metadata(ownerId).SignManifest(key, manifest)
	require.NoError(t, err)
	return signed, key
}

func TestBuildVideoManifest_KeyIsTheHashOfChunkHashes(t *testing.T) {
	video := randomVideo(t, 2*VideoChunkSize+7)
	manifest := BuildVideoManifest(video, "video/mp4")

	require.Len(t, manifest.Hashes, 3)
	assert.Equal(t, int64(len(video)), manifest.Size)
	assert.Equal(t, int64(VideoChunkSize), manifest.ChunkLen(0))
	assert.Equal(t, int64(7), manifest.ChunkLen(2))

	all := sha256.New()
	for idx, chunk := range chunksOf(video) {
		sum := sha256.Sum256(chunk)
		assert.Equal(t, sum[:], manifest.Hashes[idx])
	}
	for _, chunkHash := range manifest.Hashes {
		all.Write(chunkHash)
	}
	assert.Equal(t, hex.EncodeToString(all.Sum(nil)), manifest.Key())
	assert.Equal(t, manifest.Key(), BuildVideoManifest(video, "video/quicktime").Key(), "the key covers bytes only")
}

func TestVerifyManifest_HoldsOnlyForTheKeyAndAuthorItWasSignedFor(t *testing.T) {
	video := randomVideo(t, VideoChunkSize+1)
	manifest, key := signedManifest(t, video, "alice")
	alice := domain.User{Id: "alice", NodeId: signerID.String()}

	require.NoError(t, VerifyManifest(alice, key, manifest))

	t.Run("another key", func(t *testing.T) {
		other := BuildVideoManifest(randomVideo(t, 10), "video/mp4").Key()
		assert.ErrorIs(t, VerifyManifest(alice, other, manifest), ErrForgedManifest)
	})

	t.Run("another user on the same node", func(t *testing.T) {
		mallory := domain.User{Id: "mallory", NodeId: signerID.String()}
		assert.ErrorIs(t, VerifyManifest(mallory, key, manifest), ErrForgedManifest)
	})

	t.Run("another node", func(t *testing.T) {
		_, otherID := mustSigner("another-node-seed")
		assert.ErrorIs(t, VerifyManifest(domain.User{Id: "alice", NodeId: otherID.String()}, key, manifest), ErrForgedManifest)
	})

	t.Run("signed by a node it names falsely", func(t *testing.T) {
		otherKey, _ := mustSigner("another-node-seed")
		forger := Metadata{PrivKey: otherKey, NodeId: signerID.String(), OwnerId: "alice"}
		forged, err := forger.SignManifest(key, BuildVideoManifest(video, "video/mp4"))
		require.NoError(t, err)
		assert.ErrorIs(t, VerifyManifest(alice, key, forged), ErrForgedManifest)
	})

	t.Run("chunk hashes in another order", func(t *testing.T) {
		swapped := manifest
		swapped.Hashes = [][]byte{manifest.Hashes[1], manifest.Hashes[0]}
		assert.ErrorIs(t, VerifyManifest(alice, key, swapped), ErrForgedManifest)
	})

	t.Run("not a video", func(t *testing.T) {
		page := manifest
		page.Mime = "text/html"
		assert.ErrorIs(t, VerifyManifest(alice, key, page), ErrForgedManifest)
	})

	t.Run("a size the chunks do not cover", func(t *testing.T) {
		grown := manifest
		grown.Size = 3 * VideoChunkSize
		assert.ErrorIs(t, VerifyManifest(alice, key, grown), ErrForgedManifest)
	})
}

func TestVerifyChunk_RefusesAForgedChunk(t *testing.T) {
	video := randomVideo(t, VideoChunkSize+10)
	manifest := BuildVideoManifest(video, "video/mp4")
	chunks := chunksOf(video)

	assert.NoError(t, VerifyChunk(manifest, 1, chunks[1]))

	forged := bytes.Clone(chunks[1])
	forged[0] ^= 0xFF
	assert.ErrorIs(t, VerifyChunk(manifest, 1, forged), ErrForgedChunk)
	assert.ErrorIs(t, VerifyChunk(manifest, 0, chunks[1]), ErrForgedChunk, "a chunk is bound to its place")
	assert.ErrorIs(t, VerifyChunk(manifest, 2, chunks[1]), ErrForgedChunk)
	assert.ErrorIs(t, VerifyChunk(manifest, -1, chunks[1]), ErrForgedChunk)
}

func readRange(t *testing.T, video []byte, offset, length int64) (domain.VideoManifest, []byte) {
	t.Helper()

	manifest := BuildVideoManifest(video, "video/mp4")
	client, server := stream.NewLoopbackStream("", "", "/test/route/0.0.0")
	go func() {
		assert.NoError(t, WriteVideoRange(server, chunksOf(video), "alice", manifest.Key(), manifest, offset, length))
		_ = server.Close()
	}()

	r := bufio.NewReaderSize(client, MaxManifestSize)
	header, err := ReadVideoManifest(r)
	require.NoError(t, err)
	body, err := io.ReadAll(r)
	require.NoError(t, err)
	return header, body
}

func TestWriteVideoRange_SendsWholeChunksThatCoverTheRange(t *testing.T) {
	video := randomVideo(t, 2*VideoChunkSize+VideoChunkSize/2)

	t.Run("from a chunk boundary to the end", func(t *testing.T) {
		header, body := readRange(t, video, 0, 0)
		assert.Equal(t, BuildVideoManifest(video, "video/mp4"), header)
		assert.True(t, bytes.Equal(video, body))
	})

	t.Run("from the middle of a chunk", func(t *testing.T) {
		_, body := readRange(t, video, VideoChunkSize+VideoChunkSize/2, 0)
		assert.True(t, bytes.Equal(video[VideoChunkSize:], body), "the chunk holding the offset comes whole")
	})

	t.Run("across a chunk boundary", func(t *testing.T) {
		_, body := readRange(t, video, VideoChunkSize-10, 20)
		assert.True(t, bytes.Equal(video[:2*VideoChunkSize], body))
	})

	t.Run("past the end", func(t *testing.T) {
		_, body := readRange(t, video, int64(len(video))+1, 0)
		assert.Empty(t, body)
	})
}

func TestReadVideoManifest_AnErrorAnswerIsAnUnavailableVideo(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte(`{"code":5000,"message":"boom"}`)))
	_, err := ReadVideoManifest(r)
	assert.ErrorIs(t, err, ErrVideoUnavailable)

	_, err = ReadVideoManifest(bufio.NewReader(bytes.NewReader(nil)))
	assert.ErrorIs(t, err, io.EOF)

	_, err = ReadVideoManifest(bufio.NewReaderSize(bytes.NewReader(bytes.Repeat([]byte("a"), 64)), 16))
	assert.ErrorIs(t, err, bufio.ErrBufferFull, "a header must fit the reader's buffer")
}

func TestVerifyForeignVideo_AcceptsTheChunkedKey(t *testing.T) {
	signed := mp4WithMetadata(t, "alice")
	file := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(signed)
	alice := domain.User{Id: "alice", NodeId: signerID.String()}

	assert.NoError(t, VerifyForeignVideo(alice, BuildVideoManifest(signed, "video/mp4").Key(), file))
	assert.NoError(t, VerifyForeignVideo(alice, BuildContentKey(file), file), "videos posted before chunking keep their keys")
	assert.ErrorIs(t, VerifyForeignVideo(alice, BuildVideoManifest(randomVideo(t, 10), "video/mp4").Key(), file),
		ErrMediaKeyMismatch)
}
