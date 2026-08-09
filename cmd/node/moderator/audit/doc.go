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

// Package audit lets moderators spot-check each other. It is wired: the
// moderator answers challenges on ChallengeRoute, spot-checks a random peer
// every few minutes, and files every decided round as reference material.
// One thing is deliberately NOT wired — no verdict, vote or connection is
// refused because of a Ledger standing. See "What is missing" below for
// why that step needs more than this package can prove on its own.
//
// The problem it targets: the vote round (see the moderator package) stops a
// forged verdict, but a moderator that actually joins the network and votes
// dishonestly — or a bot that "votes" without running any model at all — is
// only outvoted, never identified. Audit closes that: any moderator may pick
// a random peer and hand it a spot-check ("challenge"): moderate exactly this
// text. The signed answer is compared against the probe's expected class and
// accumulated per peer in a Ledger; a peer whose answers are systematically
// wrong, absent or cryptographically invalid loses standing down to Banned.
//
// Three constraints shape everything here:
//
//   - Challenges must not be predictable. An earlier draft shipped a list
//     of invented probe texts, which is worthless: the source is public, so
//     anyone can tabulate "this text means unsafe" and answer correctly
//     with no model at all. Challenges are therefore drawn from Corpus —
//     real texts a vote round already ruled on, with the quorum's verdict
//     as ground truth. That pool is unique to each node, changes as traffic
//     does, and costs nothing to build, since the round already fetched the
//     text and already agreed on the answer.
//
//   - Verdict CLASSES are compared, never bytes or scores, and only
//     statistically: a single disagreement is noise, a systematic pattern
//     is a fake. The thresholds in ledger.go encode that tolerance. The
//     current tolerance assumes every moderator runs the same model — see
//     "What is missing" below.
//
//   - Cross-platform: no float comparisons, no runtime assumptions. The
//     challenge binds to hex(sha256) over the raw UTF-8 bytes of the text,
//     which is identical on every architecture, and the response is a signed
//     JSON event like every other Warpnet wire message.
//
// # What is missing for a sound fake-moderator test
//
// Under the same-model assumption an audit catches a peer with no model at
// all (constant answers, coin flips, silence) and a peer answering under
// someone else's identity. That is worth having, and it is all this package
// claims. Everything below stands between that and a verdict trustworthy
// enough to disqualify anyone — which is exactly why no vote, verdict or
// connection is currently refused on a Ledger standing.
//
//  1. A single auditor must not judge. A standing here is one node's
//     opinion, formed from references only it has seen. Acting on it lets a
//     malicious auditor disqualify honest moderators — a worse attack than
//     the one being defended against. A ban has to come from independent
//     auditors agreeing, which means gossiping the signed
//     challenge/response transcripts and deciding by quorum, the same shape
//     the vote round already uses for content.
//
//  2. Model identity has to be established, not assumed. The moment
//     moderators legitimately differ (a newer Llama Guard, another guard
//     model, a different quantization), a class disagreement no longer
//     separates a liar from an honest peer. Options, roughly by cost: pin a
//     model version per network epoch and make the version part of the
//     protocol; or have peers attest what they run and calibrate the
//     tolerated disagreement per model pair from observed rates; or
//     restrict challenges to references the network agreed on unanimously,
//     where model differences matter least.
//
//  3. References must be beyond dispute. Corpus trusts the quorum, so a
//     round decided by a colluding majority poisons every reference it
//     produces. Weighting references by how lopsided the round was, and
//     dropping those carried by a bare majority, limits the damage without
//     removing it.
//
//  4. A correct answer is not proof of work done. A peer can relay the text
//     to a third party, or recognise a reference it already holds in its
//     own corpus. Nothing here separates "runs a model" from "can obtain an
//     answer"; only economic or latency-bound arguments do, and both are
//     weak.
//
//  5. Identity still costs nothing. Audit measures behaviour per key, so a
//     banned peer returns under a fresh one. Standing starts at Probation
//     for that reason, but a probation period raises the price of a new
//     identity without setting one. Absent a real cost (stake, proof of
//     work, a vouching web), a patient attacker outlasts any behavioural
//     test.
//
// Until (1) and (2) hold, the honest reading of a Banned standing is "I
// should not rely on this node", not "this node is a fake".
package audit
