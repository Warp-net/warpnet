//nolint:all
package handler

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"testing"

	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

func TestStreamChallengeHandler_Success(t *testing.T) {
	privKey, err := security.GenerateKeyFromSeed([]byte("test"))
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKey, ok := privKey.Public().(ed25519.PublicKey)
	if !ok {
		t.Fatalf("failed to cast public key")
	}
	nonce := rand.Int64()

	ownChallenge, location, err := security.GenerateChallenge(root.GetCodeBase(), nonce)
	if err != nil {
		t.Fatalf("failed to generate challenge: %v", err)
	}

	ev := event.ChallengeEvent{[]event.ChallengeSample{{
		DirStack:  location.DirStack,
		FileStack: location.FileStack,
		Nonce:     nonce,
	}}}

	bt, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("failed to marshal challenge: %v", err)
	}

	resp, err := StreamChallengeHandler(root.GetCodeBase(), privKey)(bt, nil)
	if err != nil {
		t.Fatalf("challenge handler: %v", err)
	}

	challengeResp, ok := resp.(event.ChallengeResponse)
	if !ok {
		t.Fatalf("challenge handler returned unexpected response")
	}

	hexedOwnChallenge := hex.EncodeToString(ownChallenge)

	solution := challengeResp.Solutions[0]

	if solution.Challenge != hexedOwnChallenge {
		t.Fatalf("challenge: %s != %s", hexedOwnChallenge, solution.Challenge)
	}

	decodedSig, err := base64.StdEncoding.DecodeString(solution.Signature)
	if err != nil {
		t.Fatalf("failed to decode signature: %v", err)
	}

	challengeOrigin, err := hex.DecodeString(solution.Challenge)
	if err != nil {
		t.Fatalf("failed to decode challenge origin: %v", err)
	}

	if !ed25519.Verify([]byte(pubKey), challengeOrigin, decodedSig) {
		t.Fatalf("failed to verify challenge")
	} else {
		fmt.Println("challenge verified")
	}
}

func TestStreamChallengeHandler_OutOfBoundsSampleFallback(t *testing.T) {
	privKey, err := security.GenerateKeyFromSeed([]byte("test"))
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	ev := event.ChallengeEvent{[]event.ChallengeSample{{
		DirStack:  []int{},
		FileStack: []int{9999, 0, 0, 1},
		Nonce:     42,
	}}}

	bt, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("failed to marshal challenge: %v", err)
	}

	resp, err := StreamChallengeHandler(root.GetCodeBase(), privKey)(bt, nil)
	if err != nil {
		t.Fatalf("challenge handler should not fail on out-of-bounds sample: %v", err)
	}

	challengeResp, ok := resp.(event.ChallengeResponse)
	if !ok {
		t.Fatalf("challenge handler returned unexpected response")
	}
	if len(challengeResp.Solutions) != 1 {
		t.Fatalf("expected one solution, got %d", len(challengeResp.Solutions))
	}

	expected := hex.EncodeToString(security.ConvertToSHA256([]byte("42")))
	if challengeResp.Solutions[0].Challenge != expected {
		t.Fatalf("fallback challenge mismatch: %s != %s", challengeResp.Solutions[0].Challenge, expected)
	}
}
