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

package audit

import (
	"crypto/ed25519"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
)

const ErrResponseSignature warpnet.WarpError = "audit: challenge response signature invalid"

// ChallengeRoute is the moderator-to-moderator audit spot-check route.
// Reserved for the audit protocol; no handler is registered for it yet.
const ChallengeRoute = "/public/get/moderate/challenge/0.0.0"

// Challenge is a spot-check one moderator sends to another:
// "moderate exactly this text". ContentHash is hex(sha256) over the raw
// UTF-8 bytes of Text, so the response binds to the same content on any
// platform — no float math, no runtime assumptions.
//
// NOT WIRED YET: no handler is registered for the challenge route; this is
// the wire shape for the future moderator-audit protocol. It lives in this
// package rather than the shared event package because only moderators
// exchange challenges — no member node ever sends or receives one.
type Challenge struct {
	ChallengeID string    `json:"challenge_id"`
	Text        string    `json:"text"`
	ContentHash string    `json:"content_hash"`
	TimeAt      time.Time `json:"time_at,omitzero"`
}

// ChallengeResponse is the challenged moderator's signed
// verdict on the probe text. Moderators are free to run different models
// (Model reports which one answered), so auditors must compare verdict
// classes statistically, never byte-for-byte.
type ChallengeResponse struct {
	ChallengeID string                  `json:"challenge_id"`
	ContentHash string                  `json:"content_hash"`
	Result      domain.ModerationResult `json:"result"`
	Reason      *string                 `json:"reason,omitempty"`
	Model       domain.ModelType        `json:"model"`
	ModeratorID domain.ID               `json:"moderator_id"`
	TimeAt      time.Time               `json:"time_at,omitzero"`
	// Signature is base64(ed25519) over SigningBytes with the responder's
	// node key; it makes the answer non-repudiable audit evidence.
	Signature string `json:"signature,omitempty"`
}

// SigningBytes returns the canonical bytes the response signature covers,
// length-prefixed like ModerationResultEvent.SigningBytes.
func (e ChallengeResponse) signingBytes() []byte {
	reason := ""
	if e.Reason != nil {
		reason = *e.Reason
	}
	parts := []string{
		e.ChallengeID,
		e.ContentHash,
		strconv.FormatBool(bool(e.Result)),
		reason,
		string(e.Model),
		string(e.ModeratorID),
		strconv.FormatInt(e.TimeAt.UnixNano(), 10),
	}
	buf := make([]byte, 0, 192)
	for _, p := range parts {
		buf = append(buf, strconv.Itoa(len(p))...)
		buf = append(buf, ':')
		buf = append(buf, p...)
	}
	return buf
}

// Signed returns a stamped and signed copy of the response, answering as
// moderator with the model that actually judged the text.
func (e ChallengeResponse) Signed(privKey ed25519.PrivateKey, moderator domain.ID, model domain.ModelType) ChallengeResponse {
	e.ModeratorID = moderator
	e.Model = model
	e.TimeAt = time.Now().UTC()
	if len(privKey) == 0 {
		return e
	}
	e.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privKey, e.signingBytes()))
	return e
}

// VerifiedFrom checks that the response really came from peer: the
// signature must verify against the public key carried by that peer id, so
// a valid answer signed by anyone else counts for nothing.
func (e ChallengeResponse) VerifiedFrom(peer string) error {
	if e.ModeratorID != peer {
		return ErrResponseSignature
	}
	peerID := warpnet.FromStringToPeerID(peer)
	if peerID == "" {
		return ErrResponseSignature
	}
	pubKey := warpnet.FromIDToPubKey(peerID)
	if len(pubKey) == 0 {
		return ErrResponseSignature
	}
	if err := security.VerifySignature(pubKey, e.signingBytes(), e.Signature); err != nil {
		return ErrResponseSignature
	}
	return nil
}
