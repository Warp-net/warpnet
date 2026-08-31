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
package node

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/require"
)

// The desktop app stops the node twice: logout (PRIVATE_POST_LOGOUT) stops it,
// then the wails shutdown hook (App.close) stops it again when the window
// closes. A second Stop must be a no-op instead of panicking on an
// already closed channel.
func TestStopIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(db.Close)

	authRepo := database.NewAuthRepo(db, "test")
	require.NoError(t, authRepo.Authenticate("test", "test"))

	privKey := authRepo.PrivateKey()
	ownNodeId, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	_, err = authRepo.SetOwner(domain.Owner{
		NodeId:   ownNodeId.String(),
		UserId:   ownNodeId.String(),
		Username: "test",
	})
	require.NoError(t, err)

	n, err := NewMemberNode(ctx, privKey, nil, ownNodeId, authRepo, db, nil)
	require.NoError(t, err)

	n.Stop()
	n.Stop()
}
