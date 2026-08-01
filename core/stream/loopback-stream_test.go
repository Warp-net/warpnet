package stream

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
)

func TestNewLoopbackStream(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	assert.NotNil(t, r)
	assert.NotNil(t, w)
	defer r.Close()
	defer w.Close()
}

func TestLoopbackStream_ReadWrite(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	go func() {
		_, _ = w.Write([]byte("hello"))
		_ = w.CloseWrite()
	}()

	buf := make([]byte, 5)
	n, err := r.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf))
}

func TestLoopbackStream_Protocol(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	assert.Equal(t, protocol.ID("/test/proto"), r.Protocol())
}

func TestLoopbackStream_SetProtocol(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	err := r.SetProtocol("/new/proto")
	assert.NoError(t, err)
	assert.Equal(t, protocol.ID("/new/proto"), r.Protocol())
}

func TestLoopbackStream_ID(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	assert.Equal(t, loopbackStreamName, r.ID())
}

func TestLoopbackStream_Stat(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	stat := r.Stat()
	assert.Equal(t, network.DirInbound, stat.Direction)
}

func TestLoopbackStream_Close(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	_ = w.Close()
	err := r.Close()
	assert.NoError(t, err)
}

func TestLoopbackStream_CloseIdempotent(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	_ = w.Close()
	_ = r.Close()
	err := r.Close()
	assert.NoError(t, err)
}

func TestLoopbackStream_Reset(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	err := r.Reset()
	assert.NoError(t, err)
}

func TestLoopbackStream_Scope(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	assert.Nil(t, r.Scope())
}

func TestLoopbackStream_SetDeadline(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	err := r.SetDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)
}

func TestLoopbackConn_LocalPeer(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	assert.Equal(t, peer.ID("peer1"), conn.LocalPeer())
	assert.Equal(t, peer.ID("peer1"), conn.RemotePeer())
}

func TestLoopbackConn_ConnState(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	state := conn.ConnState()
	assert.Equal(t, loopbackStreamName, string(state.StreamMultiplexer))
	assert.Equal(t, loopbackStreamName, string(state.Security))
	assert.Equal(t, loopbackStreamName, string(state.Transport))
}

func TestLoopbackConn_Multiaddrs(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	assert.NotNil(t, conn.LocalMultiaddr())
	assert.NotNil(t, conn.RemoteMultiaddr())
}

func TestLoopbackConn_ID(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	assert.Equal(t, peer.ID("peer1").String(), conn.ID())
}

func TestLoopbackConn_Stat(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	stat := conn.Stat()
	assert.Equal(t, network.DirInbound, stat.Direction)
}

func TestLoopbackConn_NewStream(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	s, err := conn.NewStream(t.Context())
	assert.NoError(t, err)
	assert.NotNil(t, s)
}

func TestLoopbackConn_GetStreams(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	streams := conn.GetStreams()
	assert.Len(t, streams, 1)
}

func TestLoopbackConn_RemotePublicKey(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	assert.Nil(t, conn.RemotePublicKey())
}

func TestLoopbackConn_Scope(t *testing.T) {
	r, w := NewLoopbackStream("peer1", "/test/proto")
	defer r.Close()
	defer w.Close()

	conn := r.Conn()
	assert.Nil(t, conn.Scope())
}

// Reset must actually tear the pipe down. While it was a no-op, a handler
// that stopped reading early left the peer blocked in Write until its
// deadline, which surfaced as an upload hanging with no response at all.
func TestLoopbackStreamResetUnblocksBlockedWriter(t *testing.T) {
	client, server := NewLoopbackStream("peer1", "/test/proto")

	writeReturned := make(chan struct{})
	go func() {
		defer close(writeReturned)
		_ = client.SetDeadline(time.Now().Add(30 * time.Second))
		// Far more than any pipe buffer, so this blocks until the other
		// side either reads it or tears the stream down.
		_, _ = client.Write(make([]byte, 8<<20))
	}()

	// Give the writer a moment to actually block, then reset.
	time.Sleep(50 * time.Millisecond)
	if err := server.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}

	select {
	case <-writeReturned:
	case <-time.After(10 * time.Second):
		t.Fatal("Reset did not unblock the blocked writer")
	}
}
