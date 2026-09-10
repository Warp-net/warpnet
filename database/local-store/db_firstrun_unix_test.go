//go:build unix

//nolint:all
package local_store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRefusesAnUnstattableDirectory(t *testing.T) {
	// a regular file where the database directory should go: the first-run
	// probe cannot tell whether the store was initialised, so it refuses loudly
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, nil, 0o600))

	require.Panics(t, func() { _, _ = New(blocked, DefaultOptions()) })
}
