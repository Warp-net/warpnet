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

// Package audit is the moderator-audit blueprint: NOT WIRED to anything yet.
// No production code constructs an Auditor, registers the challenge route or
// consults the Ledger — this package only fixes the design in compilable,
// tested form.
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
// Two constraints shape everything here:
//
//   - Moderators run DIFFERENT models — nothing is pinned. Byte-exact
//     recomputation therefore proves nothing; the probes are deliberately
//     flagrant (any competent moderation model agrees on their class), only
//     verdict CLASSES are compared, and only statistically: a single
//     disagreement is model noise, a systematic pattern is a fake. The
//     thresholds in ledger.go encode that tolerance.
//
//   - Cross-platform: no float comparisons, no runtime assumptions. The
//     challenge binds to hex(sha256) over the raw UTF-8 bytes of the text,
//     which is identical on every architecture, and the response is a signed
//     JSON event like every other Warpnet wire message.
//
// What a challenge still proves despite unpinned models: possession of a
// working moderation model (an unseen probe text cannot be classified
// without running inference) and honesty on unambiguous content. Probe
// templates are parameterized to raise the cost of memorizing answers;
// long-term the corpus is expected to rotate with releases and to be mixed
// with live samples the auditor's own model is confident about.
//
// Wiring plan (out of scope here): register StreamChallengeHandler under
// event.PUBLIC_GET_MODERATION_CHALLENGE on the moderator node, run an
// Auditor on a timer over the moderator's seen-peers set, and let the vote
// round consult Ledger.StandingOf when tallying (Banned voters excluded).
// The signed challenge/response transcripts are the evidence a future
// reputation gossip can carry.
package audit
