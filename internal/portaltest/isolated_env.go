package portaltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// IsolateStateForTest returns an env slice and stateDir scoped to a
// per-test t.TempDir(). Callers assign env directly to exec.Cmd.Env
// when launching subprocesses (typically `portal state daemon`) so
// the spawned process resolves XDG_CONFIG_HOME — and therefore all
// portal state paths — under the isolated temp directory.
//
// SIDE EFFECT: this helper MUTATES the calling test process's
// environment via t.Setenv (HOME → fresh t.TempDir(); XDG_CONFIG_HOME
// → ""). The mutation is restored when t.Cleanup fires at test exit.
// This is intentional and load-bearing for the fingerprint-diff
// backstop described below — the verb-shaped name (Isolate…) signals
// at the call site that the test process itself is being mutated.
//
// Host-noise mitigation: this helper neutralizes host env noise by
// re-pointing HOME at a fresh t.TempDir() and clearing
// XDG_CONFIG_HOME on the test process BEFORE snapshotting the
// developer's state dir. Without this, a live host `portal state
// daemon` (the developer's real ~/.config/portal/state/ writer)
// could mutate the snapshot's pre-state during the test window and
// false-positive-trip the backstop's post-test delta check. The
// ordering invariant (env scrub → snapshot → env construction) is
// folded in here so callers cannot forget it; doing this from the
// caller is no longer required.
//
// The returned env is derived from os.Environ() (post-scrub),
// filtered to remove any inherited XDG_CONFIG_HOME entry (including
// XDG_CONFIG_HOME="") and with a single XDG_CONFIG_HOME=<tempDir>/config
// appended. Other inherited variables (PATH, etc.) are preserved
// verbatim. HOME in the returned env reflects the scrubbed value.
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
func IsolateStateForTest(t *testing.T) (env []string, stateDir string) {
	t.Helper()

	// Host-noise mitigation: re-point HOME at a fresh tempdir and
	// clear XDG_CONFIG_HOME on the test process BEFORE the snapshot
	// runs. t.Setenv registers a Cleanup that restores the prior
	// values, so the dev's real env is intact after the test exits.
	// Setting XDG_CONFIG_HOME to "" is functionally equivalent to
	// "unset" for resolveDevStateDir's precedence (`if xdg != ""`).
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	// Capture the developer's real state-dir path and snapshot it
	// AFTER the host-noise scrub but BEFORE constructing the
	// isolated env. With HOME re-pointed at a fresh tempdir and
	// XDG_CONFIG_HOME cleared, resolveDevStateDir returns
	// <tempdir>/.config/portal/state — a quiet path no live host
	// daemon writes to. A failure to snapshot is fatal: a silently
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
