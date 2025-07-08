package handler

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"math/rand/v2"
	"testing"
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

	ev := event.GetChallengeEvent{
		DirStack:  location.DirStack,
		FileStack: location.FileStack,
		Nonce:     nonce,
	}

	bt, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("failed to marshal challenge: %v", err)
	}

	resp, err := StreamChallengeHandler(root.GetCodeBase(), privKey)(bt, nil)
	if err != nil {
		t.Fatalf("challenge handler: %v", err)
	}

	challengeResp, ok := resp.(event.GetChallengeResponse)
	if !ok {
		t.Fatalf("challenge handler returned unexpected response")
	}

	hexedOwnChallenge := hex.EncodeToString(ownChallenge)

	if challengeResp.Challenge != hexedOwnChallenge {
		t.Fatalf("challenge: %s != %s", hexedOwnChallenge, challengeResp.Challenge)
	}

	decodedSig, err := base64.StdEncoding.DecodeString(challengeResp.Signature)
	if err != nil {
		t.Fatalf("failed to decode signature: %v", err)
	}

	challengeOrigin, err := hex.DecodeString(challengeResp.Challenge)
	if err != nil {
		t.Fatalf("failed to decode challenge origin: %v", err)
	}

	if !ed25519.Verify([]byte(pubKey), challengeOrigin, decodedSig) {
		t.Fatalf("failed to verify challenge")
	} else {
		fmt.Println("challenge verified")
	}
}
