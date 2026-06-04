package tmux_test

// TestMain poisons every PORTAL_*_FILE / PORTAL_STATE_DIR env var to a
// deliberately-invalid path before any test in this package binary runs. See
// cmd/testmain_isolation_test.go for the full rationale. Tests that correctly
// isolate via t.Setenv (or portaltest.IsolateStateForTest) override the
// poison normally; tests that forget to isolate fail loudly against the
// /nonexistent paths instead of silently mutating the developer's real
// ~/.config/portal/ configuration.

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
