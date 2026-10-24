// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package handler

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const videoAuthorID = "author"

type playStreamer struct {
	self, author warpnet.WarpPeerID
	ranges       map[stream.WarpRoute]warpnet.WarpHandlerFunc
	openErr      error
	file         []byte

	mx     sync.Mutex
	opened []stream.WarpRoute
	files  int
}

func (p *playStreamer) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: p.self, OwnerId: ownerID}
}

func (p *playStreamer) OpenStream(_ string, path stream.WarpRoute, data any) (warpnet.WarpStream, error) {
	p.mx.Lock()
	p.opened = append(p.opened, path)
	p.mx.Unlock()
	if p.openErr != nil {
		return nil, p.openErr
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	client, server := stream.NewLoopbackStream(p.author, p.self, warpnet.WarpProtocolID(path))
	go func() {
		defer func() { _ = server.Close() }()
		out, err := p.ranges[path](body, server)
		if err != nil {
			out = event.ResponseError{Message: err.Error()}
		}
		if bt, ok := out.([]byte); ok && len(bt) == 0 {
			return
		}
		payload, _ := json.Marshal(out)
		_, _ = server.Write(payload)
	}()
	return client, nil
}

func (p *playStreamer) GenericStream(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
	p.mx.Lock()
	defer p.mx.Unlock()
	p.files++
	return p.file, nil
}

func (p *playStreamer) openCount() int {
	p.mx.Lock()
	defer p.mx.Unlock()
	return len(p.opened)
}

type playTestbed struct {
	streamer   *playStreamer
	author     *videoChunksDouble
	users      mediaUserDouble
	videos     *database.VideoRepo
	chatVideos *database.VideoRepo
}

func newPlayTestbed(t *testing.T, size int) *playTestbed {
	t.Helper()

	db, err := local_store.New(t.TempDir(), local_store.DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("test", "test"))
	t.Cleanup(db.Close)

	author := newVideoChunksDouble(videoAuthorID, size)
	viewerNode := warpnet.FromStringToPeerID(selfNodeID)
	partners := chatFetcherDouble{partners: map[string]bool{videoAuthorID + "/" + ownerID: true}}
	authorUsers := chatUserDouble{byNode: map[string]domain.User{viewerNode.String(): {Id: ownerID, NodeId: selfNodeID}}}
	return &playTestbed{
		streamer: &playStreamer{
			self:   viewerNode,
			author: testSignerID,
			ranges: map[stream.WarpRoute]warpnet.WarpHandlerFunc{
				event.PUBLIC_GET_VIDEO_RANGE: StreamGetVideoRangeHandler(signingInformer{videoAuthorID}, testSignerKey, &author),
				event.PUBLIC_GET_CHAT_VIDEO_RANGE: StreamGetChatVideoRangeHandler(
					signingInformer{videoAuthorID}, testSignerKey, &author, partners, authorUsers,
				),
			},
		},
		author:     &author,
		users:      mediaUserDouble{users: map[string]domain.User{videoAuthorID: ownerOf(videoAuthorID)}},
		videos:     database.NewVideoRepo(db),
		chatVideos: database.NewChatVideoRepo(db),
	}
}

func (tb *playTestbed) play(t *testing.T, userId, key string, offset, length int64) event.GetVideoRangeResponse {
	t.Helper()
	h := StreamPlayVideoHandler(tb.streamer, tb.videos, tb.users)
	out, err := h(mustJSON(t, event.GetVideoRangeEvent{UserId: userId, Key: key, Offset: offset, Length: length}), nil)
	require.NoError(t, err)
	resp, ok := out.(event.GetVideoRangeResponse)
	require.True(t, ok)
	return resp
}

func TestPlayVideo_OwnVideoComesFromLocalChunks(t *testing.T) {
	tb := newPlayTestbed(t, 2*media_meta.VideoChunkSize+300)
	video := tb.author.video
	key, err := tb.videos.SetVideo(ownerID, domain.Base64Video("data:video/mp4;base64,"+base64.StdEncoding.EncodeToString(video)))
	require.NoError(t, err)

	resp := tb.play(t, ownerID, string(key), 0, 1024)
	assert.Equal(t, int64(len(video)), resp.Size)
	assert.Equal(t, "video/mp4", resp.Mime)
	assert.Equal(t, video[:1024], resp.Data)

	start := int64(media_meta.VideoChunkSize - 10)
	assert.Equal(t, video[start:start+20], tb.play(t, ownerID, string(key), start, 20).Data, "a range across a chunk boundary")
	assert.Equal(t, video[len(video)-50:], tb.play(t, ownerID, string(key), int64(len(video)-50), 1000).Data, "clipped at the end")

	assert.Zero(t, tb.streamer.openCount(), "own videos never leave the node")
}

func TestPlayVideo_ForeignVideoStreamsFromTheAuthorAndStaysCached(t *testing.T) {
	tb := newPlayTestbed(t, 2*media_meta.VideoChunkSize+300)
	video, key := tb.author.video, tb.author.key
	start := int64(media_meta.VideoChunkSize - 6)

	resp := tb.play(t, videoAuthorID, key, start, 2*media_meta.VideoChunkSize)
	assert.Equal(t, int64(len(video)), resp.Size)
	assert.True(t, bytes.Equal(video[start:], resp.Data), "the range is clipped at the end of the file")
	assert.Equal(t, 1, tb.streamer.openCount(), "the manifest and the chunks come in one stream")

	manifest, err := tb.videos.GetManifest(videoAuthorID, key)
	require.NoError(t, err)
	assert.Equal(t, key, manifest.Key())
	for idx := range manifest.Hashes {
		_, err := tb.videos.GetChunk(videoAuthorID, key, idx)
		assert.NoError(t, err, "chunk %d is cached", idx)
	}

	assert.Equal(t, video[:100], tb.play(t, videoAuthorID, key, 0, 100).Data)
	assert.Equal(t, 1, tb.streamer.openCount(), "a cached video is read without the author")
}

func TestPlayVideo_SeekingOpensAStreamAtTheChunk(t *testing.T) {
	tb := newPlayTestbed(t, 3*media_meta.VideoChunkSize)
	video, key := tb.author.video, tb.author.key

	tb.play(t, videoAuthorID, key, 0, 10)
	start := int64(2*media_meta.VideoChunkSize + 7)
	assert.Equal(t, video[start:start+10], tb.play(t, videoAuthorID, key, start, 10).Data)
	assert.Equal(t, 2, tb.streamer.openCount())

	_, err := tb.videos.GetChunk(videoAuthorID, key, 1)
	assert.ErrorIs(t, err, database.ErrMediaNotFound, "chunks nobody watched stay with the author")
}

func TestPlayVideo_ForgedChunkNeverReachesThePlayerOrTheCache(t *testing.T) {
	tb := newPlayTestbed(t, 2*media_meta.VideoChunkSize)
	tb.author.forged = map[int]bool{1: true}

	resp := tb.play(t, videoAuthorID, tb.author.key, 0, 2*media_meta.VideoChunkSize)
	assert.Empty(t, resp.Data)

	_, err := tb.videos.GetChunk(videoAuthorID, tb.author.key, 0)
	assert.NoError(t, err, "the chunk before the forged one checked out")
	_, err = tb.videos.GetChunk(videoAuthorID, tb.author.key, 1)
	assert.ErrorIs(t, err, database.ErrMediaNotFound)
}

func TestPlayVideo_ManifestOfAnotherSignerIsRefused(t *testing.T) {
	tb := newPlayTestbed(t, media_meta.VideoChunkSize)
	tb.users = mediaUserDouble{users: map[string]domain.User{videoAuthorID: {Id: videoAuthorID, NodeId: remoteNodeID}}}

	assert.Empty(t, tb.play(t, videoAuthorID, tb.author.key, 0, 100).Data)

	_, err := tb.videos.GetManifest(videoAuthorID, tb.author.key)
	assert.ErrorIs(t, err, database.ErrMediaNotFound, "nothing unverified is cached")
}

func TestPlayVideo_OlderAuthorNodeFallsBackToTheWholeFile(t *testing.T) {
	tb := newPlayTestbed(t, 10)
	file, key := videoWithMetadata(t, videoAuthorID)
	raw := rawOf(t, file)
	tb.streamer.openErr = fmt.Errorf("stream: new: %w", warpnet.ErrProtocolNotSupported)
	tb.streamer.file = mustJSON(t, event.GetVideoResponse{File: file})

	resp := tb.play(t, videoAuthorID, key, 0, int64(len(raw)))
	assert.Equal(t, raw, resp.Data)
	assert.Equal(t, raw[4:8], tb.play(t, videoAuthorID, key, 4, 4).Data)
	assert.Equal(t, 1, tb.streamer.files, "the whole file is fetched once and read from chunks after")
}

func TestPlayVideo_OlderAuthorNodeFileMustMatchTheKey(t *testing.T) {
	tb := newPlayTestbed(t, 10)
	tb.streamer.openErr = warpnet.ErrProtocolNotSupported
	tb.streamer.file = mustJSON(t, event.GetVideoResponse{File: "data:video/mp4;base64,AAAA"})

	assert.Empty(t, tb.play(t, videoAuthorID, tb.author.key, 0, 4).Data)
}

func TestPlayVideo_UnavailableVideos(t *testing.T) {
	tb := newPlayTestbed(t, 100)

	assert.Zero(t, tb.play(t, ownerID, "missing", 0, 10).Size)
	assert.Zero(t, tb.play(t, "stranger", tb.author.key, 0, 10).Size)

	tb.streamer.openErr = warpnet.ErrNodeIsOffline
	assert.Zero(t, tb.play(t, videoAuthorID, tb.author.key, 0, 10).Size)

	_, err := StreamPlayVideoHandler(tb.streamer, tb.videos, tb.users)(mustJSON(t, event.GetVideoRangeEvent{UserId: ownerID}), nil)
	assert.ErrorIs(t, err, ErrEmptyVideoKey)
}

func TestPlayVideo_RangeIsCapped(t *testing.T) {
	tb := newPlayTestbed(t, 6*media_meta.VideoChunkSize)
	key, err := tb.videos.SetVideo(ownerID, domain.Base64Video("data:video/mp4;base64,"+base64.StdEncoding.EncodeToString(tb.author.video)))
	require.NoError(t, err)

	assert.Len(t, tb.play(t, ownerID, string(key), 0, 6*media_meta.VideoChunkSize).Data, maxVideoRangeLength)
}

func TestPlayChatVideo_UsesTheChatRoutes(t *testing.T) {
	tb := newPlayTestbed(t, 100)
	h := StreamPlayChatVideoHandler(tb.streamer, tb.chatVideos, chatUserDouble{byId: map[string]domain.User{videoAuthorID: ownerOf(videoAuthorID)}})

	out, err := h(mustJSON(t, event.GetVideoRangeEvent{UserId: videoAuthorID, Key: tb.author.key, Offset: 0, Length: 100}), nil)
	require.NoError(t, err)
	assert.Equal(t, tb.author.video, out.(event.GetVideoRangeResponse).Data)
	assert.Equal(t, []stream.WarpRoute{event.PUBLIC_GET_CHAT_VIDEO_RANGE}, tb.streamer.opened)

	_, err = tb.chatVideos.GetManifest(videoAuthorID, tb.author.key)
	assert.NoError(t, err)
	_, err = tb.videos.GetManifest(videoAuthorID, tb.author.key)
	assert.ErrorIs(t, err, database.ErrMediaNotFound, "chat videos are kept apart from the timeline")
}
