package security

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	assert.NoError(t, err)

	body := []byte("message to sign")
	sig := Sign(priv, body)
	assert.NotEmpty(t, sig)

	err = VerifySignature(pub, body, sig)
	assert.NoError(t, err)
}

func TestVerifySignature_WrongBody(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	sig := Sign(priv, []byte("original message"))
	err := VerifySignature(pub, []byte("tampered message"), sig)
	assert.True(t, errors.Is(err, ErrSignatureVerificationFailed))
}

func TestVerifySignature_InvalidBase64(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	err := VerifySignature(pub, []byte("body"), "not-valid-base64!!!")
	assert.Error(t, err)
}

func TestSign_Deterministic(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	body := []byte("same message")

	s1 := Sign(priv, body)
	s2 := Sign(priv, body)
	assert.Equal(t, s1, s2)
}

func TestVerifySignature_WrongKey(t *testing.T) {
	_, priv1, _ := ed25519.GenerateKey(nil)
	pub2, _, _ := ed25519.GenerateKey(nil)

	body := []byte("message")
	sig := Sign(priv1, body)

	err := VerifySignature(pub2, body, sig)
	assert.True(t, errors.Is(err, ErrSignatureVerificationFailed))
}
