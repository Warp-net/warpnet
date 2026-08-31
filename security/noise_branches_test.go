//nolint:all
package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOrCreateNoiseStaticKey(t *testing.T) {
	t.Run("creates and then reloads the same key", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "keys", "noise.key")

		created, err := LoadOrCreateNoiseStaticKey(path)
		require.NoError(t, err)
		require.Len(t, created.Private, noiseKeySize)
		require.NotEmpty(t, created.Public)

		reloaded, err := LoadOrCreateNoiseStaticKey(path)
		require.NoError(t, err)
		require.Equal(t, created.Private, reloaded.Private)
		require.Equal(t, created.Public, reloaded.Public)
	})

	t.Run("rejects a truncated key file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "noise.key")
		require.NoError(t, os.WriteFile(path, []byte("too short"), 0o600))

		_, err := LoadOrCreateNoiseStaticKey(path)
		require.ErrorIs(t, err, ErrNoiseKeyCorrupted)
	})

	t.Run("reports an unreadable key file", func(t *testing.T) {
		// a directory in place of the key file is readable as a path but not as a file
		dir := t.TempDir()
		_, err := LoadOrCreateNoiseStaticKey(dir)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNoiseKeyCorrupted)
	})

	t.Run("reports an unwritable key location", func(t *testing.T) {
		blocked := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(blocked, nil, 0o600))

		_, err := LoadOrCreateNoiseStaticKey(filepath.Join(blocked, "noise.key"))
		require.Error(t, err)
	})
}

func TestNoiseFingerprintIsStable(t *testing.T) {
	key, err := GenerateNoiseKey()
	require.NoError(t, err)

	first := NoiseFingerprint(key.Public)
	require.Len(t, first, 64)
	require.Equal(t, first, NoiseFingerprint(key.Public))

	other, err := GenerateNoiseKey()
	require.NoError(t, err)
	require.NotEqual(t, first, NoiseFingerprint(other.Public))
}

// noisePair wires a responder and an initiator over in-memory channels.
func noisePair(t *testing.T) (responder, initiator *NoiseSession) {
	t.Helper()

	serverKey, err := GenerateNoiseKey()
	require.NoError(t, err)
	clientKey, err := GenerateNoiseKey()
	require.NoError(t, err)

	toServer := make(chan []byte, 4)
	toClient := make(chan []byte, 4)

	// each side owns its own error variable: the two run concurrently
	var responderErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		responder, responderErr = NoiseHandshake(serverKey,
			func() ([]byte, error) { return <-toServer, nil },
			func(b []byte) error { toClient <- b; return nil },
		)
	}()

	initiator, err = NoiseHandshakeInitiator(clientKey,
		func() ([]byte, error) { return <-toClient, nil },
		func(b []byte) error { toServer <- b; return nil },
	)
	require.NoError(t, err)

	<-done
	require.NoError(t, responderErr)
	return responder, initiator
}

func TestNoiseHandshakeRoundTrip(t *testing.T) {
	responder, initiator := noisePair(t)

	sealed, err := initiator.Encrypt([]byte("hello node"))
	require.NoError(t, err)

	plain, err := responder.Decrypt(sealed)
	require.NoError(t, err)
	require.Equal(t, "hello node", string(plain))

	back, err := responder.Encrypt([]byte("hello client"))
	require.NoError(t, err)
	plain, err = initiator.Decrypt(back)
	require.NoError(t, err)
	require.Equal(t, "hello client", string(plain))

	require.NotEmpty(t, responder.RemoteStatic())
	require.NotEmpty(t, initiator.RemoteStatic())
}

func TestNoiseHandshakeTransportFailures(t *testing.T) {
	key, err := GenerateNoiseKey()
	require.NoError(t, err)
	transportErr := errors.New("connection reset")

	failRead := func() ([]byte, error) { return nil, transportErr }
	okWrite := func([]byte) error { return nil }
	failWrite := func([]byte) error { return transportErr }

	t.Run("responder cannot read the initiation", func(t *testing.T) {
		_, err := NoiseHandshake(key, failRead, okWrite)
		require.ErrorIs(t, err, transportErr)
	})

	t.Run("responder rejects a malformed initiation", func(t *testing.T) {
		_, err := NoiseHandshake(key,
			func() ([]byte, error) { return []byte("not-noise"), nil }, okWrite)
		require.Error(t, err)
	})

	t.Run("initiator cannot write the initiation", func(t *testing.T) {
		_, err := NoiseHandshakeInitiator(key, failRead, failWrite)
		require.ErrorIs(t, err, transportErr)
	})

	t.Run("initiator cannot read the response", func(t *testing.T) {
		_, err := NoiseHandshakeInitiator(key, failRead, okWrite)
		require.ErrorIs(t, err, transportErr)
	})

	t.Run("initiator rejects a malformed response", func(t *testing.T) {
		_, err := NoiseHandshakeInitiator(key,
			func() ([]byte, error) { return []byte("not-noise"), nil }, okWrite)
		require.Error(t, err)
	})

	t.Run("responder cannot write its response", func(t *testing.T) {
		clientKey, err := GenerateNoiseKey()
		require.NoError(t, err)

		// produce a genuine first message so the responder gets past ReadMessage
		toServer := make(chan []byte, 1)
		go func() {
			_, _ = NoiseHandshakeInitiator(clientKey,
				func() ([]byte, error) { return nil, transportErr },
				func(b []byte) error { toServer <- b; return nil },
			)
		}()

		_, err = NoiseHandshake(key,
			func() ([]byte, error) { return <-toServer, nil }, failWrite)
		require.ErrorIs(t, err, transportErr)
	})

	t.Run("responder cannot read the client static", func(t *testing.T) {
		clientKey, err := GenerateNoiseKey()
		require.NoError(t, err)

		toServer := make(chan []byte, 2)
		toClient := make(chan []byte, 2)
		go func() {
			_, _ = NoiseHandshakeInitiator(clientKey,
				func() ([]byte, error) { return <-toClient, nil },
				func(b []byte) error { toServer <- b; return nil },
			)
		}()

		reads := 0
		_, err = NoiseHandshake(key,
			func() ([]byte, error) {
				reads++
				if reads == 1 {
					return <-toServer, nil
				}
				return nil, transportErr
			},
			func(b []byte) error { toClient <- b; return nil },
		)
		require.ErrorIs(t, err, transportErr)
	})
}
