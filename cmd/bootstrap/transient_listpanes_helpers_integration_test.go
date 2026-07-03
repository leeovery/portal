//go:build integration

// Behavioural verification of the shared transient-list-panes scaffolding
// promoted to internal/transienttest. The scaffolding itself (Commander,
// SocketCommander, SeedHooksJSON, HooksJSONBytes,
// ResolveHooksFilePathFromEnv, FailureMode + variants) lives in the
// internal/transienttest package; this file exercises each helper
// through one concrete code path so a future change that breaks the
// helper contracts surfaces here rather than in the downstream
// portal-clean integration tests.
//
// Isolation invariant: every subtest that touches the state dir or
// spawns a subprocess MUST go through portaltest.IsolateStateForTest
// and apply the returned env slice to every spawned exec.Cmd. The
// helpers under test resolve the config path from the env slice rather
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
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/leeovery/portal/internal/transienttest"
)

// tailPortalLog returns the contents of portal.log under stateDir as a
// string, or the stable `(read portal.log failed: %v)` placeholder when
// the file is unreadable. Thin wrapper around
// portaltest.ReadPortalLogSafe — kept local because it is consumed only
// by the smoke subtest below. ENOENT-tolerant by construction (the
// underlying helper folds ENOENT into the placeholder string).
func tailPortalLog(t *testing.T, stateDir string) string {
	t.Helper()
	return portaltest.ReadPortalLogSafe(stateDir)
}

// TestTransientListPanesHelpers_Smoke is the behavioural verification
// for the transienttest scaffolding. Each subtest exercises one helper
// shape through one concrete code path.
//
// Subtests:
//   - intercepts_list_panes_dash_a_with_exit_nonzero — mode (a) trigger.
//   - intercepts_list_panes_dash_a_with_empty_stdout — mode (b) trigger.
//   - passes_through_unrelated_tmux_commands — `list-windows -a` reaches
//     real tmux on the isolated socket.
//   - seed_and_read_hooks_json_roundtrip — SeedHooksJSON + HooksJSONBytes
//     round-trip a non-empty hook map.
//   - tail_portal_log_handles_missing_file — tailPortalLog tolerates
//     ENOENT and returns the stable placeholder.
//   - isolation_backstop_passes — IsolateStateForTest's fingerprint
//     backstop sees zero delta after a no-op test body.
//   - one_shot_toggle_only_intercepts_first_call — OneShot truthiness.
func TestTransientListPanesHelpers_Smoke(t *testing.T) {
	t.Run("intercepts_list_panes_dash_a_with_exit_nonzero", func(t *testing.T) {
		c := &transienttest.Commander{
			Inner: panicCommander{},
			Mode:  transienttest.FailExitNonZero,
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
		c := &transienttest.Commander{
			Inner: panicCommander{},
			Mode:  transienttest.FailEmptyStdout,
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

		inner := &transienttest.SocketCommander{SocketPath: sock.SocketPath()}
		c := &transienttest.Commander{
			Inner: inner,
			Mode:  transienttest.FailExitNonZero, // policy applies only to list-panes -a
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
		transienttest.SeedHooksJSON(t, env, entries)

		data := transienttest.HooksJSONBytes(t, env)
		if len(data) == 0 {
			t.Fatalf("HooksJSONBytes returned empty slice after seed")
		}

		// Round-trip through the production Store to confirm the
		// on-disk shape is canonical and every seeded entry is
		// present under the on-resume event.
		path := transienttest.ResolveHooksFilePathFromEnv(t, env)
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

		// A second HooksJSONBytes call must return the SAME bytes
		// (no time-dependent serialization).
		data2 := transienttest.HooksJSONBytes(t, env)
		if !bytes.Equal(data, data2) {
			t.Fatalf("HooksJSONBytes not deterministic across reads:\n  first:  %s\n  second: %s", data, data2)
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

		inner := &transienttest.SocketCommander{SocketPath: sock.SocketPath()}
		c := &transienttest.Commander{
			Inner:   inner,
			Mode:    transienttest.FailExitNonZero,
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
