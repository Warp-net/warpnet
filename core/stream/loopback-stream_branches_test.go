//nolint:all
package stream

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/stretchr/testify/require"
)

func TestLoopbackConnSurface(t *testing.T) {
	client, server := NewLoopbackStream("node-a", "node-b", "/public/get/info/0.0.0")
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	const local, remote = warpnet.WarpPeerID("node-a"), warpnet.WarpPeerID("node-b")

	conn := server.Conn()
	require.Equal(t, local.String(), conn.LocalPeer().String())
	require.Equal(t, remote.String(), conn.RemotePeer().String())
	require.Nil(t, conn.RemotePublicKey())
	require.False(t, conn.As(nil))
	require.Equal(t, local.String(), conn.ID())
	require.Nil(t, conn.Scope())
	require.NotNil(t, conn.LocalMultiaddr())
	require.NotNil(t, conn.RemoteMultiaddr())
	require.Equal(t, "loopback", conn.ConnState().Transport)
	require.Equal(t, network.DirInbound, conn.Stat().Direction)
	require.Len(t, conn.GetStreams(), 1)

	newStream, err := conn.NewStream(t.Context())
	require.NoError(t, err)
	require.NotNil(t, newStream)

	require.False(t, conn.IsClosed())
	require.NoError(t, conn.Close())
	require.True(t, conn.IsClosed())

	// closing again, and via the error-carrying variant, must stay a no-op
	require.NoError(t, conn.Close())
	require.NoError(t, conn.CloseWithError(network.ConnErrorCode(1)))
}

func TestLoopbackStreamSurface(t *testing.T) {
	client, server := NewLoopbackStream("node-a", "node-b", "/public/get/info/0.0.0")

	require.Equal(t, "loopback", server.ID())
	require.Nil(t, server.Scope())
	require.Equal(t, warpnet.WarpProtocolID("/public/get/info/0.0.0"), server.Protocol())
	require.NoError(t, server.SetProtocol("/public/get/tweets/0.0.0"))
	require.Equal(t, warpnet.WarpProtocolID("/public/get/tweets/0.0.0"), server.Protocol())
	require.Equal(t, network.DirInbound, server.Stat().Direction)

	deadline := time.Now().Add(time.Minute)
	require.NoError(t, server.SetDeadline(deadline))
	require.NoError(t, server.SetReadDeadline(deadline))
	require.NoError(t, server.SetWriteDeadline(deadline))

	// a live pair moves bytes end to end
	go func() {
		_, _ = client.Write([]byte("ping"))
		_ = client.CloseWrite()
	}()
	buf := make([]byte, 4)
	n, err := server.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "ping", string(buf[:n]))

	// ResetWithError only reports; Reset closes both halves
	require.NoError(t, server.ResetWithError(network.StreamErrorCode(1)))
	require.NoError(t, server.Reset())

	// reads and writes on a closed stream are silent no-ops
	n, err = server.Read(buf)
	require.NoError(t, err)
	require.Zero(t, n)
	n, err = server.Write([]byte("pong"))
	require.NoError(t, err)
	require.Zero(t, n)

	require.NoError(t, server.CloseRead())
	require.NoError(t, server.CloseWrite())
	require.NoError(t, server.Close())
	require.NoError(t, client.Close())
}

func TestReadRequestLimits(t *testing.T) {
	t.Run("control route", func(t *testing.T) {
		client, server := NewLoopbackStream("node-a", "node-b", "/public/get/info/0.0.0")
		t.Cleanup(func() {
			_ = client.Close()
			_ = server.Close()
		})

		go func() {
			_, _ = client.Write([]byte(`{"a":1}`))
			_ = client.CloseWrite()
		}()

		data, err := ReadRequest(server)
		require.NoError(t, err)
		require.JSONEq(t, `{"a":1}`, string(data))
	})

	t.Run("media route", func(t *testing.T) {
		client, server := NewLoopbackStream("node-a", "node-b", warpnet.WarpProtocolID("/public/get/image/0.0.0"))
		t.Cleanup(func() {
			_ = client.Close()
			_ = server.Close()
		})

		go func() {
			_, _ = client.Write([]byte("binary-ish"))
			_ = client.CloseWrite()
		}()

		data, err := ReadRequest(server)
		require.NoError(t, err)
		require.Equal(t, "binary-ish", string(data))
	})
}
