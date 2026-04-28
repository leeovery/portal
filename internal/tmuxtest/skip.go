package tmuxtest

import (
	"os/exec"
	"testing"
)

// SkipIfNoTmux skips the test when tmux is not on PATH. Integration tests
// across the portal codebase share this helper so CI environments without
// tmux installed skip cleanly rather than fail. The single canonical
// definition lives here so the check (and its message) cannot drift.
func SkipIfNoTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available; skipping integration test")
	}
}
