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

package rating

// Kind is one observable offence. The numeric values are wire format:
// they appear in signed records, so they may be appended to but never
// renumbered.
type Kind uint16

const (
	// network — observable by every node type
	KindBadSignature Kind = iota + 1
	KindMissingSignature
	KindMalformedFrame
	KindOversizePayload
	KindStaleOrReplayed
	KindPrivateRouteDenied
	KindRateLimitHit
	KindDiscoveryFlood
	KindConnectionFlap
	KindDialFailure
	KindForgedObservation

	// application — member nodes
	KindModerationUpheld
	KindForeignAuthorship
	KindWriteFlood
	KindFalseReportBurst

	// moderation — moderator nodes
	KindVerdictBadSignature
	KindVerdictNoModeratorID
	KindVerdictMalformed
	KindVerdictUnsolicited
	KindVerdictOutlier
	KindAuditWrong
	KindAuditInvalid
	KindAuditUnreachable
)

type offence struct {
	name   string
	dim    Dimension
	weight int32
	// ceiling caps the decayed contribution of this kind within one
	// (subject, observer, dimension). Zero means no ceiling. Kinds
	// that can fire for reasons outside the subject's control — a
	// flaky link, a busy moment — carry one, so they can never on
	// their own push a peer out of BandWatched.
	ceiling int32
}

//nolint:gochecknoglobals // a lookup table, not state
var catalogue = map[Kind]offence{
	// Deliberate and self-evident: these are the network weights that
	// can reach BandFloor, and only ever from first-hand evidence.
	KindBadSignature:       {"bad_signature", Network, 250, 0},
	KindMissingSignature:   {"missing_signature", Network, 250, 0},
	KindPrivateRouteDenied: {"private_route_denied", Network, 200, 0},
	KindOversizePayload:    {"oversize_payload", Network, 150, 0},
	KindMalformedFrame:     {"malformed_frame", Network, 120, 0},
	KindStaleOrReplayed:    {"stale_or_replayed", Network, 120, 0},
	KindForgedObservation:  {"forged_observation", Network, 400, 0},
	// Pressure signals: cheap individually, capped in aggregate.
	KindRateLimitHit:   {"rate_limit_hit", Network, 15, 300},
	KindDiscoveryFlood: {"discovery_flood", Network, 25, 300},
	KindConnectionFlap: {"connection_flap", Network, 10, 200},
	KindDialFailure:    {"dial_failure", Network, 2, 100},

	KindModerationUpheld:  {"moderation_upheld", Application, 300, 0},
	KindForeignAuthorship: {"foreign_authorship", Application, 350, 0},
	KindWriteFlood:        {"write_flood", Application, 20, 300},
	KindFalseReportBurst:  {"false_report_burst", Application, 60, 300},

	KindVerdictBadSignature:  {"verdict_bad_signature", Moderation, 500, 0},
	KindVerdictNoModeratorID: {"verdict_no_moderator_id", Moderation, 500, 0},
	KindAuditInvalid:         {"audit_invalid", Moderation, 500, 0},
	KindVerdictUnsolicited:   {"verdict_unsolicited", Moderation, 250, 0},
	KindVerdictMalformed:     {"verdict_malformed", Moderation, 200, 0},
	// Honest model diversity produces disagreement, so an outlier
	// ballot is cheap and capped. See gap (2) in
	// cmd/node/moderator/audit/doc.go.
	KindVerdictOutlier:   {"verdict_outlier", Moderation, 60, 400},
	KindAuditWrong:       {"audit_wrong", Moderation, 60, 400},
	KindAuditUnreachable: {"audit_unreachable", Moderation, 5, 100},
}

//nolint:gochecknoglobals // derived from catalogue at init
var kindsByName = func() map[string]Kind {
	m := make(map[string]Kind, len(catalogue))
	for k, o := range catalogue {
		m[o.name] = k
	}
	return m
}()

func (k Kind) Valid() bool {
	_, ok := catalogue[k]
	return ok
}

func (k Kind) Dimension() Dimension { return catalogue[k].dim }
func (k Kind) Weight() int32        { return catalogue[k].weight }
func (k Kind) Ceiling() int32       { return catalogue[k].ceiling }

func (k Kind) String() string {
	if o, ok := catalogue[k]; ok {
		return o.name
	}
	return "unknown"
}

func KindByName(s string) (Kind, bool) {
	k, ok := kindsByName[s]
	return k, ok
}
