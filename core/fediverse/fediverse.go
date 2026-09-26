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

// Package fediverse holds everything the node needs for the ActivityPub bridge.
// The node itself stays unaware of the gateway: this package only tags bridged
// users with a foreign network and seeds a single entry account whose home node
// is the gateway, so it resolves like any other remote user.
package fediverse

import (
	"errors"
	"github.com/Warp-net/warpnet/domain"
)

const (
	// MastodonNetwork is the User.Network tag for accounts bridged in from
	// Mastodon.
	MastodonNetwork = "mastodon"

	// DefaultGatewayNodeID is the libp2p peer id of the ActivityPub gateway,
	// deterministically derived from its fixed seed. It is the home node of
	// every bridged Mastodon user, and the fallback when the owner has not
	// configured a different gateway in settings.
	DefaultGatewayNodeID = "12D3KooWRyHvpYFjCzorxuSyXFigPfhYaHh1GW1JmwQJSPdmj4JK"

	// ThreadsNetwork is the User.Network tag for accounts bridged in from Meta's
	// Threads. They arrive through the same gateway and behave the same locally,
	// so a check for "bridged in from outside" must use IsBridged.
	ThreadsNetwork = "threads"

	// EntryHandle is the single Mastodon account seeded locally as the entry
	// point into the Fediverse; its followings lead to other Mastodon accounts.
	EntryHandle = "warpnet@mastodon.social"

	// ThreadsEntryHandle is the same thing for Threads. Threads serves no
	// account search and no browsable follow graph, so nothing discovers a
	// Threads account on its own: without a seeded one the Threads tab of the
	// recommendations has nothing to show and no way to ever get anything.
	ThreadsEntryHandle = "zuck@threads.net"
)

// IsBridged reports whether a User.Network tag names a network bridged in
// through the ActivityPub gateway rather than Warpnet itself.
func IsBridged(network string) bool {
	return network == MastodonNetwork || network == ThreadsNetwork
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
		{Id: ThreadsEntryHandle, Username: "Mark Zuckerberg", NodeId: gatewayNodeID, Network: ThreadsNetwork},
	} {
		if _, err := repo.Create(u); err != nil {
			_, _ = repo.Update(u.Id, u)
		}
	}
}
