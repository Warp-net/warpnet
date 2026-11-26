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

// Package bridge holds everything the node needs for the gateway bridges.
// The node itself stays unaware of the gateway: this package only tags bridged
// users with a foreign network and seeds a single entry account whose home node
// is the gateway, so it resolves like any other remote user.
package bridge

import (
	"errors"
	"strings"

	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
)

const (
	// MastodonNetwork is the User.Network tag for accounts bridged in from
	// Mastodon.
	MastodonNetwork = "mastodon"

	// DefaultGatewayNodeID is the libp2p peer id of the official gateway,
	// derived from its secret signing key. It is the home node of every bridged
	// Mastodon user, and the fallback when the owner has not configured their
	// own gateway in settings. Only the effective gateway gets the node's
	// bridge key.
	DefaultGatewayNodeID = "12D3KooWKSVXZAF8dE2Cv449x2jkb1mCFqs7b9CV59Dpzqnur2N3"

	// ThreadsNetwork is the User.Network tag for accounts bridged in from Meta's
	// Threads. They arrive through the same gateway and behave the same locally,
	// so a check for "bridged in from outside" must use IsBridged.
	ThreadsNetwork = "threads"

	NostrNetwork = "nostr"

	// EntryHandle is the single Mastodon account seeded locally as the entry
	// point into the Fediverse; its followings lead to other Mastodon accounts.
	EntryHandle = "warpnet@mastodon.social"

	// ThreadsEntryHandle is the same thing for Threads. Threads serves no
	// account search and no browsable follow graph, so nothing discovers a
	// Threads account on its own: without a seeded one the Threads tab of the
	// recommendations has nothing to show and no way to ever get anything.
	ThreadsEntryHandle = "engineer_of_your_ass@threads.net"
)

// IsBridged reports whether a User.Network tag names a network other than this
// node's own Warpnet network. Records default to "warpnet" whatever the node's
// network, and "mainnet" is its legacy spelling, so both count as native.
func IsBridged(network string) bool {
	switch network {
	case "", warpnet.WarpnetName, "mainnet", "testnet", config.Config().Node.Network:
		return false
	}
	return true
}

func IsFederatedWarpnet(user domain.User) bool {
	name, host, ok := strings.Cut(strings.TrimPrefix(user.Id, "@"), "@")
	if !ok || host == "" {
		return false
	}
	_, err := ulid.ParseStrict(name)
	return err == nil
}

// IsBridgedUserID reports whether a user id is a handle on another network
// (name@host); a Warpnet user id is a bare ULID and never carries one.
func IsBridgedUserID(id string) bool {
	name, host, ok := strings.Cut(strings.TrimPrefix(id, "@"), "@")
	return ok && name != "" && host != ""
}

var ErrNotSupported = errors.New("not supported functionality")

// gatewayNodeID is the effective gateway peer id. It defaults to
// DefaultGatewayNodeID and is overridden once at node startup from the owner's
// settings (see SetGatewayNodeID); it is not mutated afterwards.
var gatewayNodeID = DefaultGatewayNodeID

// GatewayNodeID returns the effective ActivityPub gateway peer id.
func GatewayNodeID() string { return gatewayNodeID }

// SetGatewayNodeID overrides the effective gateway peer id from the owner's
// settings. Called once at startup, before seeding and discovery; an empty id
// is ignored so DefaultGatewayNodeID stands.
func SetGatewayNodeID(id string) {
	if id == "" {
		return
	}
	gatewayNodeID = id
}

// UserSeeder is the subset of the user repository the seeding needs.
type UserSeeder interface {
	Create(user domain.User) (domain.User, error)
	Update(userId string, newUser domain.User) (domain.User, error)
}

// SeedEntryUser inserts one bridged entry account per bridged network so each
// is discoverable/searchable locally; opening one streams to the gateway node,
// which resolves the live profile.
func SeedEntryUser(repo UserSeeder) {
	for _, u := range []domain.User{
		{Id: EntryHandle, Username: "Warpnet", NodeId: gatewayNodeID, Network: MastodonNetwork},
		{Id: ThreadsEntryHandle, Username: "engineer_of_your_ass", NodeId: gatewayNodeID, Network: ThreadsNetwork},
	} {
		if _, err := repo.Create(u); err != nil {
			_, _ = repo.Update(u.Id, u)
		}
	}
}
