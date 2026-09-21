//go:build integration

package bootstrap_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/leeovery/portal/internal/transienttest"
)

func tailPortalLog(t *testing.T, stateDir string) string {
	t.Helper()
	return portaltest.ReadPortalLogSafe(stateDir)
}

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

		_, stateDir := portaltest.IsolateStateForTest(t)

		// LIFO runs this wait between kill-server and the TempDir RemoveAll.
		portaltest.RegisterStateDirTeardownGuard(t, stateDir)

		sock := tmuxtest.New(t, "ptl-trans-smoke-")
		if _, err := sock.TryRun("new-session", "-d", "-s", "smoke"); err != nil {
			t.Fatalf("new-session: %v", err)
		}

		inner := &transienttest.SocketCommander{SocketPath: sock.SocketPath()}
		c := &transienttest.Commander{
			Inner: inner,
			Mode:  transienttest.FailExitNonZero,
		}

		out, err := c.Run("list-windows", "-a")
		if err != nil {
			t.Fatalf("pass-through list-windows -a: unexpected error %v", err)
		}
		if !strings.Contains(out, "smoke") {
			t.Fatalf("pass-through list-windows -a: expected output to mention 'smoke' session, got %q", out)
		}

		outNoA, errNoA := c.Run("list-panes", "-t", "smoke")
		if errNoA != nil {
			t.Fatalf("pass-through list-panes -t smoke: unexpected error %v", errNoA)
		}
		if outNoA == "" {
			t.Fatalf("pass-through list-panes -t smoke: expected non-empty output")
		}

		_, errIntercept := c.Run("list-panes", "-a")
		if errIntercept == nil {
			t.Fatalf("expected list-panes -a to be intercepted")
		}
	})

	t.Run("seed_and_read_hooks_json_roundtrip", func(t *testing.T) {
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "portal", "hooks.json"))

		env, _ := portaltest.IsolateStateForTest(t)

		entries := map[string]string{
			hookstest.ReapableSeedA: "echo hello",
			hookstest.ReapableSeedB: "claude --resume",
		}
		hookstest.SeedHooksJSON(t, env, entries)

		data := hookstest.HooksJSONBytes(t, env)
		if len(data) == 0 {
			t.Fatalf("HooksJSONBytes returned empty slice after seed")
		}

		path := hookstest.ResolveHooksFilePathFromEnv(t, env)
		store := hooks.NewStore(path)
		for key, want := range entries {
			got, err := store.LookupOnResume(key, hooks.ViaHydrate)
			if err != nil {
				t.Fatalf("LookupOnResume(%s): %v", key, err)
			}
			if !got.Found {
				t.Fatalf("LookupOnResume(%s): not found", key)
			}
			if got.Command != want {
				t.Fatalf("LookupOnResume(%s): got %q, want %q", key, got.Command, want)
			}
		}

		data2 := hookstest.HooksJSONBytes(t, env)
		if !bytes.Equal(data, data2) {
			t.Fatalf("HooksJSONBytes not deterministic across reads:\n  first:  %s\n  second: %s", data, data2)
		}
	})

	t.Run("tail_portal_log_handles_missing_file", func(t *testing.T) {
		_, stateDir := portaltest.IsolateStateForTest(t)

		out := tailPortalLog(t, stateDir)
		if !strings.HasPrefix(out, "(read portal.log failed:") {
			t.Fatalf("expected ENOENT placeholder prefix, got %q", out)
		}
	})

	t.Run("isolation_backstop_passes", func(t *testing.T) {
		_, stateDir := portaltest.IsolateStateForTest(t)
		if stateDir == "" {
			t.Fatalf("IsolateStateForTest returned empty stateDir")
		}
	})

	t.Run("one_shot_toggle_only_intercepts_first_call", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)
		_, stateDir := portaltest.IsolateStateForTest(t)

		// LIFO runs this wait between kill-server and the TempDir RemoveAll.
		portaltest.RegisterStateDirTeardownGuard(t, stateDir)

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

		_, err1 := c.Run("list-panes", "-a")
		if err1 == nil {
			t.Fatalf("first list-panes -a: expected intercepted error, got nil")
		}

		out2, err2 := c.Run("list-panes", "-a")
		if err2 != nil {
			t.Fatalf("second list-panes -a: expected pass-through, got error %v", err2)
		}
		if out2 == "" {
			t.Fatalf("second list-panes -a: expected non-empty real-tmux output")
		}
	})
}

type panicCommander struct{}

func (panicCommander) Run(args ...string) (string, error) {
	panic(fmt.Sprintf("panicCommander.Run reached with args=%v — interception logic regression", args))
}

func (panicCommander) RunRaw(args ...string) (string, error) {
	panic(fmt.Sprintf("panicCommander.RunRaw reached with args=%v — interception logic regression", args))
}
