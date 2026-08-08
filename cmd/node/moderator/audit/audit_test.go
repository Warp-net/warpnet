//nolint:all
package audit

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

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

// honestEngine answers by the probe's flagrant ground truth, with an
// optional per-call error rate standing in for model diversity.
func honestEngine(rng *rand.Rand, noise float64) Engine {
	return engineFunc(func(text string) (bool, string, error) {
		unsafe := isUnsafeProbeText(text)
		if noise > 0 && rng.Float64() < noise {
			unsafe = !unsafe
		}
		if unsafe {
			return false, "Violent Crimes", nil
		}
		return true, "", nil
	})
}

// isUnsafeProbeText recovers a probe's ground truth from its instantiated
// text by matching the corpus template prefixes.
func isUnsafeProbeText(text string) bool {
	for _, p := range probes {
		head := p.template
		if i := strings.IndexByte(head, '{'); i > 0 {
			head = head[:i]
		}
		if strings.HasPrefix(text, head) {
			return p.unsafe
		}
	}
	return false
}

// peerNode routes challenges to a local handler, simulating the remote
// moderator answering over the wire.
type peerNode struct {
	handler warpnet.WarpHandlerFunc
	fail    error
}

func (n peerNode) GenericStream(_ string, _ stream.WarpRoute, data any) ([]byte, error) {
	if n.fail != nil {
		return nil, n.fail
	}
	buf, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	resp, err := n.handler(buf, nil)
	if err != nil {
		return json.Marshal(event.ResponseError{Code: 500, Message: err.Error()})
	}
	return json.Marshal(resp)
}

// runAudit drives n spot-checks of one peer and returns its final standing.
func runAudit(t *testing.T, peerID string, node Streamer, n int) (*Ledger, Standing) {
	t.Helper()
	ledger := NewLedger()
	a := NewAuditor("auditor-self", node, ledger, testRNG())
	prev := peerCooldown
	peerCooldown = 0 // audit the same peer repeatedly in one test
	t.Cleanup(func() { peerCooldown = prev })

	for i := 0; i < n; i++ {
		if _, err := a.ChallengeRandomPeer([]string{peerID}); err != nil {
			t.Fatalf("challenge: %v", err)
		}
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

// The corpus must stay balanced and parameterized: with a binary verdict
// the two constant-answer rates sum to 1, so only a balanced corpus keeps
// BOTH constant bots under the ban threshold.
func TestProbeCorpus_BalancedAndParameterized(t *testing.T) {
	unsafe := 0
	for _, p := range probes {
		if p.unsafe {
			unsafe++
		}
		if !strings.Contains(p.template, "{NAME}") && !strings.Contains(p.template, "{PLACE}") {
			t.Fatalf("probe must be parameterized to resist memorization: %q", p.template)
		}
	}
	safe := len(probes) - unsafe
	if unsafe != safe {
		t.Fatalf("corpus must be class-balanced (got %d unsafe / %d safe)", unsafe, safe)
	}
	if float64(safe)/float64(len(probes)) >= banAgreeBelow {
		t.Fatal("a constant-FAIL bot would escape the ban threshold")
	}
	if float64(unsafe)/float64(len(probes)) >= banAgreeBelow {
		t.Fatal("a constant-OK bot would escape the ban threshold")
	}
}

func TestBuildChallenge_FillsPlaceholdersAndBinds(t *testing.T) {
	rng := testRNG()
	for i := 0; i < 50; i++ {
		ch, _ := BuildChallenge(rng)
		if strings.Contains(ch.Text, "{") {
			t.Fatalf("placeholder left unfilled: %q", ch.Text)
		}
		if ch.ContentHash != ContentHash(ch.Text) {
			t.Fatal("challenge must bind its own text")
		}
		if ch.ChallengeID == "" || ch.TimeAt.IsZero() {
			t.Fatalf("challenge must be identified and stamped: %+v", ch)
		}
	}
}

// An honest moderator running a different model (10% class noise) must stay
// trusted: audits judge statistically, not byte-for-byte.
func TestAudit_HonestModeratorWithDifferentModelStaysTrusted(t *testing.T) {
	priv, peerID := identity(t, "honest-peer")
	node := peerNode{handler: StreamChallengeHandler(
		honestEngine(testRNG(), 0.10),
		NewResponseSigner(priv, peerID, domain.ModelType("SomeOtherGuard")),
	)}

	ledger, standing := runAudit(t, peerID, node, 60)
	if standing != StandingTrusted {
		t.Fatalf("expected trusted, got %s (%+v)", standing, ledger.Snapshot()[peerID])
	}
}

// The vote bot from the live T5 run: always answers OK, never runs a model.
// The audit must ban it.
func TestAudit_AlwaysOkBotIsBanned(t *testing.T) {
	priv, peerID := identity(t, "always-ok-bot")
	node := peerNode{handler: StreamChallengeHandler(
		engineFunc(func(string) (bool, string, error) { return true, "looks fine to me", nil }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)}

	ledger, standing := runAudit(t, peerID, node, 60)
	if standing != StandingBanned {
		t.Fatalf("expected banned, got %s (%+v)", standing, ledger.Snapshot()[peerID])
	}
}

// The mirror-image bot: always FAIL, censoring everything.
func TestAudit_AlwaysFailBotIsBanned(t *testing.T) {
	priv, peerID := identity(t, "always-fail-bot")
	node := peerNode{handler: StreamChallengeHandler(
		engineFunc(func(string) (bool, string, error) { return false, "Violent Crimes", nil }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)}

	_, standing := runAudit(t, peerID, node, 60)
	if standing != StandingBanned {
		t.Fatalf("expected banned, got %s", standing)
	}
}

// A coin-flipper (no real model, just guessing) must not pass as trusted.
func TestAudit_CoinFlipperIsNotTrusted(t *testing.T) {
	priv, peerID := identity(t, "coin-flipper")
	rng := rand.New(rand.NewSource(7))
	node := peerNode{handler: StreamChallengeHandler(
		engineFunc(func(string) (bool, string, error) { return rng.Intn(2) == 0, "", nil }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)}

	_, standing := runAudit(t, peerID, node, 80)
	if standing == StandingTrusted {
		t.Fatal("a guessing peer must not reach trusted standing")
	}
}

// Too few answers is probation, not trust: fresh identities carry no weight.
func TestAudit_SmallSampleStaysProbation(t *testing.T) {
	priv, peerID := identity(t, "fresh-peer")
	node := peerNode{handler: StreamChallengeHandler(
		honestEngine(testRNG(), 0),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)}

	_, standing := runAudit(t, peerID, node, minSample-1)
	if standing != StandingProbation {
		t.Fatalf("expected probation below the minimum sample, got %s", standing)
	}
}

// An unsigned answer, an answer signed by the wrong key, and a rebound
// challenge id are all invalid — and two invalids ban.
func TestAudit_InvalidResponsesBan(t *testing.T) {
	cases := map[string]ResponseSigner{
		"unsigned": func(ev *event.ModerationChallengeResponseEvent) {
			_, id := identity(t, "victim-peer")
			ev.ModeratorID = id // claims the identity, never signs
		},
		"foreign key": func(ev *event.ModerationChallengeResponseEvent) {
			impostor, _ := identity(t, "impostor")
			_, id := identity(t, "victim-peer")
			ev.ModeratorID = id
			signWith(impostor, ev)
		},
		"rebound challenge": func(ev *event.ModerationChallengeResponseEvent) {
			priv, id := identity(t, "victim-peer")
			ev.ModeratorID = id
			ev.ChallengeID = "some-other-challenge"
			signWith(priv, ev)
		},
		"foreign responder id": func(ev *event.ModerationChallengeResponseEvent) {
			other, otherID := identity(t, "somebody-else")
			ev.ModeratorID = otherID // valid signature, wrong peer
			signWith(other, ev)
		},
	}

	for name, signer := range cases {
		t.Run(name, func(t *testing.T) {
			_, peerID := identity(t, "victim-peer")
			node := peerNode{handler: StreamChallengeHandler(honestEngine(testRNG(), 0), signer)}

			ledger, standing := runAudit(t, peerID, node, maxInvalid+1)
			if standing != StandingBanned {
				t.Fatalf("expected banned, got %s (%+v)", standing, ledger.Snapshot()[peerID])
			}
		})
	}
}

func signWith(priv ed25519.PrivateKey, ev *event.ModerationChallengeResponseEvent) {
	ev.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, ev.SigningBytes()))
}

// An unreachable peer is suspect, never banned: the network may be at fault.
func TestAudit_UnreachablePeerIsSuspectNotBanned(t *testing.T) {
	_, peerID := identity(t, "offline-peer")
	node := peerNode{fail: errors.New("dial failed")}

	ledger, standing := runAudit(t, peerID, node, minSample+5)
	if standing != StandingSuspect {
		t.Fatalf("expected suspect, got %s (%+v)", standing, ledger.Snapshot()[peerID])
	}
}

// A peer that answers with an error envelope counts as unreachable, and the
// engine's error must not be mistaken for a verdict.
func TestAudit_EngineErrorCountsAsUnreachable(t *testing.T) {
	priv, peerID := identity(t, "broken-engine-peer")
	node := peerNode{handler: StreamChallengeHandler(
		engineFunc(func(string) (bool, string, error) { return false, "", errors.New("model not loaded") }),
		NewResponseSigner(priv, peerID, domain.LLAMAGuard3),
	)}

	ledger, standing := runAudit(t, peerID, node, minSample+5)
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
	node := peerNode{handler: StreamChallengeHandler(honestEngine(testRNG(), 0), nil)}
	a := NewAuditor("auditor-self", node, ledger, testRNG())

	if res, err := a.ChallengeRandomPeer([]string{"auditor-self"}); err != nil || res != nil {
		t.Fatalf("must never audit itself, got %+v %v", res, err)
	}
	if res, err := a.ChallengeRandomPeer(nil); err != nil || res != nil {
		t.Fatalf("empty peer set must be a no-op, got %+v %v", res, err)
	}

	_, peerID := identity(t, "cooldown-peer")
	if res, _ := a.ChallengeRandomPeer([]string{peerID}); res == nil {
		t.Fatal("the first challenge must go out")
	}
	if res, _ := a.ChallengeRandomPeer([]string{peerID}); res != nil {
		t.Fatal("a peer on cooldown must not be challenged again")
	}
}

// The respondent recomputes the binding: a challenge whose hash does not
// match its text is rejected rather than answered.
func TestChallengeHandler_RejectsTamperedBinding(t *testing.T) {
	priv, peerID := identity(t, "responder")
	h := StreamChallengeHandler(honestEngine(testRNG(), 0), NewResponseSigner(priv, peerID, domain.LLAMAGuard3))

	ch, _ := BuildChallenge(testRNG())
	ch.Text = "totally different text"
	buf, _ := json.Marshal(ch)
	if _, err := h(buf, nil); !errors.Is(err, ErrChallengeHashMismatch) {
		t.Fatalf("expected ErrChallengeHashMismatch, got %v", err)
	}

	ch2, _ := BuildChallenge(testRNG())
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
	base := event.ModerationChallengeResponseEvent{
		ChallengeID: "ch-1",
		ContentHash: ContentHash("some text"),
		Result:      domain.FAIL,
		Reason:      &reason,
		Model:       domain.LLAMAGuard3,
		ModeratorID: "peer-1",
	}
	mutations := map[string]func(*event.ModerationChallengeResponseEvent){
		"challenge": func(e *event.ModerationChallengeResponseEvent) { e.ChallengeID = "ch-2" },
		"hash":      func(e *event.ModerationChallengeResponseEvent) { e.ContentHash = ContentHash("other") },
		"result":    func(e *event.ModerationChallengeResponseEvent) { e.Result = domain.OK },
		"reason":    func(e *event.ModerationChallengeResponseEvent) { r := "Spam"; e.Reason = &r },
		"model":     func(e *event.ModerationChallengeResponseEvent) { e.Model = domain.ModelType("x") },
		"moderator": func(e *event.ModerationChallengeResponseEvent) { e.ModeratorID = "peer-2" },
		"time":      func(e *event.ModerationChallengeResponseEvent) { e.TimeAt = e.TimeAt.Add(1) },
	}
	baseBytes := string(base.SigningBytes())
	for name, mutate := range mutations {
		ev := base
		mutate(&ev)
		if string(ev.SigningBytes()) == baseBytes {
			t.Fatalf("mutating %s must change the signing bytes", name)
		}
	}
}
