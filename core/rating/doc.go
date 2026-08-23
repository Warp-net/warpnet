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

// Package rating gives every node a standing that its neighbours
// compute, store and replicate — never the node itself.
//
// The shape of it, and why each piece is the way it is:
//
//   - Three axes (rating.go). A relay can only witness wire behaviour;
//     a member node also sees application-level misbehaviour; a
//     moderator is additionally judged on the verdicts it casts. A node
//     writes only the axes its own role can witness and a subject's
//     overall score is the worst of its axes, so a moderator that is
//     clean on the wire and forges verdicts is not a mostly-fine node.
//
//   - Observations, not opinions (record.go). Every record says "I,
//     this observer, saw these offences from this subject in this hour",
//     signed by the observer and verifiable against the pubkey derived
//     from its peer id. Subject == Observer is refused on both the write
//     and the read side, so a node cannot rate itself by construction.
//
//   - Generation-tagged keys (record.go, store.go). Each process start
//     mints a nonce that tags every key it writes, and readers sum
//     across generations. This is what makes the CRDT worth having: a
//     relay holds no disk, so a restart comes back with an empty
//     datastore and starts observing immediately. Without a generation
//     its first write of the current bucket would land on the key the
//     DAG is still replaying and last-write-wins would destroy the very
//     history the CRDT exists to restore.
//
//   - Decay at read time (aggregate.go). A record's contribution halves
//     every half-life, computed from its bucket when the score is read.
//     No sweeper, no rewrite traffic, and every node computes the same
//     number from the same records. Ratings heal on their own.
//
//   - Two numbers (aggregate.go). The subjective score — first-hand
//     evidence at full weight, remote observers discounted by how much
//     we trust them and capped twice — is the only one that enforces.
//     The public score is an unweighted median for display, and touches
//     nothing.
//
//   - A ceiling on slander (aggregate.go, CapRemoteTotal). Remote
//     entries together can never subtract more than 400, so they
//     cannot push anyone below the bottom of BandWatched. Degrading a
//     peer past that takes evidence gathered on our own wire. A
//     coordinated slander campaign therefore costs an honest node a
//     mild priority drop and nothing else.
//
//   - No storage here (store.go, Opener). This package holds the
//     model; the store it runs on is built in core/crdt/rating.go and
//     reaches this package through the Opener callback. The dependency
//     runs core/crdt → core/rating and never back. That store is a
//     tenant of the node's one CRDT replica, sharing it with stat
//     counters under a key prefix of its own — a second replica would
//     only mean a second DAG and a second gossip topic replicating the
//     same node's data.
//
//   - Soft consequences only (enforce.go). Low standing costs
//     connection priority, gossipsub score, DHT routing-table presence
//     and rate-limit headroom. Nothing here refuses service and nothing
//     here blocklists: automatic blocklisting on a gossiped reputation
//     would let a slander campaign partition an honest node off the
//     network, which is worse than the attack being defended against.
//
// # What this does not protect against
//
// Stated plainly, in the spirit of cmd/node/moderator/audit/doc.go.
//
//  1. Identity is free. A node at BandFloor restarts under a new key at
//     MaxScore. Rating prices sustained abuse from one identity; it does
//     not price identity. Only a stake, proof of work or a vouching web
//     would.
//
//  2. Remote entries are advisory, therefore partly ignorable. The
//     caps that defeat slander also mean a real offender is fully
//     sanctioned only by the peers it actually attacked. That is the
//     trade, taken deliberately.
//
//  3. The observer weighting is circular. weightOf uses an
//     own-evidence-only score to keep the recursion one level deep and
//     terminating, but a large honest-looking clique can still move the
//     public aggregate. That aggregate is display-only for this reason.
//
//  4. Model diversity muddies the moderation axis. KindVerdictOutlier
//     is cheap and capped because gap (2) of audit/doc.go —
//     establishing which model a moderator runs rather than assuming —
//     is unsolved. Until it is, a moderator's standing may weight its
//     ballots and must never disqualify them.
//
//  5. A record proves an observer claimed something, not that it
//     happened. Nothing here reaches the standard needed to ban a node.
//
//  6. Restart recovery is only as good as the peers still holding the
//     data. The DAG can replay only what someone else kept; a node
//     restarting into an empty or partitioned network recovers nothing.
//
//  7. Blame for unsigned garbage is not attributable. A record whose
//     signature does not verify names an observer that may have had
//     nothing to do with it, so it can only be dropped, never charged.
//     A flood of such records is a bandwidth attack this package cannot
//     price.
package rating
