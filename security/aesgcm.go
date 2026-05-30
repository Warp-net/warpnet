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
)

// ErrCiphertextTooShort is returned when a sealed blob is shorter than the GCM
// nonce it must carry.
var ErrCiphertextTooShort = errors.New("security: ciphertext too short")

// AESKeyFromPassword derives a 32-byte AES-256 key from a password. Unlike the
// weak EncryptAES helper, the functions below use a fresh random nonce per
// message, so they are safe for a long-lived channel.
func AESKeyFromPassword(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return sum[:]
}

// AESGCMEncrypt seals plaintext as base64(nonce || ciphertext+tag). The layout
// matches the browser's WebCrypto AES-GCM so the two interoperate.
func AESGCMEncrypt(key, plaintext []byte) ([]byte, error) {
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

// AESGCMDecrypt reverses AESGCMEncrypt.
func AESGCMDecrypt(key, sealed []byte) ([]byte, error) {
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
