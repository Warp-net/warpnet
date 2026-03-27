/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package dpi

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// fakeConn implements manet.Conn for testing
// ---------------------------------------------------------------------------

type fakeConn struct {
	mu     sync.Mutex
	writes [][]byte // each Write call recorded separately
	closed bool
}

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "127.0.0.1:0" }

func (f *fakeConn) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(b))
	copy(cp, b)
	f.writes = append(f.writes, cp)
	return len(b), nil
}

func (f *fakeConn) Read([]byte) (int, error)        { return 0, nil }
func (f *fakeConn) Close() error                     { f.closed = true; return nil }
func (f *fakeConn) LocalAddr() net.Addr              { return fakeAddr{} }
func (f *fakeConn) RemoteAddr() net.Addr             { return fakeAddr{} }
func (f *fakeConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }
func (f *fakeConn) LocalMultiaddr() ma.Multiaddr     { return nil }
func (f *fakeConn) RemoteMultiaddr() ma.Multiaddr    { return nil }

func (f *fakeConn) allBytes() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []byte
	for _, w := range f.writes {
		out = append(out, w...)
	}
	return out
}

func (f *fakeConn) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func (f *fakeConn) writeSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	sizes := make([]int, len(f.writes))
	for i, w := range f.writes {
		sizes[i] = len(w)
	}
	return sizes
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSpoofConn_FragmentsDuringHandshake(t *testing.T) {
	fc := &fakeConn{}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 2,
		handshakeLen: 10,
		maxDelay:     0, // no delays in tests
	}

	data := []byte("HELLO_WORLD!") // 12 bytes: 10 in handshake + 2 after
	n, err := sc.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)

	// The underlying conn should have received all data intact.
	assert.Equal(t, data, fc.allBytes())

	// During the handshake (first 10 bytes) writes should be fragmented
	// into 2-byte chunks: 5 writes for 10 bytes, plus 1 write for the
	// remaining 2 bytes after handshake.
	sizes := fc.writeSizes()
	assert.Equal(t, 6, len(sizes), "expected 6 writes, got %d: %v", len(sizes), sizes)
	for i := 0; i < 5; i++ {
		assert.Equal(t, 2, sizes[i], "handshake fragment %d should be 2 bytes", i)
	}
	assert.Equal(t, 2, sizes[5], "post-handshake remainder should be written at once")
}

func TestSpoofConn_PassthroughAfterHandshake(t *testing.T) {
	fc := &fakeConn{}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 1,
		handshakeLen: 4,
		maxDelay:     0,
	}

	// Exhaust the handshake phase.
	_, err := sc.Write([]byte("ABCD"))
	require.NoError(t, err)
	assert.Equal(t, 4, fc.writeCount(), "handshake should produce 4 single-byte writes")

	// Now writes should pass through without fragmentation.
	bigData := bytes.Repeat([]byte("X"), 1024)
	_, err = sc.Write(bigData)
	require.NoError(t, err)
	assert.Equal(t, 5, fc.writeCount(), "post-handshake should be a single write")
	assert.Equal(t, 1024, fc.writeSizes()[4])
}

func TestSpoofConn_PartialHandshakeAcrossWrites(t *testing.T) {
	fc := &fakeConn{}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 3,
		handshakeLen: 8,
		maxDelay:     0,
	}

	// First write: 5 bytes. Should be fragmented into 3+2 (still in handshake).
	n, err := sc.Write([]byte("ABCDE"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 2, fc.writeCount()) // 3 + 2

	// Second write: 6 bytes. First 3 bytes are still handshake, remaining 3 are post.
	n, err = sc.Write([]byte("FGHIJK"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	// Total data should be intact.
	assert.Equal(t, []byte("ABCDEFGHIJK"), fc.allBytes())
}

func TestSpoofConn_FragmentSizeLargerThanData(t *testing.T) {
	fc := &fakeConn{}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 100,
		handshakeLen: 200,
		maxDelay:     0,
	}

	data := []byte("short")
	n, err := sc.Write(data)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 1, fc.writeCount(), "data smaller than fragment size should be one write")
	assert.Equal(t, data, fc.allBytes())
}

func TestSpoofConn_EmptyWrite(t *testing.T) {
	fc := &fakeConn{}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 2,
		handshakeLen: 10,
		maxDelay:     0,
	}

	n, err := sc.Write(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, fc.writeCount())
}

func TestSpoofConn_ZeroFragmentSizeUsesDefault(t *testing.T) {
	fc := &fakeConn{}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 0, // zero triggers fallback to DefaultFragmentSize
		handshakeLen: 10,
		maxDelay:     0,
	}

	data := []byte("ABCDEF") // 6 bytes, should fragment into DefaultFragmentSize (2) chunks
	n, err := sc.Write(data)
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, data, fc.allBytes())

	// With DefaultFragmentSize=2, 6 bytes should produce 3 writes.
	assert.Equal(t, 3, fc.writeCount())
	for _, s := range fc.writeSizes() {
		assert.Equal(t, 2, s)
	}
}

func TestOptions(t *testing.T) {
	st := &SpoofTransport{
		fragmentSize:   DefaultFragmentSize,
		handshakeLen:   DefaultHandshakeLen,
		maxDelay:       DefaultMaxDelay,
		connectTimeout: defaultConnectTimeout,
	}

	require.NoError(t, WithFragmentSize(5)(st))
	assert.Equal(t, 5, st.fragmentSize)

	require.NoError(t, WithHandshakeLen(2048)(st))
	assert.Equal(t, 2048, st.handshakeLen)

	require.NoError(t, WithMaxDelay(10*time.Millisecond)(st))
	assert.Equal(t, 10*time.Millisecond, st.maxDelay)

	require.NoError(t, WithConnectTimeout(30*time.Second)(st))
	assert.Equal(t, 30*time.Second, st.connectTimeout)

	require.NoError(t, WithSNI("custom.example.com")(st))
	assert.Equal(t, "custom.example.com", st.sni)

	require.NoError(t, WithBrowserFingerprint(BrowserFirefox)(st))
	assert.Equal(t, BrowserFirefox, st.browserFingerprint)

	require.NoError(t, WithHandshakeTimeout(5*time.Second)(st))
	assert.Equal(t, 5*time.Second, st.handshakeTimeout)

	// Negative/zero values should not change defaults.
	st2 := &SpoofTransport{fragmentSize: 2, handshakeLen: 512}
	require.NoError(t, WithFragmentSize(0)(st2))
	assert.Equal(t, 2, st2.fragmentSize)
	require.NoError(t, WithHandshakeLen(-1)(st2))
	assert.Equal(t, 512, st2.handshakeLen)
	require.NoError(t, WithConnectTimeout(-1)(st2))
	assert.Equal(t, time.Duration(0), st2.connectTimeout)
	require.NoError(t, WithHandshakeTimeout(0)(st2))
	assert.Equal(t, time.Duration(0), st2.handshakeTimeout)
}

func TestRandDuration(t *testing.T) {
	// Zero max should return zero.
	assert.Equal(t, time.Duration(0), randDuration(0))
	assert.Equal(t, time.Duration(0), randDuration(-1))

	// Positive max should return a value in [0, max).
	for range 100 {
		d := randDuration(10 * time.Millisecond)
		assert.True(t, d >= 0 && d < 10*time.Millisecond, "got %v", d)
	}
}

// ---------------------------------------------------------------------------
// Short-write / zero-write tests
// ---------------------------------------------------------------------------

// shortWriteConn simulates a connection that returns partial writes.
type shortWriteConn struct {
	fakeConn
	maxPerWrite int // max bytes per Write call
}

func (s *shortWriteConn) Write(b []byte) (int, error) {
	n := len(b)
	if n > s.maxPerWrite {
		n = s.maxPerWrite
	}
	return s.fakeConn.Write(b[:n])
}

func TestSpoofConn_ShortWrite(t *testing.T) {
	fc := &shortWriteConn{maxPerWrite: 1}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 3,
		handshakeLen: 10,
		maxDelay:     0,
	}

	data := []byte("ABCDEF")
	n, err := sc.Write(data)
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, data, fc.allBytes())
}

// zeroWriteConn always returns (0, nil) to trigger infinite-loop detection.
type zeroWriteConn struct {
	fakeConn
}

func (z *zeroWriteConn) Write([]byte) (int, error) {
	return 0, nil
}

func TestSpoofConn_ZeroWriteReturnsError(t *testing.T) {
	fc := &zeroWriteConn{}
	sc := &SpoofConn{
		Conn:         fc,
		fragmentSize: 2,
		handshakeLen: 10,
		maxDelay:     0,
	}

	_, err := sc.Write([]byte("AB"))
	require.Error(t, err)
	assert.ErrorIs(t, err, io.ErrShortWrite)
}
