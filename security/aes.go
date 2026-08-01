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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/argon2"
)

var ErrCiphertextTooShort = errors.New("security: ciphertext too short")

// Sealed layout: salt || nonce || ciphertext || tag.
const (
	keySize   = 32
	saltSize  = 16
	nonceSize = 12 // crypto/cipher GCM standard
	tagSize   = 16

	// weakPasswordSpace bounds the single-use password. It is deliberately
	// small: the media-metadata scheme relies on brute force being possible
	// but expensive. A guess costs ~19-22 ms here, putting the expected
	// search near 12 days on a 1000-core cluster and ~5 years on an
	// eight-core laptop. Both figures track per-core speed, so treat them as
	// an order of magnitude rather than a promise.
	//
	// Tune the cost here, not in argonMemory: dropping the memory parameter
	// would hand the attacker back the GPU speed-up it exists to deny.
	weakPasswordSpace = 110_000_000_000 // ~2^36.7

	// weakPasswordSize is the fixed big-endian width the bounded password is
	// encoded to before it reaches the KDF.
	weakPasswordSize = 8

	// Argon2id cost. Memory dominates: 64 MiB per guess is what denies an
	// attacker the GPU/ASIC speed-up that makes a 2^40 search cheap.
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
)

func deriveKey(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, argonTime, argonMemory, argonThreads, keySize)
}

// EncryptAES seals plainData with AES-256-GCM under an Argon2id-derived key.
// A nil password means the caller wants the weak single-use password of the
// media-metadata scheme: one is generated here, used once and discarded, so
// the plaintext is recoverable by brute force alone. The random salt and nonce
// are public and travel with the ciphertext.
func EncryptAES(plainData, password []byte) ([]byte, error) {
	if password == nil {
		n, err := rand.Int(rand.Reader, big.NewInt(weakPasswordSpace))
		if err != nil {
			return nil, fmt.Errorf("failed to generate weak password: %w", err)
		}
		password = make([]byte, weakPasswordSize)
		binary.BigEndian.PutUint64(password, n.Uint64())
		defer func() {
			for i := range password { // never stored, never logged
				password[i] = 0
			}
		}()
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	key := deriveKey(password, salt)
	aesGCM, err := newAESGCM(key)
	for i := range key { // avoid RAM snapshot attack
		key[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	out := make([]byte, 0, saltSize+len(nonce)+len(plainData)+aesGCM.Overhead())
	out = append(out, salt...)
	out = append(out, nonce...)

	return aesGCM.Seal(out, nonce, plainData, nil), nil
}

func decryptAES(sealed, password []byte) ([]byte, error) {
	if len(sealed) < saltSize {
		return nil, ErrCiphertextTooShort
	}
	salt, rest := sealed[:saltSize], sealed[saltSize:]

	key := deriveKey(password, salt)
	aesGCM, err := newAESGCM(key)
	for i := range key { // avoid RAM snapshot attack
		key[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(rest) < aesGCM.NonceSize() {
		return nil, ErrCiphertextTooShort
	}
	nonce, ciphertext := rest[:aesGCM.NonceSize()], rest[aesGCM.NonceSize():]

	plain, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plain, nil
}

func AESKeyFromPassword(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return sum[:]
}

type AESCodec struct{ Key []byte }

func (c AESCodec) Decode(frame []byte) (plain []byte, encrypted bool) {
	if len(c.Key) == 0 {
		return frame, false
	}
	if p, err := aesGCMDecrypt(c.Key, frame); err == nil {
		return p, true
	}
	return frame, false
}

func (c AESCodec) Encode(reply []byte, encrypted bool) ([]byte, error) {
	if !encrypted || len(c.Key) == 0 {
		return reply, nil
	}
	return aesGCMEncrypt(c.Key, reply)
}

func aesGCMEncrypt(key, plaintext []byte) ([]byte, error) {
	gcm, err := newAESGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	out := make([]byte, base64.StdEncoding.EncodedLen(len(ct)))
	base64.StdEncoding.Encode(out, ct)
	return out, nil
}

func aesGCMDecrypt(key, sealed []byte) ([]byte, error) {
	data := make([]byte, base64.StdEncoding.DecodedLen(len(sealed)))
	n, err := base64.StdEncoding.Decode(data, sealed)
	if err != nil {
		return nil, err
	}
	data = data[:n]

	gcm, err := newAESGCM(key)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, ErrCiphertextTooShort
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

func newAESGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
