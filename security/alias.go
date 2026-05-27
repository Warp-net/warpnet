package security

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func BuildWarpID(privKey ed25519.PrivateKey) string {
	mac := hmac.New(sha256.New, privKey)
	_, _ = mac.Write([]byte("warpid/v0"))
	return hex.EncodeToString(mac.Sum(nil))
}
