package security

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
)

func TestPSK_String(t *testing.T) {
	psk := PSK([]byte{0xab, 0xcd, 0xef})
	assert.Equal(t, "abcdef", psk.String())
}

func TestGeneratePSK_Valid(t *testing.T) {
	v := semver.MustParse("1.0.0")
	psk, err := GeneratePSK("warpnet", v)
	assert.NoError(t, err)
	assert.NotEmpty(t, psk)
	assert.Len(t, psk, 32) // SHA256 output
}

func TestGeneratePSK_EmptyNetwork(t *testing.T) {
	v := semver.MustParse("1.0.0")
	psk, err := GeneratePSK("", v)
	assert.Error(t, err)
	assert.Equal(t, ErrPSKNetwrokRequired, err)
	assert.Nil(t, psk)
}

func TestGeneratePSK_NilVersion(t *testing.T) {
	psk, err := GeneratePSK("warpnet", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrPSKVersionRequired, err)
	assert.Nil(t, psk)
}

func TestGeneratePSK_Deterministic(t *testing.T) {
	v := semver.MustParse("1.0.0")
	p1, _ := GeneratePSK("warpnet", v)
	p2, _ := GeneratePSK("warpnet", v)
	assert.Equal(t, p1, p2)
}

func TestGeneratePSK_DifferentNetworks(t *testing.T) {
	v := semver.MustParse("1.0.0")
	p1, _ := GeneratePSK("warpnet", v)
	p2, _ := GeneratePSK("testnet", v)
	assert.NotEqual(t, p1, p2)
}

func TestGeneratePSK_DifferentMajorVersions(t *testing.T) {
	v1 := semver.MustParse("1.0.0")
	v2 := semver.MustParse("2.0.0")
	p1, _ := GeneratePSK("warpnet", v1)
	p2, _ := GeneratePSK("warpnet", v2)
	assert.NotEqual(t, p1, p2)
}

func TestGeneratePSK_SameMajorDifferentMinor(t *testing.T) {
	v1 := semver.MustParse("1.0.0")
	v2 := semver.MustParse("1.5.0")
	p1, _ := GeneratePSK("warpnet", v1)
	p2, _ := GeneratePSK("warpnet", v2)
	assert.Equal(t, p1, p2, "minor version should not affect PSK")
}

func TestGeneratePSK_MainnetNormalization(t *testing.T) {
	v := semver.MustParse("1.0.0")
	p1, _ := GeneratePSK("mainnet", v)
	p2, _ := GeneratePSK("warpnet", v)
	assert.Equal(t, p1, p2, "mainnet should be normalized to warpnet")
}

func TestGenerateAnchoredEntropy(t *testing.T) {
	e1 := generateAnchoredEntropy()
	e2 := generateAnchoredEntropy()
	assert.Equal(t, e1, e2, "anchored entropy should be deterministic")
	assert.Len(t, e1, 32)
}

