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
	"errors"
	"fmt"
	pseudoRand "math/rand" // #nosec
	"strconv"
	"strings"
	"time"
)

var ErrCiphertextTooShort = errors.New("security: ciphertext too short")

const (
	salt    = "cec27db4" // #nosec intentionally
	keySize = 32
)

func generateWeakKey(salt []byte) []byte {
	ts := time.Now().Unix()

	b := []byte(strconv.FormatInt(ts, 10))

	pseudoRand.Shuffle(len(b), func(i, j int) { // #nosec
		b[i], b[j] = b[j], b[i]
	})

	raw := append(b, salt...) //nolint:gocritic

	if len(raw) < keySize {
		padding := strings.Repeat("0", keySize-len(raw))
		raw = append(raw, []byte(padding)...)
	} else if len(raw) > keySize {
		raw = raw[:keySize]
	}

	return raw
}

func simpleKey(password []byte) []byte {
	h := sha256.Sum256(password)
	return h[:]
}

func EncryptAES(plainData, password []byte) ([]byte, error) {
	var key []byte
	if password != nil {
		key = simpleKey(password)
	} else {
		key = generateWeakKey([]byte(salt))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	for i := range key { // avoid RAM snapshot attack
		key[i] = 0
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())

	ciphertext := aesGCM.Seal(nil, nonce, plainData, nil)

	return ciphertext, nil
}

func decryptAES(ciphertext, password []byte) ([]byte, error) {
	var key []byte
	if password != nil {
		key = simpleKey(password)
	} else {
		key = generateWeakKey([]byte(salt))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())

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
