// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package handler

import (
	"bytes"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const videoAuthorID = "author"

type playStreamer struct {
	self, author warpnet.WarpPeerID
	authorRoutes map[stream.WarpRoute]warpnet.WarpHandlerFunc
	openErr      error

	mx     sync.Mutex
	opened []stream.WarpRoute
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
		out, err := p.authorRoutes[path](body, server)
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

func (p *playStreamer) GenericStream(string, stream.WarpRoute, any) ([]byte, error) {
	return nil, warpnet.ErrNodeIsOffline
}

func (p *playStreamer) openedRoutes() []stream.WarpRoute {
	p.mx.Lock()
	defer p.mx.Unlock()
	return append([]stream.WarpRoute(nil), p.opened...)
}

type playTestbed struct {
	streamer   *playStreamer
	users      mediaUserDouble
	videos     *database.VideoRepo
	chatVideos *database.VideoRepo
	key        string
	video      []byte
}

func newPlayTestbed(t *testing.T, size int) *playTestbed {
	t.Helper()

	authorDB := newVideoDB(t)
	authorVideos := database.NewVideoRepo(authorDB)
	authorChatVideos := database.NewChatVideoRepo(authorDB)
	key, video := storeTestVideo(t, authorVideos, videoAuthorID, size)
	storeTestVideo(t, authorChatVideos, videoAuthorID, size)

	viewerNode := warpnet.FromStringToPeerID(selfNodeID)
	partners := chatFetcherDouble{partners: map[string]bool{videoAuthorID + "/" + ownerID: true}}
	viewers := chatUserDouble{byNode: map[string]domain.User{viewerNode.String(): {Id: ownerID, NodeId: selfNodeID}}}

	viewerDB := newVideoDB(t)
	return &playTestbed{
		streamer: &playStreamer{
			self:   viewerNode,
			author: testSignerID,
			authorRoutes: map[stream.WarpRoute]warpnet.WarpHandlerFunc{
				event.PUBLIC_GET_VIDEO_CHUNKS: StreamGetVideoChunksHandler(signingInformer{videoAuthorID}, testSignerKey, authorVideos),
				event.PUBLIC_GET_CHAT_VIDEO_CHUNKS: StreamGetChatVideoChunksHandler(
					signingInformer{videoAuthorID}, testSignerKey, authorChatVideos, partners, viewers,
				),
			},
		},
		users:      mediaUserDouble{users: map[string]domain.User{videoAuthorID: ownerOf(videoAuthorID)}},
		videos:     database.NewVideoRepo(viewerDB),
		chatVideos: database.NewChatVideoRepo(viewerDB),
		key:        key,
		video:      video,
	}
}

func (tb *playTestbed) play(t *testing.T, userId, key string, offset int64) (domain.VideoManifest, []byte, error) {
	t.Helper()
	h := StreamPlayVideoHandler(tb.streamer, tb.videos, tb.users)
	return serveVideo(t, h, event.GetVideoChunksEvent{UserId: userId, Key: key, Offset: offset}, tb.streamer.self)
}

func TestPlayVideo_OwnVideoStreamsFromLocalChunks(t *testing.T) {
	tb := newPlayTestbed(t, 10)
	key, video := storeTestVideo(t, tb.videos, ownerID, 2*media_meta.VideoChunkSize+300)

	manifest, body, err := tb.play(t, ownerID, key, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(len(video)), manifest.Size)
	assert.Equal(t, "video/mp4", manifest.Mime)
	assert.True(t, bytes.Equal(video, body), "the whole file comes in one stream")

	_, body, err = tb.play(t, ownerID, key, media_meta.VideoChunkSize+10)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(video[media_meta.VideoChunkSize:], body), "the stream starts at the chunk holding the offset")

	assert.Empty(t, tb.streamer.openedRoutes(), "own videos never leave the node")
}

func TestPlayVideo_ForeignVideoComesFromTheAuthorOnce(t *testing.T) {
	tb := newPlayTestbed(t, 2*media_meta.VideoChunkSize+300)

	manifest, body, err := tb.play(t, videoAuthorID, tb.key, 0)
	require.NoError(t, err)
	assert.Equal(t, tb.key, manifest.Key())
	assert.True(t, bytes.Equal(tb.video, body))

	_, body, err = tb.play(t, videoAuthorID, tb.key, 0)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(tb.video, body))
	assert.Equal(t, []stream.WarpRoute{event.PUBLIC_GET_VIDEO_CHUNKS}, tb.streamer.openedRoutes(),
		"one stream from the author, then the cache")
}

func TestPlayVideo_UnavailableVideos(t *testing.T) {
	tb := newPlayTestbed(t, 100)

	_, _, err := tb.play(t, ownerID, "missing", 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)
	_, _, err = tb.play(t, "stranger", tb.key, 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)

	tb.streamer.openErr = warpnet.ErrNodeIsOffline
	_, _, err = tb.play(t, videoAuthorID, tb.key, 0)
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)

	_, err = StreamPlayVideoHandler(tb.streamer, tb.videos, tb.users)(mustJSON(t, event.GetVideoChunksEvent{UserId: ownerID}), nil)
	assert.ErrorIs(t, err, ErrEmptyVideoKey)
}

func TestPlayChatVideo_UsesTheChatRoutes(t *testing.T) {
	tb := newPlayTestbed(t, 100)
	h := StreamPlayChatVideoHandler(tb.streamer, tb.chatVideos, chatUserDouble{byId: map[string]domain.User{videoAuthorID: ownerOf(videoAuthorID)}})

	_, body, err := serveVideo(t, h, event.GetVideoChunksEvent{UserId: videoAuthorID, Key: tb.key}, tb.streamer.self)
	require.NoError(t, err)
	assert.Equal(t, tb.video, body)
	assert.Equal(t, []stream.WarpRoute{event.PUBLIC_GET_CHAT_VIDEO_CHUNKS}, tb.streamer.openedRoutes())

	_, err = tb.chatVideos.GetManifest(videoAuthorID, tb.key)
	assert.NoError(t, err)
	_, err = tb.videos.GetManifest(videoAuthorID, tb.key)
	assert.ErrorIs(t, err, database.ErrMediaNotFound, "chat videos are kept apart from the timeline")
}
