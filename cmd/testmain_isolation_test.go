package cmd

// TestMain poisons every PORTAL_*_FILE / PORTAL_STATE_DIR env var to a
// deliberately-invalid path before any test in the cmd package binary runs.
// Tests that correctly isolate (via t.Setenv("PORTAL_STATE_DIR", t.TempDir())
// and siblings) override the poison normally — no behaviour change for
// correctly-written tests. Tests that forget to isolate hit the poisoned
// paths and fail loudly trying to read or write, rather than silently
// mutating the developer's real ~/.config/portal/ configuration.
//
// Subprocess inheritance: when a test spawns exec.Command(...) without
// explicitly setting cmd.Env, the subprocess inherits os.Environ() — which
// includes the poisoned values from this TestMain. So the symptom-fixture
// class of bug (subprocess inherits developer's real env because PORTAL_*_FILE
// wasn't t.Setenv'd) becomes structurally impossible to ship: the subprocess
// either uses a poisoned (failing) path or a test-supplied (isolated) one.
//
// The poison paths use /nonexistent/portal-test-must-isolate-* so a writer
// fails at the parent-dir-missing stage (AtomicWrite temp-create) rather
// than silently succeeding. A reader against an absent file gets ENOENT,
// which some production code paths tolerate (e.g. hooks.Store.Load returns
// empty map on ENOENT) — those silently-tolerant paths simply behave as if
// the config is empty, which is the correct test-isolation semantic.
//
// This file is the structural counterpart to portaltest.IsolateStateForTest:
// IsolateStateForTest is opt-in per-test; this TestMain makes opt-out the
// failing default. Together they close the test-fixture-env-isolation class
// of bug (cleanup-purge-test-no-state-isolation, symptom-fixture wipes hooks)
// at the package level rather than relying on per-test contributor discipline.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("PORTAL_STATE_DIR", "/nonexistent/portal-test-must-isolate-state")
	os.Setenv("PORTAL_HOOKS_FILE", "/nonexistent/portal-test-must-isolate-hooks.json")
	os.Setenv("PORTAL_PROJECTS_FILE", "/nonexistent/portal-test-must-isolate-projects.json")
	os.Setenv("PORTAL_ALIASES_FILE", "/nonexistent/portal-test-must-isolate-aliases")
	// TMUX poison — the tmux-boundary counterpart of the path poisons above.
	// Tests usually run inside the developer's real tmux, so any test that
	// Executes a real command body whose production wiring builds
	// tmux.DefaultClient() (state cleanup, clean, hooks, commit-now, daemon,
	// signal-hydrate, hydrate, the production orchestrator) would otherwise
	// inherit the ambient TMUX and operate on the REAL server. Incident of
	// record: two tests ran the real `portal state cleanup` body uninjected
	// and kill-sessioned the developer's live _portal-saver on every
	// `go test ./cmd`. With the poison, a missed injection dials a dead
	// socket and fails loudly. Tests that need a real server use tmuxtest's
	// explicit per-test -S sockets (which override TMUX) and set their own
	// TMUX for subprocesses; tests asserting inside/outside-tmux BEHAVIOUR
	// must t.Setenv TMUX themselves (they already had to, to be runnable
	// both inside and outside a tmux window).
	os.Setenv("TMUX", "/nonexistent/portal-test-must-set-tmux-socket,0,0")
	os.Exit(m.Run())
}
