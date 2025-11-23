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
	"bytes"
	go_crypto "crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/crypto/pb"
)

var ErrEmptySeed = errors.New("empty seed")

type PrivateKey crypto.PrivKey

func GenerateKeyFromSeed(seed []byte) (ed25519.PrivateKey, error) {
	if len(seed) == 0 {
		return nil, ErrEmptySeed
	}
	hashAlgo := go_crypto.SHA256
	keyType := pb.KeyType_Ed25519
	seed = append(seed, uint8(hashAlgo))
	seed = append(seed, uint8(keyType))
	hash := sha256.Sum256(seed)
	privKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(hash[:]))
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	return privKey.Raw()
}
