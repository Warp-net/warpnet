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

func (tb *playTestbed) play(t *testing.T, userId, key string, offset int64) (domain.VideoManifest, []byte, error) {
	t.Helper()
	h := StreamPlayVideoHandler(tb.streamer, tb.videos, tb.users)
	return serveVideoRange(t, h, event.GetVideoRangeEvent{UserId: userId, Key: key, Offset: offset}, tb.streamer.self)
}

func TestPlayVideo_OwnVideoStreamsFromLocalChunks(t *testing.T) {
	tb := newPlayTestbed(t, 2*media_meta.VideoChunkSize+300)
	video := tb.author.video
	key, err := tb.videos.SetVideo(ownerID, domain.Base64Video("data:video/mp4;base64,"+base64.StdEncoding.EncodeToString(video)))
	require.NoError(t, err)

	manifest, body, err := tb.play(t, ownerID, string(key), 0)
	require.NoError(t, err)
	assert.Equal(t, int64(len(video)), manifest.Size)
	assert.Equal(t, "video/mp4", manifest.Mime)
	assert.True(t, bytes.Equal(video, body), "the whole file comes in one stream")

	_, body, err = tb.play(t, ownerID, string(key), media_meta.VideoChunkSize+10)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(video[media_meta.VideoChunkSize:], body), "the stream starts at the chunk holding the offset")

	assert.Zero(t, tb.streamer.openCount(), "own videos never leave the node")
}

func TestPlayVideo_ForeignVideoComesInOneStreamAndStaysCached(t *testing.T) {
	tb := newPlayTestbed(t, 3*media_meta.VideoChunkSize+300)
	video, key := tb.author.video, tb.author.key

	manifest, body, err := tb.play(t, videoAuthorID, key, 0)
	require.NoError(t, err)
	assert.Equal(t, key, manifest.Key())
	assert.True(t, bytes.Equal(video, body))
	assert.Equal(t, 1, tb.streamer.openCount(), "one P2P stream carries the whole video")

	for idx := range manifest.Hashes {
		_, err := tb.videos.GetChunk(videoAuthorID, key, idx)
		assert.NoError(t, err, "chunk %d is cached", idx)
	}

	_, body, err = tb.play(t, videoAuthorID, key, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(video, body))
	assert.Equal(t, 1, tb.streamer.openCount(), "a cached video is streamed without the author")
}

func TestPlayVideo_SeekStreamsFromTheChunk(t *testing.T) {
	tb := newPlayTestbed(t, 3*media_meta.VideoChunkSize)
	video, key := tb.author.video, tb.author.key

	_, body, err := tb.play(t, videoAuthorID, key, 2*media_meta.VideoChunkSize+7)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(video[2*media_meta.VideoChunkSize:], body))
	assert.Equal(t, 1, tb.streamer.openCount())

	_, err = tb.videos.GetChunk(videoAuthorID, key, 1)
	assert.ErrorIs(t, err, database.ErrMediaNotFound, "chunks nobody watched stay with the author")
}

func TestPlayVideo_ForgedChunkStopsTheStream(t *testing.T) {
	tb := newPlayTestbed(t, 2*media_meta.VideoChunkSize)
	tb.author.forged = map[int]bool{1: true}

	_, body, err := tb.play(t, videoAuthorID, tb.author.key, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(tb.author.video[:media_meta.VideoChunkSize], body), "only the verified chunk reaches the player")

	_, err = tb.videos.GetChunk(videoAuthorID, tb.author.key, 0)
	assert.NoError(t, err)
	_, err = tb.videos.GetChunk(videoAuthorID, tb.author.key, 1)
	assert.ErrorIs(t, err, database.ErrMediaNotFound)
}

func TestPlayVideo_ManifestOfAnotherSignerIsRefused(t *testing.T) {
	tb := newPlayTestbed(t, media_meta.VideoChunkSize)
	tb.users = mediaUserDouble{users: map[string]domain.User{videoAuthorID: {Id: videoAuthorID, NodeId: remoteNodeID}}}

	_, _, err := tb.play(t, videoAuthorID, tb.author.key, 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)

	_, err = tb.videos.GetManifest(videoAuthorID, tb.author.key)
	assert.ErrorIs(t, err, database.ErrMediaNotFound, "nothing unverified is cached")
}

func TestPlayVideo_OlderAuthorNodeFallsBackToTheWholeFile(t *testing.T) {
	tb := newPlayTestbed(t, 10)
	file, key := videoWithMetadata(t, videoAuthorID)
	raw := rawOf(t, file)
	tb.streamer.openErr = fmt.Errorf("stream: new: %w", warpnet.ErrProtocolNotSupported)
	tb.streamer.file = mustJSON(t, event.GetVideoResponse{File: file})

	_, body, err := tb.play(t, videoAuthorID, key, 0)
	require.NoError(t, err)
	assert.Equal(t, raw, body)

	_, body, err = tb.play(t, videoAuthorID, key, 4)
	require.NoError(t, err)
	assert.Equal(t, raw, body)
	assert.Equal(t, 1, tb.streamer.files, "the whole file is fetched once and streamed from chunks after")
}

func TestPlayVideo_OlderAuthorNodeFileMustMatchTheKey(t *testing.T) {
	tb := newPlayTestbed(t, 10)
	tb.streamer.openErr = warpnet.ErrProtocolNotSupported
	tb.streamer.file = mustJSON(t, event.GetVideoResponse{File: "data:video/mp4;base64,AAAA"})

	_, _, err := tb.play(t, videoAuthorID, tb.author.key, 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)
}

func TestPlayVideo_UnavailableVideos(t *testing.T) {
	tb := newPlayTestbed(t, 100)

	_, _, err := tb.play(t, ownerID, "missing", 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)
	_, _, err = tb.play(t, "stranger", tb.author.key, 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)

	tb.streamer.openErr = warpnet.ErrNodeIsOffline
	_, _, err = tb.play(t, videoAuthorID, tb.author.key, 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)

	_, err = StreamPlayVideoHandler(tb.streamer, tb.videos, tb.users)(mustJSON(t, event.GetVideoRangeEvent{UserId: ownerID}), nil)
	assert.ErrorIs(t, err, ErrEmptyVideoKey)
}

func TestPlayChatVideo_UsesTheChatRoutes(t *testing.T) {
	tb := newPlayTestbed(t, 100)
	h := StreamPlayChatVideoHandler(tb.streamer, tb.chatVideos, chatUserDouble{byId: map[string]domain.User{videoAuthorID: ownerOf(videoAuthorID)}})

	_, body, err := serveVideoRange(t, h, event.GetVideoRangeEvent{UserId: videoAuthorID, Key: tb.author.key}, tb.streamer.self)
	require.NoError(t, err)
	assert.Equal(t, tb.author.video, body)
	assert.Equal(t, []stream.WarpRoute{event.PUBLIC_GET_CHAT_VIDEO_RANGE}, tb.streamer.opened)

	_, err = tb.chatVideos.GetManifest(videoAuthorID, tb.author.key)
	assert.NoError(t, err)
	_, err = tb.videos.GetManifest(videoAuthorID, tb.author.key)
	assert.ErrorIs(t, err, database.ErrMediaNotFound, "chat videos are kept apart from the timeline")
}
