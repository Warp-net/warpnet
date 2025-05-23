package security

import (
	root "github.com/Warp-net/warpnet"
	"testing"
)

func TestChallengeResolve_Success(t *testing.T) {
	codebase := root.GetCodeBase()
	nonce := int64(1)

	for i := 0; i < 10; i++ {
		sampleOrigin, location, err := GenerateChallenge(codebase, nonce)
		if err != nil {
			t.Fatal(err)
		}
		sampleFound, err := ResolveChallenge(codebase, location, nonce)
		if err != nil {
			t.Fatal(err)
		}

		if string(sampleFound) != string(sampleOrigin) {
			t.Fatalf("samples are not equal: \n %s %s", sampleOrigin, sampleFound)
		}
	}
}
