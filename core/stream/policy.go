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

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package stream

import (
	"time"

	"github.com/docker/go-units"
)

// Per-route transport policy.
//
// Only the shape and the defaults live here. Which routes deviate from those
// defaults is a property of the node that serves them — a member node hosts
// the media handlers, a relay does not — so the table itself is declared
// where the handlers are registered. Neither the transport nor the middleware
// that enforce these limits knows any concrete path.

const (
	// DefaultMaxInboundSize is the inbound ceiling for an ordinary request.
	DefaultMaxInboundSize = units.MiB * 5
	// DefaultIODeadline bounds a single request/response exchange.
	DefaultIODeadline = time.Minute
)

// RoutePolicy is the per-route transport budget. A zero field means "use the
// default", so a caller only states what actually differs.
type RoutePolicy struct {
	// MaxInboundSize is the largest request the route accepts, in bytes.
	MaxInboundSize int64
	// IODeadline is how long the exchange may take before it is abandoned.
	IODeadline time.Duration
}

// RoutePolicies maps routes to their budgets. The zero value (a nil map) is
// usable and yields defaults for every route.
type RoutePolicies map[WarpRoute]RoutePolicy

// For returns the policy for a route, substituting defaults for unset fields.
func (p RoutePolicies) For(r WarpRoute) RoutePolicy {
	policy := p[r]
	if policy.MaxInboundSize <= 0 {
		policy.MaxInboundSize = int64(DefaultMaxInboundSize)
	}
	if policy.IODeadline <= 0 {
		policy.IODeadline = DefaultIODeadline
	}
	return policy
}
