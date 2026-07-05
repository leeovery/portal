package portaltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
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

	// Enable the daemon-pgrep sandbox for this test: state.PgrepPortalDaemons
	// will surface ONLY daemons the test registers (SpawnIsolatedDaemon plus the
	// _portal-saver pane PID read by the saver-read helpers). Because the orphan
	// sweep SIGKILLs only PIDs its Pgrep seam returns, this makes it structurally
	// impossible for a sweep to enumerate — and therefore kill — the developer's
	// live daemon (or any process the test did not spawn). No-op in
	// non-integration builds. Reset on cleanup so it cannot bleed across tests.
	state.EnableDaemonSandbox()
	t.Cleanup(state.ResetDaemonSandbox)

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

	// Register this test's state dir with the daemon-pgrep sandbox. The saver
	// daemon that later runs against it (respawns and all) is owned via its
	// live <stateDir>/daemon.pid, so PgrepPortalDaemons surfaces it and the
	// real daemon (a different state dir) stays invisible. No-op in
	// non-integration builds.
	state.RegisterSandboxStateDir(stateDir)

	// Cross-process sandbox registry: subprocesses of this test (the built
	// `portal` binary — always compiled with -tags integration by
	// portalbintest) run their own PgrepPortalDaemons enumerations, where the
	// in-process registration above does not exist. The registry file extends
	// the same default-deny ownership across the process boundary: the env
	// var (set via t.Setenv BEFORE the env slice is derived from os.Environ,
	// so both the returned slice and any careless os.Environ-inheriting
	// subprocess carry it) points at a file of owned state dirs that
	// integration-built binaries consult on every enumeration.
	// SpawnIsolatedDaemon appends each orphan's state dir to the same file.
	registryPath := filepath.Join(tempDir, "sandbox-registry")
	if err := os.WriteFile(registryPath, []byte(stateDir+"\n"), 0o600); err != nil {
		t.Fatalf("portaltest: write sandbox registry: %v", err)
	}
	t.Setenv(state.SandboxRegistryEnv, registryPath)

	env = filterXDGConfigHome(os.Environ())
	env = append(env, "XDG_CONFIG_HOME="+configDir)

	// TMUX POISON — default-deny for the tmux server, mirroring the
	// TestMain PORTAL_*_FILE path-poison. Test processes usually run inside
	// the developer's real tmux, so a subprocess inheriting the ambient
	// TMUX silently attaches to the REAL server: a `portal state daemon`
	// orphan then ticks against (and capture-panes!) the developer's live
	// sessions — the systemic isolation breach + load-flakiness source
	// found while de-flaking the kill-barrier test. Subprocesses that need
	// a server MUST append their own TMUX=<test socket>,... (exec.Cmd
	// dedupes env last-wins, so appending after this slice overrides
	// cleanly; SpawnIsolatedDaemon does it via its socketPath parameter).
	// A subprocess that forgets fails LOUDLY ("error connecting to
	// /nonexistent/...") instead of silently reaching the real server via
	// the ambient TMUX or the default-socket fallback.
	env = filterEnvKeys(env, "TMUX", "TMUX_PANE")
	env = append(env, "TMUX="+PoisonedTmuxSocket+",0,0")

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

// PoisonedTmuxSocket is the deliberately-invalid tmux socket path baked
// into IsolateStateForTest's returned env as `TMUX=<this>,0,0`. Any
// subprocess that should talk to a test server must override TMUX with
// its test socket (append after the slice — exec.Cmd env dedupe is
// last-wins); one that forgets fails loudly at connect time instead of
// silently reaching the developer's real server.
const PoisonedTmuxSocket = "/nonexistent/portal-test-must-set-tmux-socket"

// filterXDGConfigHome returns a copy of env with every entry whose
// key equals XDG_CONFIG_HOME removed. Matches the empty-value edge
// case (XDG_CONFIG_HOME="") because the prefix check uses the full
// "KEY=" form.
func filterXDGConfigHome(env []string) []string {
	return filterEnvKeys(env, "XDG_CONFIG_HOME")
}

// filterEnvKeys returns a copy of env with every entry whose key equals
// one of keys removed. Matches empty-value entries (KEY=) because the
// prefix check uses the full "KEY=" form.
func filterEnvKeys(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		drop := false
		for _, k := range keys {
			if strings.HasPrefix(e, k+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, e)
		}
	}
	return out
}
