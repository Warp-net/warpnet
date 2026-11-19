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

package handler

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

const (
	ErrNostrKeyNotGateway = warpnet.WarpError("nostr key: only the gateway may ask")
	ErrNostrNotAllowed    = warpnet.WarpError("nostr key: the owner did not allow Nostr")
	ErrNostrNotOwner      = warpnet.WarpError("nostr key: not this node's owner")

	nostrConsentKey   = "nostr"
	nostrConsentAllow = "allow"
)

type NostrKeyUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

type nostrKeyHandler struct {
	authRepo UserAuthStorer
	userRepo NostrKeyUserFetcher
	privKey  ed25519.PrivateKey
}

// StreamGetNostrKeyHandler hands the owner's Nostr key to the gateway, which
// signs the owner's Nostr events with it and keeps nothing.
func StreamGetNostrKeyHandler(
	authRepo UserAuthStorer,
	userRepo NostrKeyUserFetcher,
	privKey ed25519.PrivateKey,
) warpnet.WarpHandlerFunc {
	return nostrKeyHandler{authRepo: authRepo, userRepo: userRepo, privKey: privKey}.handle
}

func (h nostrKeyHandler) handle(buf []byte, s warpnet.WarpStream) (any, error) {
	if s.Conn() == nil || s.Conn().RemotePeer().String() != fediverse.GatewayNodeID() {
		return nil, ErrNostrKeyNotGateway
	}
	var ev event.GetNostrKeyEvent
	if err := json.Unmarshal(buf, &ev); err != nil {
		return nil, fmt.Errorf("nostr key: unmarshal: %w", err)
	}
	ownerID := h.authRepo.GetOwner().UserId
	if ev.UserId != ownerID {
		return nil, ErrNostrNotOwner
	}
	u, err := h.userRepo.Get(ownerID)
	if err != nil {
		return nil, fmt.Errorf("nostr key: owner: %w", err)
	}
	if u.Metadata[nostrConsentKey] != nostrConsentAllow {
		return nil, ErrNostrNotAllowed
	}
	sk, err := security.NostrKey(h.privKey)
	if err != nil {
		return nil, err
	}
	return event.GetNostrKeyResponse{UserId: ownerID, Key: hex.EncodeToString(sk[:])}, nil
}
