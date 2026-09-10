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

// Kind is one observable offence.
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
	KindForgedRecord

	// application — member nodes
	KindModerationUpheld
	KindForeignAuthorship
	KindWriteFlood
	KindFalseReportBurst

	// moderation — moderator nodes
	KindVerdictMalformed
	KindVerdictOutlier
	KindAuditWrong
	KindAuditInvalid
	KindAuditUnreachable
)

type offence struct {
	name    string
	dim     Dimension
	weight  int32
	ceiling int32 // the most this kind may cost in one penalty; 0 is uncapped
}

var catalogue = map[Kind]offence{
	KindBadSignature:       {"bad_signature", Network, 250, 0},
	KindMissingSignature:   {"missing_signature", Network, 250, 0},
	KindPrivateRouteDenied: {"private_route_denied", Network, 200, 0},
	KindOversizePayload:    {"oversize_payload", Network, 150, 0},
	KindMalformedFrame:     {"malformed_frame", Network, 120, 0},
	KindStaleOrReplayed:    {"stale_or_replayed", Network, 120, 0},
	KindForgedRecord:       {"forged_observation", Network, 400, 0},
	// Pressure signals: cheap individually, capped in aggregate.
	KindRateLimitHit:   {"rate_limit_hit", Network, 15, 300},
	KindDiscoveryFlood: {"discovery_flood", Network, 25, 300},
	KindConnectionFlap: {"connection_flap", Network, 10, 200},
	KindDialFailure:    {"dial_failure", Network, 2, 100},

	KindModerationUpheld:  {"moderation_upheld", Application, 300, 0},
	KindForeignAuthorship: {"foreign_authorship", Application, 350, 0},
	KindWriteFlood:        {"write_flood", Application, 20, 300},
	KindFalseReportBurst:  {"false_report_burst", Application, 60, 300},

	KindAuditInvalid:     {"audit_invalid", Moderation, 500, 0},
	KindVerdictMalformed: {"verdict_malformed", Moderation, 200, 0},
	KindVerdictOutlier:   {"verdict_outlier", Moderation, 60, 400},
	KindAuditWrong:       {"audit_wrong", Moderation, 60, 400},
	KindAuditUnreachable: {"audit_unreachable", Moderation, 5, 100},
}

// Valid reports whether k is in the catalogue.
func (k Kind) Valid() bool {
	_, ok := catalogue[k]
	return ok
}

// Dimension is the axis this kind is witnessed on.
func (k Kind) Dimension() Dimension { return catalogue[k].dim }

// Weight is the penalty one occurrence costs before decay.
func (k Kind) Weight() int32 { return catalogue[k].weight }

// Ceiling is the most this kind may cost in one penalty; zero means uncapped.
func (k Kind) Ceiling() int32 { return catalogue[k].ceiling }

// String is the stable wire name of the kind.
func (k Kind) String() string {
	if o, ok := catalogue[k]; ok {
		return o.name
	}
	return unknownName
}

// ParseKind resolves a kind from its wire name.
func ParseKind(name string) (Kind, bool) {
	for k, o := range catalogue {
		if o.name == name {
			return k, true
		}
	}
	return 0, false
}
