//go:build integration

// Shared scaffolding for Phase 3 integration tests that reproduce a
// tmux transient where `list-panes -a` either exits non-zero (failure
// mode (a)) or returns exit 0 with empty stdout (failure mode (b)).
//
// This file is the canonical Commander-injection primitive mandated by
// spec § "Deterministic Repro Mechanism" and consumed by Tasks 3-2
// (bootstrap end-to-end) and 3-3 (`portal clean` end-to-end). It
// exposes four helpers with stable signatures:
//
//   1. transientListPanesCommander — wraps an inner tmux.Commander and
//      intercepts only `list-panes -a` calls based on a per-test
//      policy; everything else delegates verbatim. Default policy is
//      sticky failure (every intercepted call fails until the test
//      explicitly flips the mode). A OneShot toggle is exposed so
//      Tasks 3-2 / 3-3 can isolate which bootstrap step observes the
//      transient.
//   2. seedHooksJSON — writes a populated hooks.json into the
//      isolated config tree resolved from the env slice returned by
//      portaltest.IsolateStateForTest. Uses the production
//      internal/hooks package so the on-disk shape stays canonical.
//   3. hooksJSONBytes — raw os.ReadFile against the same resolved
//      path, used for the byte-identical before/after assertion that
//      pins the "no wipe" invariant.
//   4. tailPortalLog — thin wrapper around portaltest.ReadPortalLogSafe
//      for substring-match assertions on Warn / Debug breadcrumbs in
//      the daemon's audit trail.
//
// Isolation invariant: every subtest that touches the state dir or
// spawns a subprocess MUST go through portaltest.IsolateStateForTest
// and apply the returned env slice to every spawned exec.Cmd. The
// helpers below resolve the config path from that env slice rather
// than os.Getenv so they remain robust to future changes in which env
// vars IsolateStateForTest overrides.
//
// No t.Parallel: cmd-package convention (mock-injection via package-
// level mutable state cleaned up by t.Cleanup) applies here even
// though this file does not itself use package-level mocks — keeping
// it serial avoids cross-test contention against the shared
// portaltest fingerprint-backstop infrastructure.

package bootstrap_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// failureMode selects the per-test policy applied by
// transientListPanesCommander when it observes a `list-panes -a`
// invocation. passThrough disables the interception entirely so a
// single Commander instance can be constructed and rotated between
// modes during a test without rebuilding the wrapping chain.
type failureMode int

const (
	// passThrough disables interception — every call (including
	// `list-panes -a`) delegates to the inner Commander verbatim.
	passThrough failureMode = iota
	// failExitNonZero makes intercepted calls return
	// ("", error) — modelling tmux exit ≠ 0 on the wire.
	failExitNonZero
	// failEmptyStdout makes intercepted calls return ("", nil) —
	// modelling tmux exit 0 with empty stdout (the mode (b) trigger
	// for the bootstrap hazard guard).
	failEmptyStdout
)

// transientListPanesCommander wraps an inner tmux.Commander and
// intercepts only invocations matching `list-panes` with the `-a`
// flag. All other invocations (including `list-panes` without `-a`,
// `list-windows -a`, capture-pane, etc.) are delegated to the inner
// Commander verbatim — preserving production fidelity for every
// non-target tmux call.
//
// Policy semantics:
//   - mode == passThrough: no interception, inner Commander handles
//     every call.
//   - mode == failExitNonZero: intercepted calls return
//     ("", fmt.Errorf("tmux list-panes -a: exit 1 (simulated transient)")).
//   - mode == failEmptyStdout: intercepted calls return ("", nil).
//
// The OneShot toggle is the lever Tasks 3-2 / 3-3 use when they need
// step 4 (orphan sweep) to succeed before step 11 (CleanStale)
// observes the transient. When OneShot is true, the FIRST intercepted
// call applies the policy; every subsequent intercepted call falls
// through to the inner Commander. When OneShot is false (the
// default), every intercepted call applies the policy — "sticky
// failure" matching the prevailing semantics across this file's
// consumers.
//
// Concurrent-safety: the interception counter uses atomic.Int64 so
// the OneShot toggle is safe under the parallel `tmux ...` calls
// that bootstrap step 4 (orphan sweep) and step 11 (CleanStale)
// may issue. The Mode and Inner fields are NOT protected because
// tests are expected to flip them only between phases, not during
// concurrent tmux activity.
type transientListPanesCommander struct {
	// Inner is the downstream Commander. Defaults at construction
	// time to &tmux.RealCommander{} in production-fidelity tests;
	// integration tests targeting an isolated tmux server should
	// wire a socket-anchored Commander here instead.
	Inner tmux.Commander
	// Mode selects the interception policy. Default zero value is
	// passThrough — explicitly require the test to opt in to a
	// failure policy.
	Mode failureMode
	// OneShot, when true, causes only the first intercepted call to
	// apply the policy. Subsequent intercepted calls delegate to
	// Inner verbatim. Default false (sticky failure).
	OneShot bool

	// intercepted is the atomic counter backing the OneShot toggle.
	// Zero-value means "no intercepted calls observed yet".
	intercepted atomic.Int64
}

// shouldIntercept reports whether the supplied tmux argv targets
// `list-panes -a`. The check is positional on argv[0] and a substring
// scan for "-a" in the remaining args — matching the production
// callsites (tmux.ListAllPanesWithFormat and bootstrap step 4's
// orphan-sweep pgrep precondition).
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

// applyPolicy applies the per-mode policy AFTER the OneShot gate has
// decided this invocation is the one to act on. Returns the
// (output, error) pair every Run / RunRaw caller expects.
func (c *transientListPanesCommander) applyPolicy() (string, error) {
	switch c.Mode {
	case failExitNonZero:
		return "", fmt.Errorf("tmux list-panes -a: exit 1 (simulated transient)")
	case failEmptyStdout:
		return "", nil
	case passThrough:
		// Should not be reached — passThrough is filtered before
		// applyPolicy via the caller's policy check. Defensive
		// fall-through to a clear error so a future regression
		// surfaces immediately rather than silently degrading.
		return "", fmt.Errorf("transientListPanesCommander: applyPolicy called with passThrough mode")
	default:
		return "", fmt.Errorf("transientListPanesCommander: unknown failure mode %d", c.Mode)
	}
}

// intercept centralises the OneShot / Mode dispatch shared by Run
// and RunRaw. Returns (output, error, true) when the policy applied,
// or ("", nil, false) when the caller should delegate to Inner.
func (c *transientListPanesCommander) intercept(args []string) (string, error, bool) {
	if c.Mode == passThrough {
		return "", nil, false
	}
	if !c.shouldIntercept(args) {
		return "", nil, false
	}
	n := c.intercepted.Add(1)
	if c.OneShot && n > 1 {
		return "", nil, false
	}
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
// the configured policy and delegates every other call to Inner —
// scrollback-capturing paths in bootstrap depend on RunRaw fidelity.
func (c *transientListPanesCommander) RunRaw(args ...string) (string, error) {
	if out, err, handled := c.intercept(args); handled {
		return out, err
	}
	return c.Inner.RunRaw(args...)
}

// resolveHooksFilePathFromEnv mirrors cmd/config.go's configFilePath
// resolution chain but consumes the env slice returned by
// portaltest.IsolateStateForTest rather than os.Getenv. The chain:
//
//  1. If env contains PORTAL_HOOKS_FILE=<path>, return <path> verbatim.
//  2. Otherwise scan env for XDG_CONFIG_HOME=<dir> and return
//     <dir>/portal/hooks.json.
//  3. Otherwise t.Fatalf — signals isolation regression.
//
// Returning early on PORTAL_HOOKS_FILE matches the production
// behaviour where the env-var override takes precedence over
// XDG-derived defaults.
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
// config path. The supplied entries map is interpreted as
// {structuralKey: onResumeCommand} — for each entry, a single
// on-resume hook is registered via the production hooks.Store so the
// on-disk JSON layout matches what `portal hooks set --on-resume`
// would produce at runtime.
//
// The resolved path is t.Logf'd to verify the seed lands under the
// isolated tree, per the project's daemon-test isolation rule.
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
// comparisons that pin the "no wipe" invariant. Fails the test on
// any read error other than ENOENT — callers asserting on byte
// identity have no meaningful answer if the file is unreadable.
//
// ENOENT returns a nil slice so a missing-file precondition can be
// distinguished from a present-but-empty file (bytes.Equal handles
// both cases the same way, but the caller may want to check the
// distinction).
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

// tailPortalLog returns the contents of portal.log under stateDir as
// a string, or the stable `(read portal.log failed: %v)` placeholder
// when the file is unreadable. Thin wrapper around
// portaltest.ReadPortalLogSafe — exists so Tasks 3-2 / 3-3 can call
// the same helper name as documented in the plan rather than reaching
// into portaltest directly. ENOENT-tolerant by construction (the
// underlying helper folds ENOENT into the placeholder string).
func tailPortalLog(t *testing.T, stateDir string) string {
	t.Helper()
	return portaltest.ReadPortalLogSafe(stateDir)
}

// Compile-time guard: transientListPanesCommander must satisfy
// tmux.Commander so the Task 3-2 / 3-3 wiring (which builds a
// *tmux.Client from this stub) compiles even if Commander gains
// methods.
var _ tmux.Commander = (*transientListPanesCommander)(nil)

// passthroughSocketCommander is a tiny tmux.Commander that targets
// the test's isolated tmux socket via `tmux -S <path>`. Used only by
// the pass-through smoke subtest below — production code paths build
// the Inner Commander differently (either &tmux.RealCommander{} or
// the unexported tmuxtest socketCommander accessible via Socket.Client).
//
// We construct one locally rather than reaching into the unexported
// tmuxtest socketCommander so this file does not couple to internal
// tmuxtest details. The shape is intentionally minimal: it just
// shells out to tmux with the -S socket prefix.
type passthroughSocketCommander struct {
	socketPath string
}

func (p *passthroughSocketCommander) runArgs(args []string) []string {
	return append([]string{"-S", p.socketPath, "-f", "/dev/null"}, args...)
}

func (p *passthroughSocketCommander) Run(args ...string) (string, error) {
	out, err := exec.Command("tmux", p.runArgs(args)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (p *passthroughSocketCommander) RunRaw(args ...string) (string, error) {
	out, err := exec.Command("tmux", p.runArgs(args)...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// TestTransientListPanesHelpers_Smoke is the behavioural verification
// for this scaffolding file. It exercises each helper through one
// concrete code path so a future change that breaks the helper
// contracts surfaces here rather than in Tasks 3-2 / 3-3.
//
// Subtests:
//   - intercepts_list_panes_dash_a_with_exit_nonzero — mode (a) trigger.
//   - intercepts_list_panes_dash_a_with_empty_stdout — mode (b) trigger.
//   - passes_through_unrelated_tmux_commands — `list-windows -a` reaches
//     real tmux on the isolated socket.
//   - seed_and_read_hooks_json_roundtrip — seedHooksJSON + hooksJSONBytes
//     round-trip a non-empty hook map.
//   - tail_portal_log_handles_missing_file — tailPortalLog tolerates
//     ENOENT and returns the stable placeholder.
//   - isolation_backstop_passes — IsolateStateForTest's fingerprint
//     backstop sees zero delta after a no-op test body.
func TestTransientListPanesHelpers_Smoke(t *testing.T) {
	t.Run("intercepts_list_panes_dash_a_with_exit_nonzero", func(t *testing.T) {
		c := &transientListPanesCommander{
			Inner: panicCommander{},
			Mode:  failExitNonZero,
		}
		out, err := c.Run("list-panes", "-a", "-F", "#{pane_id}")
		if err == nil {
			t.Fatalf("expected non-nil error, got nil (out=%q)", out)
		}
		if out != "" {
			t.Fatalf("expected empty out, got %q", out)
		}
		if !strings.Contains(err.Error(), "simulated transient") {
			t.Fatalf("expected error to mention 'simulated transient', got %q", err.Error())
		}

		// RunRaw must apply the same policy.
		outRaw, errRaw := c.RunRaw("list-panes", "-a", "-F", "#{pane_id}")
		if errRaw == nil {
			t.Fatalf("RunRaw: expected non-nil error, got nil (out=%q)", outRaw)
		}
	})

	t.Run("intercepts_list_panes_dash_a_with_empty_stdout", func(t *testing.T) {
		c := &transientListPanesCommander{
			Inner: panicCommander{},
			Mode:  failEmptyStdout,
		}
		out, err := c.Run("list-panes", "-a", "-F", "#{pane_id}")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if out != "" {
			t.Fatalf("expected empty out, got %q", out)
		}
	})

	t.Run("passes_through_unrelated_tmux_commands", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)

		// Create the isolated env BEFORE spinning up tmux so the
		// fingerprint backstop's pre-snapshot covers the entire
		// subtest window.
		_, _ = portaltest.IsolateStateForTest(t)

		sock := tmuxtest.New(t, "ptl-trans-smoke-")
		// Bootstrap a minimal session so list-windows -a has
		// something to enumerate.
		if _, err := sock.TryRun("new-session", "-d", "-s", "smoke"); err != nil {
			t.Fatalf("new-session: %v", err)
		}

		inner := &passthroughSocketCommander{socketPath: sock.SocketPath()}
		c := &transientListPanesCommander{
			Inner: inner,
			Mode:  failExitNonZero, // policy applies only to list-panes -a
		}

		// list-windows -a is NOT intercepted; must reach real tmux
		// and return non-empty output for the seeded session.
		out, err := c.Run("list-windows", "-a")
		if err != nil {
			t.Fatalf("pass-through list-windows -a: unexpected error %v", err)
		}
		if !strings.Contains(out, "smoke") {
			t.Fatalf("pass-through list-windows -a: expected output to mention 'smoke' session, got %q", out)
		}

		// Sanity: list-panes WITHOUT -a is also NOT intercepted.
		// Target the smoke session's pane explicitly so the call
		// succeeds even though no client is attached.
		outNoA, errNoA := c.Run("list-panes", "-t", "smoke")
		if errNoA != nil {
			t.Fatalf("pass-through list-panes -t smoke: unexpected error %v", errNoA)
		}
		if outNoA == "" {
			t.Fatalf("pass-through list-panes -t smoke: expected non-empty output")
		}

		// And the intercepted shape still trips the policy.
		_, errIntercept := c.Run("list-panes", "-a")
		if errIntercept == nil {
			t.Fatalf("expected list-panes -a to be intercepted")
		}
	})

	t.Run("seed_and_read_hooks_json_roundtrip", func(t *testing.T) {
		env, _ := portaltest.IsolateStateForTest(t)

		entries := map[string]string{
			"smoke:0.0": "echo hello",
			"smoke:1.0": "claude --resume",
		}
		seedHooksJSON(t, env, entries)

		data := hooksJSONBytes(t, env)
		if len(data) == 0 {
			t.Fatalf("hooksJSONBytes returned empty slice after seed")
		}

		// Round-trip through the production Store to confirm the
		// on-disk shape is canonical and every seeded entry is
		// present under the on-resume event.
		path := resolveHooksFilePathFromEnv(t, env)
		store := hooks.NewStore(path)
		for key, want := range entries {
			cmd, ok, err := hooks.LookupOnResume(store, key)
			if err != nil {
				t.Fatalf("LookupOnResume(%s): %v", key, err)
			}
			if !ok {
				t.Fatalf("LookupOnResume(%s): not found", key)
			}
			if cmd != want {
				t.Fatalf("LookupOnResume(%s): got %q, want %q", key, cmd, want)
			}
		}

		// A second hooksJSONBytes call must return the SAME bytes
		// (no time-dependent serialization).
		data2 := hooksJSONBytes(t, env)
		if !bytes.Equal(data, data2) {
			t.Fatalf("hooksJSONBytes not deterministic across reads:\n  first:  %s\n  second: %s", data, data2)
		}
	})

	t.Run("tail_portal_log_handles_missing_file", func(t *testing.T) {
		_, stateDir := portaltest.IsolateStateForTest(t)

		// portal.log does not exist under a fresh isolated state
		// dir — tailPortalLog must NOT fail; it must return the
		// stable placeholder shape from ReadPortalLogSafe.
		out := tailPortalLog(t, stateDir)
		if !strings.HasPrefix(out, "(read portal.log failed:") {
			t.Fatalf("expected ENOENT placeholder prefix, got %q", out)
		}
	})

	t.Run("isolation_backstop_passes", func(t *testing.T) {
		// IsolateStateForTest registers a fingerprint-diff
		// backstop in t.Cleanup. With no test-body activity that
		// touches the developer's state dir, the backstop must
		// observe zero delta and the subtest must pass cleanly.
		_, stateDir := portaltest.IsolateStateForTest(t)
		if stateDir == "" {
			t.Fatalf("IsolateStateForTest returned empty stateDir")
		}
	})

	t.Run("one_shot_toggle_only_intercepts_first_call", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)
		_, _ = portaltest.IsolateStateForTest(t)

		sock := tmuxtest.New(t, "ptl-trans-oneshot-")
		if _, err := sock.TryRun("new-session", "-d", "-s", "oneshot"); err != nil {
			t.Fatalf("new-session: %v", err)
		}

		inner := &passthroughSocketCommander{socketPath: sock.SocketPath()}
		c := &transientListPanesCommander{
			Inner:   inner,
			Mode:    failExitNonZero,
			OneShot: true,
		}

		// First intercepted call: policy applies.
		_, err1 := c.Run("list-panes", "-a")
		if err1 == nil {
			t.Fatalf("first list-panes -a: expected intercepted error, got nil")
		}

		// Second intercepted call: passes through to real tmux.
		out2, err2 := c.Run("list-panes", "-a")
		if err2 != nil {
			t.Fatalf("second list-panes -a: expected pass-through, got error %v", err2)
		}
		if out2 == "" {
			t.Fatalf("second list-panes -a: expected non-empty real-tmux output")
		}
	})
}

// panicCommander is the inner-Commander used by smoke subtests that
// must NOT delegate to a real tmux invocation. Any Run / RunRaw call
// reaching it indicates the interception logic failed to match.
type panicCommander struct{}

func (panicCommander) Run(args ...string) (string, error) {
	panic(fmt.Sprintf("panicCommander.Run reached with args=%v — interception logic regression", args))
}

func (panicCommander) RunRaw(args ...string) (string, error) {
	panic(fmt.Sprintf("panicCommander.RunRaw reached with args=%v — interception logic regression", args))
}
