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
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/argon2"
)

var (
	ErrCiphertextTooShort = errors.New("security: ciphertext too short")
	ErrEmptyPassword      = errors.New("security: empty password")
)

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
// The random salt and nonce are public and travel with the ciphertext.
func EncryptAES(plainData, password []byte) ([]byte, error) {
	if len(password) == 0 {
		return nil, ErrEmptyPassword
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

func newAESGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// NewWeakPassword draws a single-use password from the bounded space above:
// small enough that brute force stays possible, expensive enough that it costs
// a data centre. Wipe returns the bytes to zero once they have been used.
func NewWeakPassword() ([]byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(weakPasswordSpace))
	if err != nil {
		return nil, fmt.Errorf("failed to generate weak password: %w", err)
	}

	password := make([]byte, weakPasswordSize)
	binary.BigEndian.PutUint64(password, n.Uint64())
	return password, nil
}

func Wipe(secret []byte) {
	for i := range secret {
		secret[i] = 0
	}
}
