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

import (
	"errors"
	"strings"

	"github.com/Warp-net/warpnet/domain"
)

const (
	// Network is the User.Network tag for accounts bridged in from Mastodon.
	Network = "mastodon"

	// DefaultGatewayNodeID is the libp2p peer id of the ActivityPub gateway,
	// deterministically derived from its fixed seed. It is the home node of
	// every bridged Mastodon user, and the fallback when the owner has not
	// configured a different gateway in settings.
	DefaultGatewayNodeID = "12D3KooWRyHvpYFjCzorxuSyXFigPfhYaHh1GW1JmwQJSPdmj4JK"

	// EntryHandle is the single Mastodon account seeded locally as the entry
	// point into the Fediverse; its followings lead to other Mastodon accounts.
	EntryHandle = "warpnet@mastodon.social"
)

var ErrNotSupported = errors.New("not supported functionality")

// IsBridgedID reports whether an id names a bridged Fediverse entity: native
// Warpnet ids are ULIDs, bridged tweets travel under their status URL.
func IsBridgedID(id string) bool {
	return strings.HasPrefix(id, "https://") || strings.HasPrefix(id, "http://")
}

// BridgedStatusID extracts the trailing status id from a bridged status URL
// (".../statuses/<id>[?...]") — the shape the gateway publishes a federated
// Warpnet tweet under. It lets a node line such a copy up with the bare id in
// its own store; ok is false for native (non-URL) ids and for URLs without a
// status path.
func BridgedStatusID(id string) (string, bool) {
	if !IsBridgedID(id) {
		return "", false
	}
	id, _, _ = strings.Cut(id, "?")
	_, tail, found := strings.Cut(id, "/statuses/")
	if !found || tail == "" || strings.ContainsRune(tail, '/') {
		return "", false
	}
	return tail, true
}

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

// SeedEntryUser inserts the bridged Mastodon entry account so it is
// discoverable/searchable locally; opening it streams to the gateway node.
func SeedEntryUser(repo UserSeeder) {
	u := domain.User{
		Id:       EntryHandle,
		Username: "Warpnet",
		NodeId:   gatewayNodeID,
		Network:  Network,
	}
	if _, err := repo.Create(u); err != nil {
		_, _ = repo.Update(u.Id, u)
	}
}
