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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package security

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

// The dashboard /ws channel runs Noise NX: the browser is an anonymous
// initiator, the node authenticates with a long-lived static X25519 key that
// the browser pins on first contact (trust-on-first-use). Each connection
// derives fresh session keys via ephemeral ECDH (forward secrecy), and the
// transport cipher states use counter nonces, so frames cannot be replayed
// or reordered within or across sessions. No preshared secret is involved;
// the owner's account credentials travel inside the encrypted channel.
var noiseSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

const noiseKeySize = 32

var ErrNoiseKeyCorrupted = errors.New("security: noise static key file is corrupted")

// LoadOrCreateNoiseStaticKey reads the node's static X25519 private key from
// path, generating and persisting a new one (0600) on first run.
func LoadOrCreateNoiseStaticKey(path string) (noise.DHKey, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key, err := noiseSuite.GenerateKeypair(nil)
		if err != nil {
			return noise.DHKey{}, fmt.Errorf("security: generate noise key: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return noise.DHKey{}, fmt.Errorf("security: noise key dir: %w", err)
		}
		if err := os.WriteFile(path, key.Private, 0o600); err != nil {
			return noise.DHKey{}, fmt.Errorf("security: persist noise key: %w", err)
		}
		return key, nil
	}
	if err != nil {
		return noise.DHKey{}, fmt.Errorf("security: read noise key: %w", err)
	}
	if len(raw) != noiseKeySize {
		return noise.DHKey{}, ErrNoiseKeyCorrupted
	}
	pub, err := curve25519.X25519(raw, curve25519.Basepoint)
	if err != nil {
		return noise.DHKey{}, fmt.Errorf("security: derive noise public key: %w", err)
	}
	return noise.DHKey{Private: raw, Public: pub}, nil
}

// NoiseFingerprint is the human-comparable form of a static public key, shown
// by the node at startup and by the browser when it pins an unknown node.
func NoiseFingerprint(pub []byte) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// NoiseSession is an established transport channel. Decrypt is only safe from
// a single reader goroutine. Encrypt uses a counter nonce, so the caller must
// serialize Encrypt with the actual network write under one lock: the peer
// decrypts frames in arrival order.
type NoiseSession struct {
	send *noise.CipherState
	recv *noise.CipherState

	remoteStatic []byte // initiator side only: the responder's static key
}

func (s *NoiseSession) Encrypt(plain []byte) ([]byte, error) {
	return s.send.Encrypt(nil, nil, plain)
}

func (s *NoiseSession) Decrypt(frame []byte) ([]byte, error) {
	return s.recv.Decrypt(nil, nil, frame)
}

// RemoteStatic returns the peer's static public key learned during the
// handshake (initiator side); nil on the responder side (NX clients are
// anonymous).
func (s *NoiseSession) RemoteStatic() []byte { return s.remoteStatic }

// NoiseHandshake runs the responder side of the NX handshake over a
// message-oriented transport (one read/write per handshake message).
func NoiseHandshake(static noise.DHKey, read func() ([]byte, error), write func([]byte) error) (*NoiseSession, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   noiseSuite,
		Pattern:       noise.HandshakeNX,
		StaticKeypair: static,
	})
	if err != nil {
		return nil, fmt.Errorf("security: init noise responder: %w", err)
	}

	msg1, err := read()
	if err != nil {
		return nil, fmt.Errorf("security: read noise initiation: %w", err)
	}
	if _, _, _, err = hs.ReadMessage(nil, msg1); err != nil {
		return nil, fmt.Errorf("security: bad noise initiation: %w", err)
	}

	// Split() order is fixed by the Noise spec regardless of role: the first
	// CipherState carries initiator→responder traffic, the second the reverse.
	msg2, recv, send, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("security: build noise response: %w", err)
	}
	if err := write(msg2); err != nil {
		return nil, fmt.Errorf("security: write noise response: %w", err)
	}
	return &NoiseSession{send: send, recv: recv}, nil
}

// NoiseHandshakeInitiator runs the initiator (client) side of the NX
// handshake. The browser dashboard implements the same role in JS; this Go
// version serves tests and throwaway probes.
func NoiseHandshakeInitiator(read func() ([]byte, error), write func([]byte) error) (*NoiseSession, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite: noiseSuite,
		Pattern:     noise.HandshakeNX,
		Initiator:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("security: init noise initiator: %w", err)
	}

	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("security: build noise initiation: %w", err)
	}
	if err := write(msg1); err != nil {
		return nil, fmt.Errorf("security: write noise initiation: %w", err)
	}

	msg2, err := read()
	if err != nil {
		return nil, fmt.Errorf("security: read noise response: %w", err)
	}
	if _, send, recv, err := hs.ReadMessage(nil, msg2); err != nil {
		return nil, fmt.Errorf("security: bad noise response: %w", err)
	} else {
		return &NoiseSession{send: send, recv: recv, remoteStatic: hs.PeerStatic()}, nil
	}
}
