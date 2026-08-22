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
	"crypto/ed25519"
	"strings"
)

const (
	identityKeyContext = "warpnet/kdf/v1/identity-key"
	databaseKeyContext = "warpnet/kdf/v1/database-key"
)

func DeriveIdentityKey(username, password, network string) (ed25519.PrivateKey, error) {
	if password == "" {
		return nil, ErrEmptyPassword
	}
	seed := deriveKey([]byte(password), derivationSalt(identityKeyContext, network, username))
	defer Wipe(seed)

	return GenerateKeyFromSeed(seed)
}

func DeriveDatabaseKey(username, password string) ([]byte, error) {
	if password == "" {
		return nil, ErrEmptyPassword
	}
	return deriveKey([]byte(password), derivationSalt(databaseKeyContext, username)), nil
}

func derivationSalt(parts ...string) []byte {
	return ConvertToSHA256([]byte(strings.Join(parts, "\x00")))
}
