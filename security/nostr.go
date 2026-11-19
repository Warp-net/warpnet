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
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"strconv"
)

const nostrKeyInfo = "warpnet/nostr/v1"

// secp256k1Order is n; a Nostr secret key must be in [1, n).
var secp256k1Order = []byte{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe,
	0xba, 0xae, 0xdc, 0xe6, 0xaf, 0x48, 0xa0, 0x3b, 0xbf, 0xd2, 0x5e, 0x8c, 0xd0, 0x36, 0x41, 0x41,
}

var ErrInvalidNodeKey = errors.New("nostr key: invalid node key")

// NostrKey derives the owner's Nostr (secp256k1) secret key from the node's
// ed25519 identity, so every node has its own stable npub.
func NostrKey(priv ed25519.PrivateKey) ([32]byte, error) {
	var sk [32]byte
	if len(priv) != ed25519.PrivateKeySize {
		return sk, ErrInvalidNodeKey
	}
	zero := make([]byte, len(sk))
	for counter := range 256 {
		info := nostrKeyInfo
		if counter > 0 {
			info += "/" + strconv.Itoa(counter)
		}
		out, err := hkdf.Key(sha256.New, priv.Seed(), nil, info, len(sk))
		if err != nil {
			return sk, err
		}
		if bytes.Compare(out, secp256k1Order) < 0 && !bytes.Equal(out, zero) {
			copy(sk[:], out)
			return sk, nil
		}
	}
	return sk, ErrInvalidNodeKey
}
