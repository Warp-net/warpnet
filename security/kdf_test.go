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
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testUser     = "alice"
	testPassword = "CorrectHorse1!"
	testNetwork  = "testnet"
)

func TestDeriveIdentityKey_IsDeterministic(t *testing.T) {
	first, err := DeriveIdentityKey(testUser, testPassword, testNetwork)
	require.NoError(t, err)
	second, err := DeriveIdentityKey(testUser, testPassword, testNetwork)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same credentials must rebuild the same peer ID")
	assert.Len(t, first, ed25519.PrivateKeySize)
}

func TestDeriveIdentityKey_SeparatesAccountsAndNetworks(t *testing.T) {
	base, err := DeriveIdentityKey(testUser, testPassword, testNetwork)
	require.NoError(t, err)

	otherUser, err := DeriveIdentityKey("bob", testPassword, testNetwork)
	require.NoError(t, err)
	assert.NotEqual(t, base, otherUser)

	otherNetwork, err := DeriveIdentityKey(testUser, testPassword, "mainnet")
	require.NoError(t, err)
	assert.NotEqual(t, base, otherNetwork)

	otherPassword, err := DeriveIdentityKey(testUser, testPassword+"x", testNetwork)
	require.NoError(t, err)
	assert.NotEqual(t, base, otherPassword)
}

func TestDeriveDatabaseKey_IsDeterministicAndSized(t *testing.T) {
	first, err := DeriveDatabaseKey(testUser, testPassword)
	require.NoError(t, err)
	second, err := DeriveDatabaseKey(testUser, testPassword)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Len(t, first, keySize, "badger needs a 32-byte AES key")

	otherUser, err := DeriveDatabaseKey("bob", testPassword)
	require.NoError(t, err)
	assert.NotEqual(t, first, otherUser)
}

func TestDerivedRootSecretsAreDomainSeparated(t *testing.T) {
	identity, err := DeriveIdentityKey(testUser, testPassword, testNetwork)
	require.NoError(t, err)
	dbKey, err := DeriveDatabaseKey(testUser, testPassword)
	require.NoError(t, err)

	assert.False(t, bytes.Contains(identity, dbKey))
	assert.NotEqual(t, derivationSalt(identityKeyContext, testNetwork, testUser),
		derivationSalt(databaseKeyContext, testUser))
}

func TestDerivationRunsThroughArgon2id(t *testing.T) {
	salt := derivationSalt(identityKeyContext, testNetwork, testUser)
	assert.Len(t, salt, keySize)

	assert.Equal(t,
		deriveKey([]byte(testPassword), salt),
		deriveKey([]byte(testPassword), salt),
	)
	assert.NotEqual(t,
		ConvertToSHA256([]byte(testUser+"@"+testPassword)),
		deriveKey([]byte(testPassword), salt),
	)
}

func TestDerivation_RefusesEmptyPassword(t *testing.T) {
	_, err := DeriveIdentityKey(testUser, "", testNetwork)
	assert.ErrorIs(t, err, ErrEmptyPassword)

	_, err = DeriveDatabaseKey(testUser, "")
	assert.ErrorIs(t, err, ErrEmptyPassword)
}
