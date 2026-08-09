// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// ContentHash is the cross-platform binding between a challenge and its
// response: hex(sha256) over the raw UTF-8 bytes of the text. Identical on
// every architecture, no normalization games.
func ContentHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// BuildChallenge draws a reference from the corpus and wraps it as a
// spot-check. The bool is false while the corpus is too thin to audit
// anyone with; expectUnsafe is the verdict a round already reached on that
// text, which is what the answer gets compared against.
func BuildChallenge(rng *rand.Rand, corpus *Corpus) (ch Challenge, expectUnsafe, ok bool) {
	text, expectUnsafe, ok := corpus.Sample(rng)
	if !ok {
		return Challenge{}, false, false
	}
	return Challenge{
		ChallengeID: uuid.New().String(),
		Text:        text,
		ContentHash: ContentHash(text),
		TimeAt:      time.Now().UTC(),
	}, expectUnsafe, true
}
