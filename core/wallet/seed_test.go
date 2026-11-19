// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package wallet

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveSeed(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))

	t.Run("the seed is derived from the owner's login", func(t *testing.T) {
		seed, err := DeriveSeed(domain.Owner{Login: "alice"}, key, "testnet")
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(security.DeriveWalletSeed(key, "testnet", "alice")), seed)

		again, err := DeriveSeed(domain.Owner{Login: "alice"}, key, "testnet")
		require.NoError(t, err)
		assert.Equal(t, seed, again, "the same login always opens the same wallet")
	})

	t.Run("another login opens another wallet", func(t *testing.T) {
		alice, err := DeriveSeed(domain.Owner{Login: "alice"}, key, "testnet")
		require.NoError(t, err)
		bob, err := DeriveSeed(domain.Owner{Login: "bob"}, key, "testnet")
		require.NoError(t, err)
		assert.NotEqual(t, alice, bob)
	})

	t.Run("without a login or a key there is no wallet", func(t *testing.T) {
		_, err := DeriveSeed(domain.Owner{UserId: "u1"}, key, "testnet")
		require.Error(t, err)
		_, err = DeriveSeed(domain.Owner{Login: "alice"}, nil, "testnet")
		require.Error(t, err)
	})
}
