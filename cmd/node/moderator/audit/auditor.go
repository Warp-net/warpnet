// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"math/rand"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

// Streamer is the slice of the moderator node the auditor dials peers with.
type Streamer interface {
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

// peerCooldown limits how often one peer gets challenged by this auditor,
// so audits stay cheap for honest moderators (one inference per probe).
var peerCooldown = 10 * time.Minute

// Result is one finished challenge exchange, kept alongside the raw signed
// response — the transcript a future reputation gossip can re-verify.
type Result struct {
	Peer     string
	Outcome  Outcome
	Expected bool // expectUnsafe: true means the honest class is FAIL
	Response ChallengeResponse
}

// Auditor drives moderator-to-moderator spot-checks: pick a random peer,
// hand it a probe, judge the signed answer, feed the ledger.
type Auditor struct {
	selfID string
	node   Streamer
	ledger *Ledger
	rng    *rand.Rand

	mu        sync.Mutex
	lastAsked map[string]time.Time
}

func NewAuditor(selfID string, node Streamer, ledger *Ledger, rng *rand.Rand) *Auditor {
	return &Auditor{
		selfID:    selfID,
		node:      node,
		ledger:    ledger,
		rng:       rng,
		lastAsked: make(map[string]time.Time),
	}
}

// ChallengeRandomPeer picks one eligible peer at random and runs a single
// spot-check against it. A nil Result means nobody was eligible (empty set,
// all on cooldown, or only self). An audit never fails: a peer that cannot
// answer, or answers garbage, has simply earned that outcome.
func (a *Auditor) ChallengeRandomPeer(peers []string) *Result {
	target := a.pickTarget(peers)
	if target == "" {
		return nil
	}
	return a.challenge(target)
}

func (a *Auditor) pickTarget(peers []string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	eligible := make([]string, 0, len(peers))
	for _, p := range peers {
		if p == "" || p == a.selfID {
			continue
		}
		if last, ok := a.lastAsked[p]; ok && now.Sub(last) < peerCooldown {
			continue
		}
		eligible = append(eligible, p)
	}
	if len(eligible) == 0 {
		return ""
	}
	target := eligible[a.rng.Intn(len(eligible))]
	a.lastAsked[target] = now
	return target
}

func (a *Auditor) challenge(peer string) *Result {
	ch, expectUnsafe := BuildChallenge(a.rng)
	res := &Result{Peer: peer, Expected: expectUnsafe}

	data, err := a.node.GenericStream(peer, ChallengeRoute, ch)
	if err != nil {
		log.Infof("audit: peer %s unreachable: %v", peer, err)
		res.Outcome = OutcomeUnreachable
		a.ledger.Record(peer, res.Outcome)
		return res
	}
	var respErr event.ResponseError
	if json.Unmarshal(data, &respErr) == nil && respErr.Message != "" {
		log.Infof("audit: peer %s refused challenge: %s", peer, respErr.Message)
		res.Outcome = OutcomeUnreachable
		a.ledger.Record(peer, res.Outcome)
		return res
	}

	var resp ChallengeResponse
	if json.Unmarshal(data, &resp) != nil {
		res.Outcome = OutcomeInvalid
		a.ledger.Record(peer, res.Outcome)
		return res
	}
	res.Response = resp

	res.Outcome = judge(ch, expectUnsafe, peer, resp)
	if res.Outcome == OutcomeInvalid || res.Outcome == OutcomeWrong {
		log.Warnf("audit: peer %s outcome=%d on challenge %s", peer, res.Outcome, ch.ChallengeID)
	}
	a.ledger.Record(peer, res.Outcome)
	return res
}

// judge validates the response binding and signature, then compares verdict
// classes. Class comparison only: moderators run different models, and the
// probe corpus is flagrant enough that any honest model lands the class.
func judge(ch Challenge, expectUnsafe bool, peer string, resp ChallengeResponse) Outcome {
	if resp.ChallengeID != ch.ChallengeID || resp.ContentHash != ch.ContentHash {
		return OutcomeInvalid
	}
	// The answer must be signed by the exact peer that was challenged;
	// a valid signature from anyone else is a proxy, not an answer.
	if resp.ModeratorID != peer {
		return OutcomeInvalid
	}
	peerID := warpnet.FromStringToPeerID(resp.ModeratorID)
	if peerID == "" {
		return OutcomeInvalid
	}
	pubKey := warpnet.FromIDToPubKey(peerID)
	if len(pubKey) == 0 {
		return OutcomeInvalid
	}
	if err := security.VerifySignature(pubKey, resp.SigningBytes(), resp.Signature); err != nil {
		return OutcomeInvalid
	}

	gotUnsafe := !bool(resp.Result)
	if gotUnsafe == expectUnsafe {
		return OutcomeCorrect
	}
	return OutcomeWrong
}
