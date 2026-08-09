//nolint:all
package audit

import (
	"crypto/ed25519"
	"errors"
	"math/rand"
	"strconv"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

// referenceCorpus fills a corpus with neutral placeholder texts. Nothing
// here needs to read like real content: the audit only ever compares the
// verdict a round already reached against the answer a peer gives.
func referenceCorpus(t *testing.T, n int) (*Corpus, map[string]bool) {
	t.Helper()
	c := NewCorpus()
	truth := make(map[string]bool, 2*n)
	for i := 0; i < n; i++ {
		clean := "reference-clean-" + strconv.Itoa(i)
		flagged := "reference-flagged-" + strconv.Itoa(i)
		c.Remember(clean, domain.OK)
		c.Remember(flagged, domain.FAIL)
		truth[clean], truth[flagged] = false, true
	}
	return c, truth
}

func testRNG() *rand.Rand { return rand.New(rand.NewSource(1)) }

// identity builds a peer whose id embeds its pubkey, like a real node id.
func identity(t *testing.T, seed string) (ed25519.PrivateKey, string) {
	t.Helper()
	priv, err := security.GenerateKeyFromSeed([]byte(seed))
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("derive id: %v", err)
	}
	return priv, id.String()
}

// engineFunc adapts a func to the Engine interface.
type engineFunc func(string) (bool, string, error)

func (f engineFunc) Moderate(c string) (bool, string, error) { return f(c) }

// honestEngine stands in for a peer whose model agrees with the network,
// with an optional per-call error rate standing in for model diversity.
// It is handed the ground truth because a test cannot run an LLM; a real
// peer has to reach the same answer by actually classifying the text.
func honestEngine(truth map[string]bool, rng *rand.Rand, noise float64) Engine {
	return engineFunc(func(text string) (bool, string, error) {
		unsafe := truth[text]
		if noise > 0 && rng.Float64() < noise {
			unsafe = !unsafe
		}
		if unsafe {
			return false, "flagged", nil
		}
		return true, "", nil
	})
}

// answerFunc is a Challenger that produces the peer's answer directly, so
// audit logic can be tested without any transport at all.
type answerFunc func(peer string, ch Challenge) (ChallengeResponse, error)

func (f answerFunc) Ask(peer string, ch Challenge) (ChallengeResponse, error) { return f(peer, ch) }

// enginePeer answers challenges the way a real moderator would: run the
// engine over the challenged text, then sign under the given identity.
func enginePeer(engine Engine, sign ResponseSigner) Challenger {
	return answerFunc(func(_ string, ch Challenge) (ChallengeResponse, error) {
		if ContentHash(ch.Text) != ch.ContentHash {
			return ChallengeResponse{}, ErrChallengeHashMismatch
		}
		ok, reason, err := engine.Moderate(ch.Text)
		if err != nil {
			return ChallengeResponse{}, err
		}
		resp := ChallengeResponse{
			ChallengeID: ch.ChallengeID,
			ContentHash: ch.ContentHash,
			Result:      domain.ModerationResult(ok),
			Reason:      &reason,
		}
		if sign != nil {
			resp = sign(resp)
		}
		return resp, nil
	})
}

// runAudit drives n spot-checks of one peer and returns its final standing.
func runAudit(t *testing.T, peerID string, corpus *Corpus, challenger Challenger, n int) (*Ledger, Standing) {
	t.Helper()
	ledger := NewLedger()
	a := NewAuditor("auditor-self", challenger, ledger, corpus, testRNG())
	a.cooldown = 0 // audit the same peer repeatedly in one test

	for i := 0; i < n; i++ {
		a.ChallengeRandomPeer([]string{peerID})
	}
	return ledger, ledger.StandingOf(peerID)
}

func TestContentHash_StableAndBinding(t *testing.T) {
	if ContentHash("hello") != ContentHash("hello") {
		t.Fatal("hash must be deterministic")
	}
	if ContentHash("hello") == ContentHash("hello ") {
		t.Fatal("hash must bind the exact bytes")
	}
	if len(ContentHash("hello")) != 64 {
		t.Fatal("expected hex sha256")
	}
}

// A constant-answer bot must land under the ban threshold whichever class
// it picks, which only holds while the corpus draws both classes evenly.
func TestCorpus_DrawsBothClassesEvenly(t *testing.T) {
	corpus, _ := referenceCorpus(t, 8)
	rng := testRNG()

	unsafe := 0
	const draws = 400
	for i := 0; i < draws; i++ {
		_, wantUnsafe, ok := corpus.Sample(rng)
		if !ok {
			t.Fatal("a filled corpus must yield samples")
		}
		if wantUnsafe {
			unsafe++
		}
	}
	share := float64(unsafe) / draws
	if share < 0.4 || share > 0.6 {
		t.Fatalf("classes must be drawn evenly, got %.2f unsafe", share)
	}
	if share >= banAgreeBelow || 1-share >= banAgreeBelow {
		t.Fatal("a constant-answer bot would escape the ban threshold")
	}
}

// Until a node has seen both a clean and a moderated text it has nothing to
// audit anyone against, and must not guess.
func TestCorpus_ThinCorpusYieldsNothing(t *testing.T) {
	c := NewCorpus()
	if _, _, ok := c.Sample(testRNG()); ok {
		t.Fatal("an empty corpus must not yield a sample")
	}
	c.Remember("only-clean", domain.OK)
	if _, _, ok := c.Sample(testRNG()); ok {
		t.Fatal("one class alone must not yield a sample")
	}
	c.Remember("now-flagged", domain.FAIL)
	if _, _, ok := c.Sample(testRNG()); !ok {
		t.Fatal("both classes present must yield a sample")
	}
}

// A report storm over one tweet must not flood the references with copies.
func TestCorpus_RemembersEachTextOnce(t *testing.T) {
	c := NewCorpus()
	for i := 0; i < 10; i++ {
		c.Remember("same-text", domain.FAIL)
	}
	if _, unsafe := c.Len(); unsafe != 1 {
		t.Fatalf("expected one reference, got %d", unsafe)
	}
}

// The ring keeps the corpus bounded without ever emptying a class.
func TestCorpus_RingIsBounded(t *testing.T) {
	c := NewCorpus()
	for i := 0; i < corpusPerClass*3; i++ {
		c.Remember("clean-"+strconv.Itoa(i), domain.OK)
		c.Remember("flagged-"+strconv.Itoa(i), domain.FAIL)
	}
	safe, unsafe := c.Len()
	if safe != corpusPerClass || unsafe != corpusPerClass {
		t.Fatalf("each class must cap at %d, got %d/%d", corpusPerClass, safe, unsafe)
	}
}

func TestBuildChallenge_BindsTheDrawnReference(t *testing.T) {
	corpus, truth := referenceCorpus(t, 4)
	rng := testRNG()

	for i := 0; i < 50; i++ {
		ch, wantUnsafe, ok := BuildChallenge(rng, corpus)
		if !ok {
			t.Fatal("a filled corpus must produce a challenge")
		}
		if ch.ContentHash != ContentHash(ch.Text) {
			t.Fatal("a challenge must bind its own text")
		}
		if ch.ChallengeID == "" || ch.TimeAt.IsZero() {
			t.Fatalf("challenge must be identified and stamped: %+v", ch)
		}
		if truth[ch.Text] != wantUnsafe {
			t.Fatalf("the expected class must come from the reference, got %t for %q", wantUnsafe, ch.Text)
		}
	}
}

func TestBuildChallenge_ThinCorpusProducesNothing(t *testing.T) {
	if _, _, ok := BuildChallenge(testRNG(), NewCorpus()); ok {
		t.Fatal("an empty corpus must not produce a challenge")
	}
}

// An honest moderator running a different model (10% class noise) must stay
// trusted: audits judge statistically, not byte-for-byte.
func TestAudit_HonestModeratorWithDifferentModelStaysTrusted(t *testing.T) {
	priv, peerID := identity(t, "honest-peer")
	corpus, truth := referenceCorpus(t, 8)
	peer := enginePeer(
		honestEngine(truth, testRNG(), 0.10),
		NewResponseSigner(priv, peerID, domain.ModelType("SomeOtherGuard")),
	)

	ledger, standing := runAudit(t, peerID, corpus, peer, 60)
	if standing != StandingTrusted {
		t.Fatalf("expected trusted, got %s (%+v)", standing, ledger.Snapshot()[peerID])
	}
}

// The vote bot from the live T5 run: always answers OK, never runs a model.
// The audit must ban it.
func TestAudit_AlwaysOkBotIsBanned(t *testing.T) {
	priv, peerID := identity(t, "always-ok-bot")
	corpus, _ := referenceCorpus(t, 8)
	peer := enginePeer(
		engineFunc(func(string) (bool, string, error) { return true, "looks fine to me", nil }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)

	ledger, standing := runAudit(t, peerID, corpus, peer, 60)
	if standing != StandingBanned {
		t.Fatalf("expected banned, got %s (%+v)", standing, ledger.Snapshot()[peerID])
	}
}

// The mirror-image bot: always FAIL, censoring everything.
func TestAudit_AlwaysFailBotIsBanned(t *testing.T) {
	priv, peerID := identity(t, "always-fail-bot")
	corpus, _ := referenceCorpus(t, 8)
	peer := enginePeer(
		engineFunc(func(string) (bool, string, error) { return false, "flagged", nil }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)

	_, standing := runAudit(t, peerID, corpus, peer, 60)
	if standing != StandingBanned {
		t.Fatalf("expected banned, got %s", standing)
	}
}

// A coin-flipper (no real model, just guessing) must not pass as trusted.
func TestAudit_CoinFlipperIsNotTrusted(t *testing.T) {
	priv, peerID := identity(t, "coin-flipper")
	corpus, _ := referenceCorpus(t, 8)
	rng := rand.New(rand.NewSource(7))
	peer := enginePeer(
		engineFunc(func(string) (bool, string, error) { return rng.Intn(2) == 0, "", nil }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)

	_, standing := runAudit(t, peerID, corpus, peer, 80)
	if standing == StandingTrusted {
		t.Fatal("a guessing peer must not reach trusted standing")
	}
}

// Too few answers is probation, not trust: fresh identities carry no weight.
func TestAudit_SmallSampleStaysProbation(t *testing.T) {
	priv, peerID := identity(t, "fresh-peer")
	corpus, truth := referenceCorpus(t, 8)
	peer := enginePeer(
		honestEngine(truth, testRNG(), 0),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)

	_, standing := runAudit(t, peerID, corpus, peer, minSample-1)
	if standing != StandingProbation {
		t.Fatalf("expected probation below the minimum sample, got %s", standing)
	}
}

// An unsigned answer, an answer signed by the wrong key, and a rebound
// challenge id are all invalid — and two invalids ban.
func TestAudit_InvalidResponsesBan(t *testing.T) {
	cases := map[string]ResponseSigner{
		"unsigned": func(resp ChallengeResponse) ChallengeResponse {
			_, id := identity(t, "victim-peer")
			resp.ModeratorID = id // claims the identity, never signs
			return resp
		},
		"foreign key": func(resp ChallengeResponse) ChallengeResponse {
			impostor, _ := identity(t, "impostor")
			_, id := identity(t, "victim-peer")
			return resp.Signed(impostor, id, domain.LLAMAGuard3)
		},
		"rebound challenge": func(resp ChallengeResponse) ChallengeResponse {
			priv, id := identity(t, "victim-peer")
			resp.ChallengeID = "some-other-challenge"
			return resp.Signed(priv, id, domain.LLAMAGuard3)
		},
		"foreign responder id": func(resp ChallengeResponse) ChallengeResponse {
			other, otherID := identity(t, "somebody-else")
			// valid signature, wrong peer
			return resp.Signed(other, otherID, domain.LLAMAGuard3)
		},
	}

	for name, signer := range cases {
		t.Run(name, func(t *testing.T) {
			_, peerID := identity(t, "victim-peer")
			corpus, truth := referenceCorpus(t, 8)
			peer := enginePeer(honestEngine(truth, testRNG(), 0), signer)

			ledger, standing := runAudit(t, peerID, corpus, peer, maxInvalid+1)
			if standing != StandingBanned {
				t.Fatalf("expected banned, got %s (%+v)", standing, ledger.Snapshot()[peerID])
			}
		})
	}
}

// An unreachable peer is suspect, never banned: the network may be at fault.
func TestAudit_UnreachablePeerIsSuspectNotBanned(t *testing.T) {
	_, peerID := identity(t, "offline-peer")
	corpus, _ := referenceCorpus(t, 8)
	peer := answerFunc(func(string, Challenge) (ChallengeResponse, error) {
		return ChallengeResponse{}, errors.New("dial failed")
	})

	ledger, standing := runAudit(t, peerID, corpus, peer, minSample+5)
	if standing != StandingSuspect {
		t.Fatalf("expected suspect, got %s (%+v)", standing, ledger.Snapshot()[peerID])
	}
}

// A peer that answers with an error envelope counts as unreachable, and the
// engine's error must not be mistaken for a verdict.
func TestAudit_EngineErrorCountsAsUnreachable(t *testing.T) {
	priv, peerID := identity(t, "broken-engine-peer")
	corpus, _ := referenceCorpus(t, 8)
	peer := enginePeer(
		engineFunc(func(string) (bool, string, error) { return false, "", errors.New("model not loaded") }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)

	ledger, standing := runAudit(t, peerID, corpus, peer, minSample+5)
	rep := ledger.Snapshot()[peerID]
	if rep.Correct != 0 || rep.Wrong != 0 {
		t.Fatalf("an engine failure is not a verdict: %+v", rep)
	}
	if standing != StandingSuspect {
		t.Fatalf("expected suspect, got %s", standing)
	}
}

func TestAuditor_SkipsSelfAndCooldownPeers(t *testing.T) {
	ledger := NewLedger()
	corpus, truth := referenceCorpus(t, 8)
	peer := enginePeer(honestEngine(truth, testRNG(), 0), nil)
	a := NewAuditor("auditor-self", peer, ledger, corpus, testRNG())

	if res := a.ChallengeRandomPeer([]string{"auditor-self"}); res != nil {
		t.Fatalf("must never audit itself, got %+v", res)
	}
	if res := a.ChallengeRandomPeer(nil); res != nil {
		t.Fatalf("empty peer set must be a no-op, got %+v", res)
	}

	_, peerID := identity(t, "cooldown-peer")
	if res := a.ChallengeRandomPeer([]string{peerID}); res == nil {
		t.Fatal("the first challenge must go out")
	}
	if res := a.ChallengeRandomPeer([]string{peerID}); res != nil {
		t.Fatal("a peer on cooldown must not be challenged again")
	}
}

// The respondent recomputes the binding: a challenge whose hash does not
// match its text is rejected rather than answered.
func TestChallengeHandler_RejectsTamperedBinding(t *testing.T) {
	priv, peerID := identity(t, "responder")
	corpus, truth := referenceCorpus(t, 8)
	h := StreamChallengeHandler(honestEngine(truth, testRNG(), 0), NewResponseSigner(priv, peerID, domain.LLAMAGuard3))

	ch, _, _ := BuildChallenge(testRNG(), corpus)
	ch.Text = "totally different text"
	buf, _ := json.Marshal(ch)
	if _, err := h(buf, nil); !errors.Is(err, ErrChallengeHashMismatch) {
		t.Fatalf("expected ErrChallengeHashMismatch, got %v", err)
	}

	ch2, _, _ := BuildChallenge(testRNG(), corpus)
	ch2.Text = ""
	ch2.ContentHash = ContentHash("")
	buf2, _ := json.Marshal(ch2)
	if _, err := h(buf2, nil); !errors.Is(err, ErrEmptyChallenge) {
		t.Fatalf("expected ErrEmptyChallenge, got %v", err)
	}
}

// The response signature must cover every field, so a transcript cannot be
// edited after the fact and still verify.
func TestChallengeResponse_SigningBytesCoverFields(t *testing.T) {
	reason := "Violent Crimes"
	base := ChallengeResponse{
		ChallengeID: "ch-1",
		ContentHash: ContentHash("some text"),
		Result:      domain.FAIL,
		Reason:      &reason,
		Model:       domain.LLAMAGuard3,
		ModeratorID: "peer-1",
	}
	mutations := map[string]func(*ChallengeResponse){
		"challenge": func(e *ChallengeResponse) { e.ChallengeID = "ch-2" },
		"hash":      func(e *ChallengeResponse) { e.ContentHash = ContentHash("other") },
		"result":    func(e *ChallengeResponse) { e.Result = domain.OK },
		"reason":    func(e *ChallengeResponse) { r := "Spam"; e.Reason = &r },
		"model":     func(e *ChallengeResponse) { e.Model = domain.ModelType("x") },
		"moderator": func(e *ChallengeResponse) { e.ModeratorID = "peer-2" },
		"time":      func(e *ChallengeResponse) { e.TimeAt = e.TimeAt.Add(1) },
	}
	baseBytes := string(base.signingBytes())
	for name, mutate := range mutations {
		ev := base
		mutate(&ev)
		if string(ev.signingBytes()) == baseBytes {
			t.Fatalf("mutating %s must change the signing bytes", name)
		}
	}
}
