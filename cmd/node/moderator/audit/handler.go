// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"crypto/ed25519"
	"encoding/base64"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

const (
	ErrEmptyChallenge        warpnet.WarpError = "audit: empty challenge text"
	ErrChallengeHashMismatch warpnet.WarpError = "audit: challenge text does not match its content hash"
)

// Engine is the moderation engine slice the respondent side needs — the
// same shape the moderator package uses.
type Engine interface {
	Moderate(content string) (bool, string, error)
}

// ResponseSigner stamps identity, model and signature onto an outgoing
// challenge response.
type ResponseSigner func(*event.ModerationChallengeResponseEvent)

// NewResponseSigner builds the production signer: the responder's node key
// and the model it actually runs (self-reported — audits judge classes, not
// model claims).
func NewResponseSigner(privKey ed25519.PrivateKey, selfID string, model domain.ModelType) ResponseSigner {
	return func(ev *event.ModerationChallengeResponseEvent) {
		ev.ModeratorID = selfID
		ev.Model = model
		ev.TimeAt = time.Now().UTC()
		if len(privKey) == 0 {
			return
		}
		ev.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privKey, ev.SigningBytes()))
	}
}

// StreamChallengeHandler answers audit spot-checks: run the local engine on
// exactly the challenged text and return a signed verdict. NOT REGISTERED
// anywhere yet — wiring it under event.PUBLIC_GET_MODERATION_CHALLENGE on
// the moderator node is the integration step.
func StreamChallengeHandler(engine Engine, sign ResponseSigner) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ch event.ModerationChallengeEvent
		if err := json.Unmarshal(buf, &ch); err != nil {
			return nil, err
		}
		if ch.Text == "" {
			return nil, ErrEmptyChallenge
		}
		// Recompute the binding instead of trusting it: the signed answer
		// must attest the text that was actually judged, on any platform.
		if ContentHash(ch.Text) != ch.ContentHash {
			return nil, ErrChallengeHashMismatch
		}

		ok, reason, err := engine.Moderate(ch.Text)
		if err != nil {
			return nil, err
		}

		resp := event.ModerationChallengeResponseEvent{
			ChallengeID: ch.ChallengeID,
			ContentHash: ch.ContentHash,
			Result:      domain.ModerationResult(ok),
			Reason:      &reason,
		}
		if sign != nil {
			sign(&resp)
		}
		return resp, nil
	}
}
