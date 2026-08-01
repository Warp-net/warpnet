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

//nolint:all
package security

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAESEncryptDecrypt_Success(t *testing.T) {
	password := []byte("SuperSecretPassword123!")
	in := []byte("Hello, this is a secret message.")

	cipherData, err := EncryptAES(in, password)
	assert.NoError(t, err)

	out, err := decryptAES(cipherData, password)
	assert.NoError(t, err)

	assert.Equal(t, in, out)
}

func TestAESEncryptDecrypt_WrongPassword(t *testing.T) {
	in := []byte("Hello, this is a secret message.")

	cipherData, err := EncryptAES(in, []byte("right"))
	assert.NoError(t, err)

	_, err = decryptAES(cipherData, []byte("wrong"))
	assert.Error(t, err)
}

// The nil-password branch is the one media uploads actually use
// (core/handler/image.go). It was previously untested.
func TestAESEncrypt_WeakPasswordBranch(t *testing.T) {
	in := []byte(`{"MAC":"3c:52:82:1a:9b:04"}`)

	sealed, err := EncryptAES(in, nil)
	assert.NoError(t, err)

	// Salt and nonce are public and embedded, per the scheme's design.
	assert.Len(t, sealed, saltSize+nonceSize+len(in)+tagSize)

	salt, nonce := sealed[:saltSize], sealed[saltSize:saltSize+nonceSize]
	assert.NotEqual(t, make([]byte, saltSize), salt, "salt must not be all-zero")
	assert.NotEqual(t, make([]byte, nonceSize), nonce, "nonce must not be all-zero")

	// The plaintext must not survive anywhere in the output.
	assert.False(t, bytes.Contains(sealed, in))
}

// Regression: the key used to come from a shuffled time.Now().Unix() with a
// fixed all-zero nonce, so two calls in the same second reused the same
// (key, nonce) pair and the whole thing was brute-forceable in milliseconds.
func TestAESEncrypt_WeakPasswordIsNotDerivedFromClock(t *testing.T) {
	in := []byte("same plaintext, same second")

	first, err := EncryptAES(in, nil)
	assert.NoError(t, err)

	second, err := EncryptAES(in, nil)
	assert.NoError(t, err)

	assert.NotEqual(t, first[:saltSize], second[:saltSize], "salt must be per-call random")
	assert.NotEqual(t,
		first[saltSize:saltSize+nonceSize], second[saltSize:saltSize+nonceSize],
		"nonce must be per-call random",
	)
	assert.NotEqual(t, first[saltSize+nonceSize:], second[saltSize+nonceSize:],
		"identical plaintext must not produce identical ciphertext",
	)
}

func TestAESDecrypt_TooShort(t *testing.T) {
	_, err := decryptAES(make([]byte, saltSize-1), []byte("pw"))
	assert.ErrorIs(t, err, ErrCiphertextTooShort)

	_, err = decryptAES(make([]byte, saltSize+nonceSize-1), []byte("pw"))
	assert.ErrorIs(t, err, ErrCiphertextTooShort)
}
