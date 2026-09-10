//go:build !windows

//nolint:all
package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"
)

func TestRelayArtifact(t *testing.T) {
	a := RelayArtifact()
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		require.True(t, a.isSupported())
		require.Equal(t, "relay", a.BinaryName)
		return
	}
	require.False(t, a.isSupported(), "only linux/amd64 publishes a relay asset")
}

func TestArtifactIsSupported(t *testing.T) {
	require.False(t, Artifact{}.isSupported())
	require.False(t, Artifact{AssetName: "a"}.isSupported())
	require.False(t, Artifact{AssetName: "a", ChecksumName: "c"}.isSupported())
	require.True(t, Artifact{AssetName: "a", ChecksumName: "c", BinaryName: "b"}.isSupported())
}

func TestRunIsDisabledWithoutPrerequisites(t *testing.T) {
	supported := Artifact{AssetName: "a", ChecksumName: "c", BinaryName: "b"}

	t.Run("nil updater", func(t *testing.T) {
		var u *SelfUpdater
		require.NotPanics(t, func() { u.Run(nil) })
		require.NotPanics(t, u.Close)
	})

	t.Run("unknown current version", func(t *testing.T) {
		u := &SelfUpdater{ctx: context.Background(), artifact: supported, stopChan: make(chan struct{})}
		u.Run(nil)
		u.Close()
	})

	t.Run("unsupported platform", func(t *testing.T) {
		u := &SelfUpdater{
			ctx: context.Background(), current: semver.MustParse("1.0.0"),
			stopChan: make(chan struct{}),
		}
		u.Run(nil)
		u.Close()
	})

	t.Run("no binary to replace", func(t *testing.T) {
		u := &SelfUpdater{
			ctx: context.Background(), current: semver.MustParse("1.0.0"),
			artifact: supported, stopChan: make(chan struct{}),
		}
		u.Run(nil)
		u.Close()
	})
}

func TestRunTicksAndStops(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, _ := updaterFixture(t, testVersion, archive, sumsFor(archive))

	// The release equals the current version, so the tick is a no-op check —
	// enough to prove the loop runs and then shuts down cleanly.
	u.interval = 10 * time.Millisecond
	u.Run(nil)

	time.Sleep(50 * time.Millisecond)
	u.Close()

	// Close is guarded against a double close by its own recover.
	require.NotPanics(t, u.Close)
}

func TestRunStopsWithContext(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, _ := updaterFixture(t, testVersion, archive, sumsFor(archive))

	ctx, cancel := context.WithCancel(context.Background())
	u.ctx = ctx
	u.interval = 10 * time.Millisecond
	u.Run(nil)

	cancel()
	time.Sleep(30 * time.Millisecond)
	u.Close()
}

func TestTickLogsOrdinaryFailures(t *testing.T) {
	u, _ := updaterFixture(t, "not-a-version", nil, nil)
	// A malformed tag makes Latest fail; tick must swallow it rather than
	// escalating to a fatal restart failure.
	require.NotPanics(t, func() { u.tick(nil) })
}

func TestNewSelfUpdaterResolvesItsOwnBinary(t *testing.T) {
	u := NewSelfUpdater(context.Background(), semver.MustParse("1.0.0"), RelayArtifact())
	require.NotNil(t, u)
	require.NotNil(t, u.binary)
	require.NotNil(t, u.failures)
	require.Equal(t, checkInterval, u.interval)
}

func TestCurrentExecutable(t *testing.T) {
	e, err := currentExecutable()
	require.NoError(t, err)
	require.NotEmpty(t, e.Path())
	require.Equal(t, filepath.Join(filepath.Dir(e.Path()), "asset"), e.StagePath("asset"))
}

func TestInstallRollsBackWhenTheNewBinaryIsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warpnet")
	require.NoError(t, os.WriteFile(path, []byte("current"), 0o755))

	e := &executable{path: path}
	_, err := e.Install(filepath.Join(dir, "does-not-exist"))
	require.Error(t, err)

	// the previous binary must be back in place
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "current", string(data))
}

func TestInstallFailsWhenTheCurrentBinaryIsGone(t *testing.T) {
	e := &executable{path: filepath.Join(t.TempDir(), "absent", "warpnet")}
	_, err := e.Install(filepath.Join(t.TempDir(), "new"))
	require.Error(t, err)
}

func TestInstallRollbackRestoresPreviousBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warpnet")
	require.NoError(t, os.WriteFile(path, []byte("current"), 0o755))

	staged := filepath.Join(dir, "warpnet.new")
	require.NoError(t, os.WriteFile(staged, []byte("next"), 0o755))

	e := &executable{path: path}
	rollback, err := e.Install(staged)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "next", string(data))

	rollback()
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "current", string(data))

	// a second rollback has nothing left to move and only logs
	require.NotPanics(t, rollback)
}

func TestRestartReportsExecFailure(t *testing.T) {
	e := &executable{path: filepath.Join(t.TempDir(), "not-an-executable")}

	stopped := false
	err := e.Restart(func() { stopped = true })
	require.Error(t, err)
	require.True(t, stopped, "the node is released before the process is replaced")
}

func TestFailureMarkerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warpnet")
	m := newFailureMarker(path)
	v := semver.MustParse("1.2.3")

	require.False(t, m.Has(v), "no marker yet")

	m.Set(v)
	require.True(t, m.Has(v))
	require.False(t, m.Has(semver.MustParse("1.2.4")))

	m.Clear()
	require.False(t, m.Has(v))
	// clearing twice is not an error
	require.NotPanics(t, m.Clear)
}

func TestFailureMarkerDiscardsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warpnet")
	m := newFailureMarker(path)
	require.NoError(t, os.WriteFile(m.path, []byte("not-a-version"), markerMode))

	require.False(t, m.Has(semver.MustParse("1.2.3")))
	require.NoFileExists(t, m.path, "a malformed marker is dropped so updates resume")
}

func TestFailureMarkerSetOnAnUnwritablePathIsLogged(t *testing.T) {
	m := newFailureMarker(filepath.Join(t.TempDir(), "absent-dir", "warpnet"))
	require.NotPanics(t, func() { m.Set(semver.MustParse("1.2.3")) })
}

func TestWriteBinaryRejectsUnwritableDestination(t *testing.T) {
	dir := t.TempDir()
	// a directory where the file should go makes the create fail
	dst := filepath.Join(dir, "blocked")
	require.NoError(t, os.Mkdir(dst, 0o755))

	require.Error(t, writeBinary(errReader{}, dst))

	// a read failure mid-copy surfaces too
	require.Error(t, writeBinary(errReader{}, filepath.Join(dir, "binary")))
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
