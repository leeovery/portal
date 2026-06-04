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
	os.Exit(m.Run())
}
