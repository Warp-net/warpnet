//nolint:all
package videostream

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type frames struct {
	mx  sync.Mutex
	got []event.VideoFrame
	err error
}

func (f *frames) SendFrame(_ string, frame event.VideoFrame) error {
	f.mx.Lock()
	defer f.mx.Unlock()
	f.got = append(f.got, frame)
	return f.err
}

func (f *frames) count() int {
	f.mx.Lock()
	defer f.mx.Unlock()
	return len(f.got)
}

func (f *frames) all() []event.VideoFrame {
	f.mx.Lock()
	defer f.mx.Unlock()
	return append([]event.VideoFrame(nil), f.got...)
}

func testVideo(size int) []byte {
	video := make([]byte, size)
	for i := range video {
		video[i] = byte(i % 241)
	}
	return video
}

func serveVideo(t *testing.T, video []byte, offset int64) (warpnet.WarpStream, chan error) {
	t.Helper()
	manifest := media_meta.BuildVideoManifest(video, "video/mp4")
	client, server := stream.NewLoopbackStream("", "", "/private/get/video/0.0.0")
	written := make(chan error, 1)
	go func() {
		written <- writeVideo(server, manifest, video, offset)
		_ = server.Close()
	}()
	return client, written
}

func writeVideo(s warpnet.WarpStream, manifest domain.VideoManifest, video []byte, offset int64) error {
	if err := media_meta.WriteVideoManifest(s, manifest); err != nil {
		return err
	}
	size := int64(len(video))
	for start := offset - offset%media_meta.VideoChunkSize; start < size; start += media_meta.VideoChunkSize {
		if _, err := s.Write(video[start:min(start+media_meta.VideoChunkSize, size)]); err != nil {
			return err
		}
	}
	return nil
}

func TestSessions_FramesFlowOnlyForCredits(t *testing.T) {
	video := testVideo(2*media_meta.VideoChunkSize + 500)
	s, _ := serveVideo(t, video, 0)
	sessions := NewSessions()
	sent := &frames{}

	manifest, err := sessions.Start("stream-1", 0, s, sent)
	require.NoError(t, err)
	assert.Equal(t, int64(len(video)), manifest.Size)

	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, sent.count(), "nothing is pushed before the player asks")

	sessions.AddCredits("stream-1", 2)
	require.Eventually(t, func() bool { return sent.count() == 2 }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, sent.count(), "two credits, two chunks")

	sessions.AddCredits("stream-1", 5)
	require.Eventually(t, func() bool { return sent.count() == 3 }, 5*time.Second, 5*time.Millisecond)

	var joined []byte
	for i, frame := range sent.all() {
		assert.Equal(t, int64(i)*media_meta.VideoChunkSize, frame.Offset)
		assert.Equal(t, i == 2, frame.IsLast)
		joined = append(joined, frame.Data...)
	}
	assert.True(t, bytes.Equal(video, joined))
}

func TestSessions_StreamStartsAtTheChunkOfTheOffset(t *testing.T) {
	video := testVideo(2*media_meta.VideoChunkSize + 500)
	offset := int64(media_meta.VideoChunkSize + 77)
	s, _ := serveVideo(t, video, offset)
	sessions := NewSessions()
	sent := &frames{}

	_, err := sessions.Start("stream-1", offset, s, sent)
	require.NoError(t, err)
	sessions.AddCredits("stream-1", maxCredits)

	require.Eventually(t, func() bool { return sent.count() == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, int64(media_meta.VideoChunkSize), sent.all()[0].Offset)
	assert.True(t, sent.all()[1].IsLast)
}

func TestSessions_StopReleasesTheNodeStream(t *testing.T) {
	video := testVideo(3 * media_meta.VideoChunkSize)
	s, written := serveVideo(t, video, 0)
	sessions := NewSessions()
	sent := &frames{}

	_, err := sessions.Start("stream-1", 0, s, sent)
	require.NoError(t, err)
	sessions.AddCredits("stream-1", 1)
	require.Eventually(t, func() bool { return sent.count() == 1 }, 5*time.Second, 5*time.Millisecond)

	sessions.Stop("stream-1")

	select {
	case err := <-written:
		assert.Error(t, err, "the handler stops writing once the player is gone")
	case <-time.After(5 * time.Second):
		t.Fatal("the handler kept the stream")
	}
	sessions.AddCredits("stream-1", maxCredits)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, sent.count())
}

func TestSessions_BrokenStreamSendsAnErrorFrame(t *testing.T) {
	video := testVideo(2 * media_meta.VideoChunkSize)
	manifest := media_meta.BuildVideoManifest(video, "video/mp4")
	client, server := stream.NewLoopbackStream("", "", "/private/get/video/0.0.0")
	go func() {
		_ = media_meta.WriteVideoManifest(server, manifest)
		_, _ = server.Write(video[:media_meta.VideoChunkSize])
		_ = server.Reset()
	}()

	sessions := NewSessions()
	sent := &frames{}
	_, err := sessions.Start("stream-1", 0, client, sent)
	require.NoError(t, err)
	sessions.AddCredits("stream-1", maxCredits)

	require.Eventually(t, func() bool { return sent.count() == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Empty(t, sent.all()[0].Error)
	assert.NotEmpty(t, sent.all()[1].Error, "the player learns the stream broke and can reopen it")
}

func TestSessions_UnavailableVideoStartsNothing(t *testing.T) {
	client, server := stream.NewLoopbackStream("", "", "/private/get/video/0.0.0")
	go func() {
		payload, _ := json.Marshal(domain.VideoManifest{})
		_, _ = server.Write(payload)
		_ = server.Close()
	}()

	_, err := NewSessions().Start("stream-1", 0, client, &frames{})
	assert.ErrorIs(t, err, media_meta.ErrVideoUnavailable)
}

func TestSessions_StopAllEndsEveryStream(t *testing.T) {
	sessions := NewSessions()
	var written []chan error
	for _, id := range []string{"a", "b"} {
		s, w := serveVideo(t, testVideo(2*media_meta.VideoChunkSize), 0)
		_, err := sessions.Start(id, 0, s, &frames{err: errors.New("unused")})
		require.NoError(t, err)
		written = append(written, w)
	}

	sessions.StopAll()

	for _, w := range written {
		select {
		case err := <-w:
			assert.Error(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("a stream outlived StopAll")
		}
	}
}
