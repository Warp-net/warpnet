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
	"strings"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
)

const (
	// MastodonNetwork is the User.Network tag for accounts bridged in from
	// Mastodon.
	MastodonNetwork = "mastodon"

	// DefaultGatewayNodeID is the libp2p peer id of the ActivityPub gateway,
	// deterministically derived from its secret seed. It is the home node of
	// every bridged Mastodon user, and the fallback when the owner has not
	// configured a different gateway in settings.
	DefaultGatewayNodeID = "NEW_GATEWAY_PEER_ID"
	// LegacyGatewayNodeID is the gateway id derived from its former public
	// seed: anyone can run a node with it, so it is never trusted and users
	// homed on it are moved to the current gateway (MigrateLegacyGatewayUsers).
	LegacyGatewayNodeID = "12D3KooWRyHvpYFjCzorxuSyXFigPfhYaHh1GW1JmwQJSPdmj4JK"

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

func IsFederatedWarpnet(user domain.User) bool {
	name, host, ok := strings.Cut(strings.TrimPrefix(user.Id, "@"), "@")
	if !ok || host == "" {
		return false
	}
	_, err := ulid.ParseStrict(name)
	return err == nil
}

var ErrNotSupported = errors.New("not supported functionality")

// gatewayNodeID is the effective gateway peer id. It defaults to
// DefaultGatewayNodeID and is overridden once at node startup from the owner's
// settings (see SetGatewayNodeID); it is not mutated afterwards.
var gatewayNodeID = DefaultGatewayNodeID

// GatewayNodeID returns the effective ActivityPub gateway peer id.
func GatewayNodeID() string { return gatewayNodeID }

// SetGatewayNodeID overrides the effective gateway peer id from the owner's
// settings. Called once at startup, before seeding and discovery; an empty or
// legacy id is ignored so DefaultGatewayNodeID stands.
func SetGatewayNodeID(id string) {
	if id == "" || id == LegacyGatewayNodeID {
		return
	}
	gatewayNodeID = id
}

// UserMigrator is the subset of the user repository the legacy migration needs.
type UserMigrator interface {
	List(limit *uint64, cursor *string) ([]domain.User, string, error)
	Update(userId string, newUser domain.User) (domain.User, error)
}

// MigrateLegacyGatewayUsers re-homes every stored user whose node is the
// legacy gateway id onto the effective gateway, so authorship checks stop
// trusting the publicly derivable key. Call it after SetGatewayNodeID.
func MigrateLegacyGatewayUsers(repo UserMigrator) (int, error) {
	limit := uint64(100)
	var (
		cursor string
		legacy []domain.User
	)
	for {
		users, cur, err := repo.List(&limit, &cursor)
		if err != nil {
			return 0, err
		}
		for _, u := range users {
			if u.NodeId == LegacyGatewayNodeID {
				legacy = append(legacy, u)
			}
		}
		if len(users) == 0 || cur == "" || cur == local_store.EndCursor || cur == cursor {
			break
		}
		cursor = cur
	}
	for _, u := range legacy {
		u.NodeId = gatewayNodeID
		if _, err := repo.Update(u.Id, u); err != nil {
			return 0, err
		}
	}
	return len(legacy), nil
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
