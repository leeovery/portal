//go:build integration

// End-to-end integration coverage for the bootstrap step-11
// stale-hook cleanup path under the tmux `list-panes -a` transient
// that motivated this workstream. The unit tests in
// cmd/bootstrap_production_test.go and cmd/clean_test.go close the
// failure modes at the adapter level; this file closes them
// end-to-end against a real tmux server driven through the production
// buildProductionOrchestrator, exercising:
//
//   - the orchestrator's eleven-step ordering and soft-warning wiring
//     (step 11 must NOT escalate to a fatal abort under either mode),
//   - the production *cleanStaleAdapter (composition: real
//     *tmux.Client + real *hooks.Store + real *state.Logger),
//   - the Change 4 logging contract (entry-point Debug vs terminal
//     Warn distinguishing mode (a) from mode (b)),
//   - the byte-identical hooks.json invariant ("no wipe").
//
// Three subtests pin the three coverage rows:
//
//   - mode_a_list_panes_exit_nonzero — `list-panes -a` returns
//     ("", err); step 11 hits the err-from-ListAllPanes branch, emits
//     the propagated-error Warn, and returns BEFORE the entry-point
//     Debug. hooks.json byte-identical.
//   - mode_b_list_panes_empty_stdout — `list-panes -a` returns
//     ("", nil); step 11 parses zero live panes, emits the entry-point
//     Debug (live=0 persisted=N), then the hazard-guard Warn skips the
//     destructive store.CleanStale. hooks.json byte-identical.
//   - normal_path_legitimate_stale_removal_still_works — pass-through
//     Commander; a stale entry whose paneKey is not represented by any
//     live pane is removed while live-pane-backed entries survive. The
//     completion Debug (removed=1) fires.
//
// Package: this file lives in `package cmd` so it can call the
// unexported `buildProductionOrchestrator` and override the
// unexported `commanderFactory` seam (declared in
// cmd/bootstrap_production.go).
//
// Helper duplication: the Phase 3-1 scaffolding file
// (cmd/bootstrap/transient_listpanes_helpers_integration_test.go)
// declares `transientListPanesCommander`, `seedHooksJSON`,
// `hooksJSONBytes`, and `tailPortalLog` under `package bootstrap_test`.
// Symbols defined in another package's `_test.go` files are NOT
// importable across packages (even under the same package name), so
// the small handful of shapes this file needs are duplicated locally
// rather than re-shape the Phase 3-1 file. The duplication is bounded
// (~60 lines) and the shapes are stable; the more invasive
// alternatives (promote helpers to a non-`_test.go` file in
// `package bootstrap`, or move them to a new test-only sub-package)
// would touch unrelated wiring without buying material additional
// safety. See the parent plan task's Option B/C/D discussion.
//
// Isolation: every subtest calls portaltest.IsolateStateForTest(t) to
// scrub host env and install the fingerprint-diff backstop. Each
// subtest gets its own tmuxtest.Socket so the isolated tmux server is
// torn down cleanly. PORTAL_STATE_DIR is set on the test process so
// any subprocess spawned by the orchestrator (notably the
// `_portal-saver` pane's `portal state daemon`) resolves the same
// isolated state directory. PORTAL_LOG_LEVEL=debug ensures step 11's
// Debug breadcrumbs land in portal.log (default level is LevelWarn,
// which would suppress them and false-negative the assertions).
//
// PATH: the orchestrator's step 5 (EnsureSaver) spawns the `portal`
// binary inside the tmux server's pane. portalbintest.StagePortalBinary
// puts a freshly-built `portal` on PATH so that subprocess resolves
// correctly; the binary spawn happens inside the test process's
// scrubbed environment, so the daemon writes to the isolated state
// directory.
//
// No t.Parallel: cmd-package convention (mock-injection via
// package-level mutable state cleaned up by t.Cleanup) applies. This
// file mutates `commanderFactory`, restored in t.Cleanup.

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// failureMode selects the per-test policy applied by
// transientListPanesCommander when it observes a `list-panes -a`
// invocation. Mirrors the type in
// cmd/bootstrap/transient_listpanes_helpers_integration_test.go;
// duplicated locally because that file lives in `package bootstrap_test`
// and is not importable across packages.
type failureMode int

const (
	// passThrough disables interception — every call (including
	// `list-panes -a`) delegates to the inner Commander verbatim.
	passThrough failureMode = iota
	// failExitNonZero makes intercepted calls return ("", error) —
	// modelling tmux exit ≠ 0 on the wire.
	failExitNonZero
	// failEmptyStdout makes intercepted calls return ("", nil) —
	// modelling tmux exit 0 with empty stdout.
	failEmptyStdout
)

// transientListPanesCommander wraps an inner tmux.Commander and
// intercepts only invocations matching `list-panes` with the `-a`
// flag. All other invocations (including `list-panes` without `-a`,
// `list-windows -a`, capture-pane, etc.) are delegated to the inner
// Commander verbatim — preserving production fidelity for every
// non-target tmux call.
//
// Sticky failure (OneShot=false): every intercepted call applies the
// policy. The orchestrator's step 9 (CleanStaleMarkers) and step 11
// (CleanStale) BOTH call `list-panes -a`; sticky failure ensures step
// 11 — the target of this test — observes the policy regardless of
// step-ordering details. Assertions filter on the "stale-hook cleanup:"
// prefix so step 9's own log lines (which use a different prefix) do
// not contaminate the assertion surface.
//
// Mirrors the type in cmd/bootstrap/transient_listpanes_helpers_integration_test.go;
// duplicated locally because that file lives in `package bootstrap_test`.
type transientListPanesCommander struct {
	Inner       tmux.Commander
	Mode        failureMode
	intercepted atomic.Int64
}

// shouldIntercept reports whether the supplied tmux argv targets
// `list-panes -a`.
func (c *transientListPanesCommander) shouldIntercept(args []string) bool {
	if len(args) == 0 || args[0] != "list-panes" {
		return false
	}
	for _, a := range args[1:] {
		if a == "-a" {
			return true
		}
	}
	return false
}

// applyPolicy applies the per-mode policy AFTER the intercept gate.
func (c *transientListPanesCommander) applyPolicy() (string, error) {
	switch c.Mode {
	case failExitNonZero:
		return "", fmt.Errorf("tmux list-panes -a: exit 1 (simulated transient)")
	case failEmptyStdout:
		return "", nil
	default:
		return "", fmt.Errorf("transientListPanesCommander: unexpected mode %d", c.Mode)
	}
}

// intercept centralises the Mode dispatch shared by Run and RunRaw.
func (c *transientListPanesCommander) intercept(args []string) (string, error, bool) {
	if c.Mode == passThrough {
		return "", nil, false
	}
	if !c.shouldIntercept(args) {
		return "", nil, false
	}
	c.intercepted.Add(1)
	out, err := c.applyPolicy()
	return out, err, true
}

// Run implements tmux.Commander. Intercepts `list-panes -a` per the
// configured policy and delegates every other call to Inner.
func (c *transientListPanesCommander) Run(args ...string) (string, error) {
	if out, err, handled := c.intercept(args); handled {
		return out, err
	}
	return c.Inner.Run(args...)
}

// RunRaw implements tmux.Commander. Intercepts `list-panes -a` per
// the configured policy and delegates every other call to Inner.
func (c *transientListPanesCommander) RunRaw(args ...string) (string, error) {
	if out, err, handled := c.intercept(args); handled {
		return out, err
	}
	return c.Inner.RunRaw(args...)
}

// socketCommander is a tmux.Commander targeting a specific tmux
// socket via `tmux -S <path>`. The `-f /dev/null` flag suppresses the
// user's ~/.tmux.conf so the production-builder's tmux invocations do
// not pull in user config that would couple this test to the
// developer's environment. Errors are wrapped via
// tmux.WrapCommandError so any callers that errors.As against
// *tmux.CommandError continue to work — matches the production
// tmux.RealCommander error-wrapping shape end-to-end.
type socketCommander struct {
	socketPath string
}

func (s *socketCommander) runArgs(args []string) []string {
	return append([]string{"-S", s.socketPath, "-f", "/dev/null"}, args...)
}

func (s *socketCommander) Run(args ...string) (string, error) {
	out, err := exec.Command("tmux", s.runArgs(args)...).Output()
	if err != nil {
		return "", tmux.WrapCommandError(err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *socketCommander) RunRaw(args ...string) (string, error) {
	out, err := exec.Command("tmux", s.runArgs(args)...).Output()
	if err != nil {
		return "", tmux.WrapCommandError(err)
	}
	return string(out), nil
}

// Compile-time guards: both Commanders must satisfy tmux.Commander.
var (
	_ tmux.Commander = (*transientListPanesCommander)(nil)
	_ tmux.Commander = (*socketCommander)(nil)
)

// resolveHooksFilePathFromEnv mirrors cmd/config.go's configFilePath
// resolution chain but consumes the env slice returned by
// portaltest.IsolateStateForTest rather than os.Getenv. The chain:
//
//  1. If env contains PORTAL_HOOKS_FILE=<path>, return <path> verbatim.
//  2. Otherwise scan env for XDG_CONFIG_HOME=<dir> and return
//     <dir>/portal/hooks.json.
//  3. Otherwise t.Fatalf — signals isolation regression.
func resolveHooksFilePathFromEnv(t *testing.T, env []string) string {
	t.Helper()
	const (
		hooksFileKey = "PORTAL_HOOKS_FILE="
		xdgKey       = "XDG_CONFIG_HOME="
	)
	var xdg string
	for _, e := range env {
		if strings.HasPrefix(e, hooksFileKey) {
			return strings.TrimPrefix(e, hooksFileKey)
		}
		if strings.HasPrefix(e, xdgKey) {
			xdg = strings.TrimPrefix(e, xdgKey)
		}
	}
	if xdg == "" {
		t.Fatalf("resolveHooksFilePathFromEnv: env slice contains neither PORTAL_HOOKS_FILE nor XDG_CONFIG_HOME — IsolateStateForTest isolation regression")
	}
	return filepath.Join(xdg, "portal", "hooks.json")
}

// seedHooksJSON writes a populated hooks.json under the resolved
// config path using the production hooks.Store so the on-disk shape
// matches what `portal hooks set --on-resume` would produce at
// runtime.
func seedHooksJSON(t *testing.T, env []string, entries map[string]string) {
	t.Helper()
	path := resolveHooksFilePathFromEnv(t, env)
	t.Logf("seedHooksJSON: resolved hooks.json path = %s", path)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("seedHooksJSON: mkdir %s: %v", filepath.Dir(path), err)
	}

	store := hooks.NewStore(path)
	for key, cmd := range entries {
		if err := store.Set(key, "on-resume", cmd); err != nil {
			t.Fatalf("seedHooksJSON: set %s=%q: %v", key, cmd, err)
		}
	}
}

// hooksJSONBytes returns the raw on-disk bytes of hooks.json resolved
// from the test-isolated env. Used for byte-identical before/after
// comparisons. ENOENT returns a nil slice.
func hooksJSONBytes(t *testing.T, env []string) []byte {
	t.Helper()
	path := resolveHooksFilePathFromEnv(t, env)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("hooksJSONBytes: read %s: %v", path, err)
	}
	return data
}

// setupTransientCleanStaleEnv builds the per-subtest scaffolding.
// Side effects:
//   - calls portaltest.IsolateStateForTest (scrubs HOME / XDG, installs
//     fingerprint backstop),
//   - sets PORTAL_STATE_DIR on the test process so subprocesses
//     resolve the same isolated dir,
//   - sets PORTAL_LOG_LEVEL=debug so step 11's Debug breadcrumbs land
//     in portal.log,
//   - stages the portal binary on PATH so step 5 (EnsureSaver) can
//     spawn `portal state daemon` inside the saver pane,
//   - constructs the isolated tmux socket.
func setupTransientCleanStaleEnv(t *testing.T, sockPrefix string) (env []string, stateDir string, sock *tmuxtest.Socket) {
	t.Helper()

	tmuxtest.SkipIfNoTmux(t)

	_ = portalbintest.StagePortalBinary(t)

	env, stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_LOG_LEVEL", "debug")

	// IsolateStateForTest sets XDG_CONFIG_HOME="" on the test process
	// (load-bearing for its fingerprint backstop), pushing the
	// isolated XDG_CONFIG_HOME=<configDir> into the env slice for
	// subprocesses only. The orchestrator under test runs IN the test
	// process, so cmd-package hook-store path resolution (configFilePath
	// → xdg.ConfigBase → $XDG_CONFIG_HOME) would otherwise resolve to
	// the test process's HOME-based fallback and miss the seeded
	// hooks.json. Override XDG_CONFIG_HOME on the test process too —
	// IsolateStateForTest's pre-snapshot already ran, so this
	// post-snapshot Setenv does not perturb the backstop.
	configDir := configDirFromEnvSlice(t, env)
	t.Setenv("XDG_CONFIG_HOME", configDir)

	sock = tmuxtest.New(t, sockPrefix)

	return env, stateDir, sock
}

// configDirFromEnvSlice extracts the XDG_CONFIG_HOME value from the
// env slice produced by portaltest.IsolateStateForTest. The slice
// always contains exactly one such entry — its absence signals an
// isolation regression worth a fatal test failure.
func configDirFromEnvSlice(t *testing.T, env []string) string {
	t.Helper()
	const key = "XDG_CONFIG_HOME="
	for _, e := range env {
		if strings.HasPrefix(e, key) {
			return strings.TrimPrefix(e, key)
		}
	}
	t.Fatalf("configDirFromEnvSlice: XDG_CONFIG_HOME not present in env slice — IsolateStateForTest contract regression")
	return ""
}

// installTransientCommanderFactory installs a commanderFactory that
// wires a transientListPanesCommander wrapping a socket-anchored
// Commander targeting the test's isolated tmux socket. The factory is
// reverted on test exit via t.Cleanup.
func installTransientCommanderFactory(t *testing.T, socketPath string, mode failureMode) {
	t.Helper()
	prev := commanderFactory
	commanderFactory = func() tmux.Commander {
		return &transientListPanesCommander{
			Inner: &socketCommander{socketPath: socketPath},
			Mode:  mode,
		}
	}
	t.Cleanup(func() { commanderFactory = prev })
}

// staleHookCleanupLogLines returns the subset of portal.log lines
// that carry the step-11 stale-hook cleanup prefix. Filtering on the
// prefix excludes step 9's CleanStaleMarkers log lines (which have
// their own prefix) and any unrelated bootstrap noise, narrowing the
// assertion surface to exactly the step under test.
func staleHookCleanupLogLines(portalLog string) []string {
	const prefix = "stale-hook cleanup:"
	var matches []string
	for _, line := range strings.Split(portalLog, "\n") {
		if strings.Contains(line, prefix) {
			matches = append(matches, line)
		}
	}
	return matches
}

// containsLineMatching reports whether any line contains every
// substring in needles (AND semantics).
func containsLineMatching(lines []string, needles ...string) bool {
	for _, line := range lines {
		matched := true
		for _, n := range needles {
			if !strings.Contains(line, n) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// runProductionBootstrap invokes buildProductionOrchestrator and
// runs the eleven-step pipeline. Returns the (serverStarted, warnings,
// err) triple verbatim.
func runProductionBootstrap(t *testing.T) (bool, []bootstrap.Warning, error) {
	t.Helper()
	orch, _ := buildProductionOrchestrator()
	return orch.Run(context.Background())
}

// TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks pins the
// end-to-end invariant: under either failure mode of the
// `list-panes -a` transient, the production bootstrap pipeline must
// (a) not fatally abort, (b) preserve hooks.json byte-for-byte, and
// (c) emit the Change 4 log fingerprint distinguishing mode (a) from
// mode (b). The pass-through subtest verifies the normal path
// continues to work — a legitimate stale entry is removed while live
// entries survive.
func TestBootstrap_CleanStale_TmuxTransient_DoesNotWipeHooks(t *testing.T) {
	t.Run("mode_a_list_panes_exit_nonzero", func(t *testing.T) {
		env, stateDir, sock := setupTransientCleanStaleEnv(t, "ptl-cs-a-")

		seedEntries := map[string]string{
			"alpha:0.0": "echo a",
			"beta:0.0":  "echo b",
			"gamma:0.0": "echo c",
		}
		seedHooksJSON(t, env, seedEntries)
		before := hooksJSONBytes(t, env)
		if len(before) == 0 {
			t.Fatalf("precondition: hooksJSONBytes returned empty slice after seed")
		}

		installTransientCommanderFactory(t, sock.SocketPath(), failExitNonZero)

		_, _, err := runProductionBootstrap(t)
		if err != nil {
			t.Fatalf("bootstrap fatally aborted under mode (a); want nil err (step 11 must Warn-and-swallow): %v", err)
		}

		after := hooksJSONBytes(t, env)
		if !bytes.Equal(before, after) {
			t.Fatalf("hooks.json mutated under mode (a) — the wipe regression has returned\n"+
				"  before: %s\n"+
				"  after:  %s",
				before, after)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log under mode (a); want at least the propagated-error Warn\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "list-panes failed", "simulated transient") {
			t.Fatalf("missing mode (a) propagated-error Warn line; want a `stale-hook cleanup:` line containing `list-panes failed` and `simulated transient`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		for _, line := range lines {
			if strings.Contains(line, "live=") {
				t.Fatalf("mode (a) emitted entry-point Debug (`live=...`); must be absent — the err-from-ListAllPanes branch returns before the Debug emission\n"+
					"  offending line: %s", line)
			}
		}
	})

	t.Run("mode_b_list_panes_empty_stdout", func(t *testing.T) {
		env, stateDir, sock := setupTransientCleanStaleEnv(t, "ptl-cs-b-")

		seedEntries := map[string]string{
			"alpha:0.0": "echo a",
			"beta:0.0":  "echo b",
			"gamma:0.0": "echo c",
		}
		seedHooksJSON(t, env, seedEntries)
		before := hooksJSONBytes(t, env)
		if len(before) == 0 {
			t.Fatalf("precondition: hooksJSONBytes returned empty slice after seed")
		}

		installTransientCommanderFactory(t, sock.SocketPath(), failEmptyStdout)

		_, _, err := runProductionBootstrap(t)
		if err != nil {
			t.Fatalf("bootstrap fatally aborted under mode (b); want nil err (step 11 must Warn-and-swallow): %v", err)
		}

		after := hooksJSONBytes(t, env)
		if !bytes.Equal(before, after) {
			t.Fatalf("hooks.json mutated under mode (b) — the hazard guard failed and the wipe regression has returned\n"+
				"  before: %s\n"+
				"  after:  %s",
				before, after)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log under mode (b); want the entry-point Debug and the hazard-guard Warn\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "live=0", "persisted=3") {
			t.Fatalf("missing mode (b) entry-point Debug; want a `stale-hook cleanup:` line containing `live=0` and `persisted=3`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "zero live panes", "3 hook(s) present", "mass-deletion hazard") {
			t.Fatalf("missing mode (b) hazard-guard Warn; want a `stale-hook cleanup:` line containing `zero live panes`, `3 hook(s) present`, and `mass-deletion hazard`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
	})

	t.Run("normal_path_legitimate_stale_removal_still_works", func(t *testing.T) {
		env, stateDir, sock := setupTransientCleanStaleEnv(t, "ptl-cs-pass-")

		// Pre-create a live tmux session BEFORE installing the
		// passthrough factory. The orchestrator's step 5 (EnsureSaver)
		// will additionally create `_portal-saver`, so the live pane
		// set on entry to step 11 includes at minimum:
		//   - `live:0.0` (this session's first window/pane)
		//   - `_portal-saver:0.0` (if step 5 succeeded; best-effort)
		if _, err := sock.TryRun("new-session", "-d", "-s", "live"); err != nil {
			t.Fatalf("seed live session: %v", err)
		}

		// Seed two entries: one whose paneKey IS live (must survive),
		// one whose paneKey is NOT live (must be removed).
		seedEntries := map[string]string{
			"live:0.0": "echo live",
			"gone:0.0": "echo gone",
		}
		seedHooksJSON(t, env, seedEntries)

		installTransientCommanderFactory(t, sock.SocketPath(), passThrough)

		_, _, err := runProductionBootstrap(t)
		if err != nil {
			t.Fatalf("bootstrap fatally aborted on the normal path; want nil err: %v", err)
		}

		// After the normal path, the live entry must still be
		// present AND the stale entry must have been removed.
		after := hooksJSONBytes(t, env)
		afterStr := string(after)
		if !strings.Contains(afterStr, `"live:0.0"`) {
			t.Fatalf("normal path destroyed the live entry `live:0.0`; want it preserved\n"+
				"  hooks.json after: %s", afterStr)
		}
		if strings.Contains(afterStr, `"gone:0.0"`) {
			t.Fatalf("normal path failed to remove the stale entry `gone:0.0`; want it removed\n"+
				"  hooks.json after: %s", afterStr)
		}

		lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
		if len(lines) == 0 {
			t.Fatalf("no `stale-hook cleanup:` lines found in portal.log on the normal path; want entry-point + completion Debug\n"+
				"  full log:\n%s", portaltest.ReadPortalLogSafe(stateDir))
		}
		// Entry-point Debug: persisted=2 (we seeded two entries).
		// The live= count depends on whether EnsureSaver succeeded —
		// at least 1 (the seeded `live` session's pane). We assert
		// only on persisted=2 to stay robust to saver-step variance.
		if !containsLineMatching(lines, "stale-hook cleanup:", "persisted=2") {
			t.Fatalf("missing normal-path entry-point Debug; want a `stale-hook cleanup:` line containing `persisted=2`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
		// Completion Debug: removed=1 (exactly one stale entry).
		if !containsLineMatching(lines, "stale-hook cleanup:", "removed=1") {
			t.Fatalf("missing normal-path completion Debug; want a `stale-hook cleanup:` line containing `removed=1`\n"+
				"  matched stale-hook lines:\n%s", strings.Join(lines, "\n"))
		}
	})
}
