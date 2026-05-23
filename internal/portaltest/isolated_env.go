// Test-only. Importing this package from non-*_test.go files is
// prohibited — the *testing.T parameter enforces this structurally.

package portaltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NewIsolatedStateEnv returns an env slice and stateDir scoped to a
// per-test t.TempDir(). Callers assign env directly to exec.Cmd.Env
// when launching subprocesses (typically `portal state daemon`) so
// the spawned process resolves XDG_CONFIG_HOME — and therefore all
// portal state paths — under the isolated temp directory.
//
// The returned env is derived from os.Environ(), filtered to remove
// any inherited XDG_CONFIG_HOME entry (including XDG_CONFIG_HOME="")
// and with a single XDG_CONFIG_HOME=<tempDir>/config appended. All
// other inherited variables (HOME, PATH, etc.) are preserved
// verbatim.
//
// stateDir resolves to <XDG_CONFIG_HOME>/portal/state and is
// MkdirAll'd before return (0o700) so callers that immediately stat
// or read from it do not race the subprocess. The configDir itself
// is also created with 0o700.
//
// Errors from MkdirAll are fatal via t.Fatalf — this helper exists
// solely to make daemon tests safe, and a failure here means the
// test cannot proceed.
//
// The *testing.T parameter is intentional: it enforces test-only
// usage structurally. Production code cannot import this package
// without dragging in the testing stdlib package, which would fail
// `go build .` for the main binary.
func NewIsolatedStateEnv(t *testing.T) (env []string, stateDir string) {
	t.Helper()

	// Capture the developer's real state-dir path and snapshot it
	// BEFORE mutating any env. The snapshot resolves XDG_CONFIG_HOME
	// via the live test-process env so the backstop targets the
	// developer's actual install — not the temp dir the helper is
	// about to construct. A failure to snapshot is fatal: a silently
	// degraded backstop is worse than no backstop.
	devStateDir := resolveDevStateDir()
	var preSnapshot map[string]Fingerprint
	if devStateDir != "" {
		snap, err := SnapshotStateDir(devStateDir)
		if err != nil {
			t.Fatalf("portaltest: snapshot dev state dir %s: %v", devStateDir, err)
		}
		preSnapshot = snap
	}

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("portaltest: mkdir configDir: %v", err)
	}

	stateDir = filepath.Join(configDir, "portal", "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("portaltest: mkdir stateDir: %v", err)
	}

	env = filterXDGConfigHome(os.Environ())
	env = append(env, "XDG_CONFIG_HOME="+configDir)

	// Backstop: on test exit, walk the dev state dir again and fail
	// the host *testing.T if anything diverged from the pre-snapshot.
	// Skips when devStateDir was unresolvable (HOME unset) — the
	// env override is then the sole defence, which is acceptable in
	// CI containers that have no $HOME.
	if devStateDir != "" {
		installBackstopCleanup(t, devStateDir, preSnapshot)
	}

	return env, stateDir
}

// backstopT narrows the *testing.T surface to exactly what
// installBackstopCleanup needs: Cleanup(fn) to register the
// post-test hook, and Errorf(format, args...) to surface a
// detected mutation. The narrow interface lets a fake recorder
// drive installBackstopCleanup in unit tests without constructing
// a real *testing.T.
type backstopT interface {
	Cleanup(fn func())
	Errorf(format string, args ...any)
}

// installBackstopCleanup registers a t.Cleanup that re-snapshots
// devStateDir on test exit and calls t.Errorf for every delta
// against pre. Pure wiring — the diff logic lives in
// reportStateDirDelta and is exercised independently.
func installBackstopCleanup(t backstopT, devStateDir string, pre map[string]Fingerprint) {
	t.Cleanup(func() {
		reportStateDirDelta(t.Errorf, devStateDir, pre)
	})
}

// filterXDGConfigHome returns a copy of env with every entry whose
// key equals XDG_CONFIG_HOME removed. Matches the empty-value edge
// case (XDG_CONFIG_HOME="") because the prefix check uses the full
// "KEY=" form.
func filterXDGConfigHome(env []string) []string {
	const prefix = "XDG_CONFIG_HOME="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}
