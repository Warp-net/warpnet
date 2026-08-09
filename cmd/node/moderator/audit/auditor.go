// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"math/rand"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Challenger delivers a challenge to a peer and brings its answer back.
// Everything about how that happens — streams, routes, encoding — lives
// behind this interface, so the audit logic never has to know.
type Challenger interface {
	Ask(peer string, ch Challenge) (ChallengeResponse, error)
}

// defaultCooldown limits how often one peer gets challenged by an auditor,
// so audits stay cheap for honest moderators (one inference per challenge).
const defaultCooldown = 10 * time.Minute

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
	selfID     string
	challenger Challenger
	ledger     *Ledger
	corpus     *Corpus
	rng        *rand.Rand
	cooldown   time.Duration

	mu        sync.Mutex
	lastAsked map[string]time.Time
}

func NewAuditor(selfID string, challenger Challenger, ledger *Ledger, corpus *Corpus, rng *rand.Rand) *Auditor {
	return &Auditor{
		selfID:     selfID,
		challenger: challenger,
		ledger:     ledger,
		corpus:     corpus,
		rng:        rng,
		cooldown:   defaultCooldown,
		lastAsked:  make(map[string]time.Time),
	}
}

// ChallengeRandomPeer picks one eligible peer at random and runs a single
// spot-check against it. A nil Result means there was nothing to run: no
// eligible peer (empty set, all on cooldown, or only self), or a corpus too
// thin to judge anyone by. An audit never fails: a peer that cannot answer,
// or answers garbage, has simply earned that outcome.
func (a *Auditor) ChallengeRandomPeer(peers []string) *Result {
	ch, expectUnsafe, ok := BuildChallenge(a.rng, a.corpus)
	if !ok {
		return nil
	}
	target := a.pickTarget(peers)
	if target == "" {
		return nil
	}
	return a.challenge(target, ch, expectUnsafe)
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
		if last, ok := a.lastAsked[p]; ok && now.Sub(last) < a.cooldown {
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

func (a *Auditor) challenge(peer string, ch Challenge, expectUnsafe bool) *Result {
	res := &Result{Peer: peer, Expected: expectUnsafe}

	resp, err := a.challenger.Ask(peer, ch)
	if err != nil {
		log.Infof("audit: peer %s did not answer: %v", peer, err)
		res.Outcome = OutcomeUnreachable
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

// judge checks that the answer is bound to this challenge and really signed
// by the peer that was asked, then compares verdict classes. Class
// comparison only: moderators run different models, so agreement is judged
// statistically over many challenges, never on a single one.
func judge(ch Challenge, expectUnsafe bool, peer string, resp ChallengeResponse) Outcome {
	if resp.ChallengeID != ch.ChallengeID || resp.ContentHash != ch.ContentHash {
		return OutcomeInvalid
	}
	if err := resp.VerifiedFrom(peer); err != nil {
		return OutcomeInvalid
	}

	gotUnsafe := !bool(resp.Result)
	if gotUnsafe == expectUnsafe {
		return OutcomeCorrect
	}
	return OutcomeWrong
}
