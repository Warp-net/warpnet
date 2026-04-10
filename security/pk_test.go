package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateKeyFromSeed_Valid(t *testing.T) {
	seed := []byte("test-seed-value")
	key, err := GenerateKeyFromSeed(seed)
	assert.NoError(t, err)
	assert.NotNil(t, key)
	assert.Len(t, key, 64) // ed25519 private key is 64 bytes
}

func TestGenerateKeyFromSeed_EmptySeed(t *testing.T) {
	key, err := GenerateKeyFromSeed([]byte{})
	assert.Error(t, err)
	assert.Equal(t, ErrEmptySeed, err)
	assert.Nil(t, key)
}

func TestGenerateKeyFromSeed_NilSeed(t *testing.T) {
	key, err := GenerateKeyFromSeed(nil)
	assert.Error(t, err)
	assert.Equal(t, ErrEmptySeed, err)
	assert.Nil(t, key)
}

func TestGenerateKeyFromSeed_Deterministic(t *testing.T) {
	seed := []byte("reproducible-seed")
	k1, err1 := GenerateKeyFromSeed(seed)
	k2, err2 := GenerateKeyFromSeed(seed)
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, k1, k2)
}

func TestGenerateKeyFromSeed_DifferentSeeds(t *testing.T) {
	k1, _ := GenerateKeyFromSeed([]byte("seed1"))
	k2, _ := GenerateKeyFromSeed([]byte("seed2"))
	assert.NotEqual(t, k1, k2)
}
