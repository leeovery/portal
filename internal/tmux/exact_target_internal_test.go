package tmux

import "testing"

// TestExactTarget pins the session-level exact-match target builder. The helper
// is unexported, and tmux_test.go is the external package tmux_test (it calls
// tmux.NewClient), so it cannot reach exactTarget directly — this same-package
// internal test file is the only place the focused unit assertion can live.
func TestExactTarget(t *testing.T) {
	if got := exactTarget("foo"); got != "=foo" {
		t.Errorf("exactTarget(\"foo\") = %q, want \"=foo\"", got)
	}
}
