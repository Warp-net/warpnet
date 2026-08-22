package security

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertToSHA256_Empty(t *testing.T) {
	result := ConvertToSHA256([]byte{})
	assert.Empty(t, result)
}

func TestConvertToSHA256_Nil(t *testing.T) {
	result := ConvertToSHA256(nil)
	assert.Empty(t, result)
}

func TestConvertToSHA256_ValidInput(t *testing.T) {
	input := []byte("hello world")
	result := ConvertToSHA256(input)

	expected := sha256.New()
	expected.Write(input)
	assert.Equal(t, expected.Sum(nil), result)
	assert.Len(t, result, 32)
}

func TestConvertToSHA256_Deterministic(t *testing.T) {
	input := []byte("deterministic test")
	r1 := ConvertToSHA256(input)
	r2 := ConvertToSHA256(input)
	assert.Equal(t, r1, r2)
}

func TestConvertToSHA256_DifferentInputs(t *testing.T) {
	r1 := ConvertToSHA256([]byte("input1"))
	r2 := ConvertToSHA256([]byte("input2"))
	assert.NotEqual(t, r1, r2)
}
