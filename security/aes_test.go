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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
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
func TestAESKeyFromPassword_IsStableAndDistinct(t *testing.T) {
	a := AESKeyFromPassword("hunter2")
	b := AESKeyFromPassword("hunter2")
	c := AESKeyFromPassword("hunter3")

	assert.Len(t, a, 32, "AES-256 needs a 32-byte key")
	assert.Equal(t, a, b, "the same dashboard password must derive the same key")
	assert.NotEqual(t, a, c, "a different password must not collide")

	assert.Len(t, AESKeyFromPassword(""), 32)
}

func TestAESCodec_RoundTrip(t *testing.T) {
	codec := AESCodec{Key: AESKeyFromPassword("dashboard-secret")}

	plain := []byte(`{"text":"привет 🔥","private":true}`)

	sealed, err := codec.Encode(plain, true)
	require.NoError(t, err)
	assert.NotEqual(t, plain, sealed, "the payload must not travel in the clear")
	assert.NotContains(t, string(sealed), "привет")

	got, encrypted := codec.Decode(sealed)
	assert.True(t, encrypted)
	assert.Equal(t, plain, got)
}

func TestAESCodec_NonceIsFresh(t *testing.T) {
	codec := AESCodec{Key: AESKeyFromPassword("secret")}
	plain := []byte("same message")

	first, err := codec.Encode(plain, true)
	require.NoError(t, err)
	second, err := codec.Encode(plain, true)
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "ciphertexts must not repeat for identical input")

	one, ok := codec.Decode(first)
	assert.True(t, ok)
	two, ok := codec.Decode(second)
	assert.True(t, ok)
	assert.Equal(t, one, two)
}

func TestAESCodec_NoKeyIsPassthrough(t *testing.T) {
	codec := AESCodec{}
	payload := []byte("plain text")

	out, err := codec.Encode(payload, true)
	require.NoError(t, err)
	assert.Equal(t, payload, out)

	got, encrypted := codec.Decode(payload)
	assert.Equal(t, payload, got)
	assert.False(t, encrypted, "an unkeyed codec must never claim a frame was encrypted")
}

func TestAESCodec_EncodeRespectsEncryptedFlag(t *testing.T) {
	codec := AESCodec{Key: AESKeyFromPassword("secret")}
	payload := []byte("public notice")

	out, err := codec.Encode(payload, false)
	require.NoError(t, err)
	assert.Equal(t, payload, out, "a frame marked plain must go out plain")
}

func TestAESCodec_WrongKeyDoesNotDecrypt(t *testing.T) {
	sender := AESCodec{Key: AESKeyFromPassword("correct-password")}
	attacker := AESCodec{Key: AESKeyFromPassword("guessed-password")}

	plain := []byte("session token: abc123")
	sealed, err := sender.Encode(plain, true)
	require.NoError(t, err)

	got, encrypted := attacker.Decode(sealed)
	assert.False(t, encrypted, "a wrong key must never report a successful decrypt")
	assert.NotEqual(t, plain, got)
	assert.Equal(t, sealed, got, "the frame is handed back untouched")
}

func TestAESCodec_TamperedCiphertextIsRejected(t *testing.T) {
	codec := AESCodec{Key: AESKeyFromPassword("secret")}

	sealed, err := codec.Encode([]byte("transfer 10 to alice"), true)
	require.NoError(t, err)

	tampered := bytes.Clone(sealed)
	idx := len(tampered) / 2
	if tampered[idx] == 'A' {
		tampered[idx] = 'B'
	} else {
		tampered[idx] = 'A'
	}

	got, encrypted := codec.Decode(tampered)
	assert.False(t, encrypted, "a tampered frame must not authenticate")
	assert.Equal(t, tampered, got)
}

func TestAESCodec_MalformedFramesAreRejectedNotPanicking(t *testing.T) {
	codec := AESCodec{Key: AESKeyFromPassword("secret")}

	frames := [][]byte{
		nil,
		{},
		[]byte("not base64 at all !!!"),
		[]byte("AAAA"),                       // valid base64, far too short for a nonce
		[]byte(strings.Repeat("A", 16)),      // still shorter than nonce+tag
		[]byte("////"),                       // valid base64, junk bytes
		[]byte(strings.Repeat("QUJD", 1000)), // long but meaningless
	}

	for _, frame := range frames {
		assert.NotPanics(t, func() {
			got, encrypted := codec.Decode(frame)
			assert.False(t, encrypted)
			assert.Equal(t, frame, got)
		})
	}
}

func TestAESCodec_EmptyPayloadRoundTrips(t *testing.T) {
	codec := AESCodec{Key: AESKeyFromPassword("secret")}

	sealed, err := codec.Encode([]byte{}, true)
	require.NoError(t, err)
	assert.NotEmpty(t, sealed, "even an empty body carries a nonce and tag")

	got, encrypted := codec.Decode(sealed)
	assert.True(t, encrypted)
	assert.Empty(t, got)
}

func TestAESCodec_LargePayloadRoundTrips(t *testing.T) {
	codec := AESCodec{Key: AESKeyFromPassword("secret")}

	plain := bytes.Repeat([]byte("warpnet"), 100_000)
	sealed, err := codec.Encode(plain, true)
	require.NoError(t, err)

	got, encrypted := codec.Decode(sealed)
	assert.True(t, encrypted)
	assert.Equal(t, plain, got)
}

func TestAESGCM_RejectsInvalidKeySizes(t *testing.T) {
	for _, size := range []int{0, 1, 15, 17, 31, 33, 64} {
		_, err := aesGCMEncrypt(make([]byte, size), []byte("x"))
		assert.Errorf(t, err, "key size %d must be rejected", size)

		_, err = aesGCMDecrypt(make([]byte, size), []byte("QUJD"))
		assert.Errorf(t, err, "key size %d must be rejected", size)
	}
}

func TestAESGCM_ShortCiphertextIsReported(t *testing.T) {
	key := AESKeyFromPassword("secret")

	_, err := aesGCMDecrypt(key, []byte("QUJD"))
	assert.ErrorIs(t, err, ErrCiphertextTooShort)
}
