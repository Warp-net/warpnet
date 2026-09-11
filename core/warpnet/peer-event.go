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

package warpnet

// PeerEventType names something one node observed another node do on the
// wire. The vocabulary is the observation only: what it is worth, and on
// which axis, is decided by core/rating.
type PeerEventType string

const (
	// wire behaviour, observable by every node type
	PeerBadSignature       PeerEventType = "bad_signature"
	PeerMissingSignature   PeerEventType = "missing_signature"
	PeerMalformedFrame     PeerEventType = "malformed_frame"
	PeerOversizePayload    PeerEventType = "oversize_payload"
	PeerStaleMessage       PeerEventType = "stale_or_replayed"
	PeerPrivateRouteDenied PeerEventType = "private_route_denied"
	PeerRateLimited        PeerEventType = "rate_limit_hit"
	PeerDialFailure        PeerEventType = "dial_failure"

	// plain facts: an offence only in numbers, which the rating counts
	PeerConnected  PeerEventType = "connected"
	PeerDiscovered PeerEventType = "discovered"

	// what a member node observes about another member
	PeerModerationUpheld  PeerEventType = "moderation_upheld"
	PeerForeignAuthorship PeerEventType = "foreign_authorship"

	// what any node observes about a moderator
	PeerVerdictMalformed PeerEventType = "verdict_malformed"

	// what a moderator observes about another moderator
	PeerAuditWrong       PeerEventType = "audit_wrong"
	PeerAuditInvalid     PeerEventType = "audit_invalid"
	PeerAuditUnreachable PeerEventType = "audit_unreachable"
)

// PeerEvent is one observation about one peer.
type PeerEvent struct {
	PeerID string
	Type   PeerEventType
	Route  string // the route it happened on, where there is one
}

// peerEventsBuffer bounds a fan-out; past it an observation is dropped.
const peerEventsBuffer = 256

// PeerEmitter is a module's fan-out of what it saw its peers do. Whoever
// rates them reads it; nobody waits for that reader.
type PeerEmitter chan PeerEvent

func NewPeerEmitter() PeerEmitter {
	return make(PeerEmitter, peerEventsBuffer)
}

// Emit reports one observation, and drops it when the reader is behind:
// no request, stream or audit is ever held up for a rating.
func (e PeerEmitter) Emit(ev PeerEvent) {
	if e == nil || ev.PeerID == "" {
		return
	}
	select {
	case e <- ev:
	default:
	}
}
