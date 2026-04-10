package warpnet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCodeBase(t *testing.T) {
	fs := GetCodeBase()
	// The embedded FS contains *.go files
	_, err := fs.ReadFile("embedded.go")
	assert.NoError(t, err)
}

func TestGetVersion(t *testing.T) {
	v := GetVersion()
	assert.NotEmpty(t, v)
}

func TestGetLogo(t *testing.T) {
	logo := GetLogo()
	assert.NotEmpty(t, logo)
}
