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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package security

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pipe struct{ toResp, toInit chan []byte }

func newPipe() *pipe {
	return &pipe{toResp: make(chan []byte, 4), toInit: make(chan []byte, 4)}
}

func TestNoiseHandshake_RoundTrip(t *testing.T) {
	static, err := LoadOrCreateNoiseStaticKey(filepath.Join(t.TempDir(), "ws-noise.key"))
	require.NoError(t, err)

	p := newPipe()
	respErr := make(chan error, 1)
	var responder *NoiseSession
	go func() {
		var err error
		responder, err = NoiseHandshake(static,
			func() ([]byte, error) { return <-p.toResp, nil },
			func(msg []byte) error { p.toInit <- msg; return nil },
		)
		respErr <- err
	}()

	initiator, err := NoiseHandshakeInitiator(
		func() ([]byte, error) { return <-p.toInit, nil },
		func(msg []byte) error { p.toResp <- msg; return nil },
	)
	require.NoError(t, err)
	require.NoError(t, <-respErr)

	assert.Equal(t, static.Public, initiator.RemoteStatic())
	assert.Nil(t, responder.RemoteStatic(), "NX clients are anonymous")

	for i := 0; i < 3; i++ {
		req := []byte(`{"path":"is-first-run"}`)
		sealed, err := initiator.Encrypt(req)
		require.NoError(t, err)
		assert.NotContains(t, string(sealed), "is-first-run")
		plain, err := responder.Decrypt(sealed)
		require.NoError(t, err)
		assert.Equal(t, req, plain)

		reply := []byte(`{"body":true}`)
		sealed, err = responder.Encrypt(reply)
		require.NoError(t, err)
		plain, err = initiator.Decrypt(sealed)
		require.NoError(t, err)
		assert.Equal(t, reply, plain)
	}
}

func TestNoiseSession_ReplayAndTamperAreRejected(t *testing.T) {
	static, err := LoadOrCreateNoiseStaticKey(filepath.Join(t.TempDir(), "ws-noise.key"))
	require.NoError(t, err)

	p := newPipe()
	initiatorCh := make(chan *NoiseSession, 1)
	go func() {
		s, err := NoiseHandshakeInitiator(
			func() ([]byte, error) { return <-p.toInit, nil },
			func(msg []byte) error { p.toResp <- msg; return nil },
		)
		if err != nil {
			close(initiatorCh)
			return
		}
		initiatorCh <- s
	}()
	responder, err := NoiseHandshake(static,
		func() ([]byte, error) { return <-p.toResp, nil },
		func(msg []byte) error { p.toInit <- msg; return nil },
	)
	require.NoError(t, err)
	initiator := <-initiatorCh
	require.NotNil(t, initiator)

	frame, err := initiator.Encrypt([]byte("one"))
	require.NoError(t, err)
	_, err = responder.Decrypt(frame)
	require.NoError(t, err)

	_, err = responder.Decrypt(frame)
	assert.Error(t, err, "replayed frame must not decrypt")

	frame2, err := initiator.Encrypt([]byte("two"))
	require.NoError(t, err)
	frame2[len(frame2)-1] ^= 0xff
	_, err = responder.Decrypt(frame2)
	assert.Error(t, err, "tampered frame must not decrypt")
}

func TestLoadOrCreateNoiseStaticKey_PersistsAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage-parent", "ws-noise.key")

	first, err := LoadOrCreateNoiseStaticKey(path)
	require.NoError(t, err)
	assert.Len(t, first.Private, 32)
	assert.Len(t, first.Public, 32)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "private key must not be world-readable")

	second, err := LoadOrCreateNoiseStaticKey(path)
	require.NoError(t, err)
	assert.Equal(t, first, second, "a restart must keep the pinned identity stable")
}

func TestLoadOrCreateNoiseStaticKey_RejectsCorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ws-noise.key")
	require.NoError(t, os.WriteFile(path, []byte("short"), 0o600))

	_, err := LoadOrCreateNoiseStaticKey(path)
	assert.ErrorIs(t, err, ErrNoiseKeyCorrupted)
}

func TestNoiseFingerprint_IsStable(t *testing.T) {
	pub, _ := hex.DecodeString("64b101b1d0be5a8704bd078f9895001fc03e8e9f9522f188dd128d9846d48466")
	a := NoiseFingerprint(pub)
	b := NoiseFingerprint(pub)
	assert.Equal(t, a, b)
	assert.Len(t, a, 64, "sha256 hex")
	assert.NotEqual(t, a, NoiseFingerprint([]byte("other")))
}
