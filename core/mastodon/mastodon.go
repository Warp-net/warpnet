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

// Package mastodon holds everything the node needs for the Mastodon bridge.
// The node itself stays unaware of the ActivityPub gateway: this package only
// tags bridged users with a foreign network and seeds a single entry account
// whose home node is the gateway, so it resolves like any other remote user.
package mastodon

import "github.com/Warp-net/warpnet/domain"

const (
	// Network is the User.Network tag for accounts bridged in from Mastodon.
	Network = "mastodon"

	// GatewayNodeID is the libp2p peer id of the ActivityPub gateway,
	// deterministically derived from its fixed seed. It is the home node of
	// every bridged Mastodon user.
	GatewayNodeID = "12D3KooWRyHvpYFjCzorxuSyXFigPfhYaHh1GW1JmwQJSPdmj4JK"

	// EntryHandle is the single Mastodon account seeded locally as the entry
	// point into the Fediverse; its followings lead to other Mastodon accounts.
	EntryHandle = "warpnet@mastodon.social"
)

// UserSeeder is the subset of the user repository the seeding needs.
type UserSeeder interface {
	Create(user domain.User) (domain.User, error)
	Update(userId string, newUser domain.User) (domain.User, error)
}

// SeedEntryUser inserts the bridged Mastodon entry account so it is
// discoverable/searchable locally; opening it streams to the gateway node.
func SeedEntryUser(repo UserSeeder) {
	u := domain.User{
		Id:       EntryHandle,
		Username: "Warpnet",
		NodeId:   GatewayNodeID,
		Network:  Network,
	}
	if _, err := repo.Create(u); err != nil {
		_, _ = repo.Update(u.Id, u)
	}
}
