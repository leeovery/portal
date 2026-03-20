package cmd

import (
	"fmt"
	"testing"
)

// mockSessionKiller records KillSession calls for testing.
type mockSessionKiller struct {
	killedName string
	err        error
}

func (m *mockSessionKiller) KillSession(name string) error {
	m.killedName = name
	return m.err
}

func TestKillCommand(t *testing.T) {
	bootstrapDeps = &BootstrapDeps{Bootstrapper: &mockServerBootstrapper{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("existing session calls kill-session and exits 0", func(t *testing.T) {
		killer := &mockSessionKiller{}
		validator := &mockSessionValidator{sessions: map[string]bool{"my-session": true}}
		killDeps = &KillDeps{
			Killer:    killer,
			Validator: validator,
		}
		t.Cleanup(func() { killDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"kill", "my-session"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if killer.killedName != "my-session" {
			t.Errorf("KillSession called with %q, want %q", killer.killedName, "my-session")
		}
	})

	t.Run("non-existent session prints error and exits 1", func(t *testing.T) {
		killer := &mockSessionKiller{}
		validator := &mockSessionValidator{sessions: map[string]bool{}}
		killDeps = &KillDeps{
			Killer:    killer,
			Validator: validator,
		}
		t.Cleanup(func() { killDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"kill", "nonexistent"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		want := "No session found: nonexistent"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		// Verify KillSession was NOT called
		if killer.killedName != "" {
			t.Errorf("KillSession should not be called for non-existent session, but was called with %q", killer.killedName)
		}
	})

	t.Run("uses exact name match", func(t *testing.T) {
		killer := &mockSessionKiller{}
		validator := &mockSessionValidator{sessions: map[string]bool{"my-session-abc123": true}}
		killDeps = &KillDeps{
			Killer:    killer,
			Validator: validator,
		}
		t.Cleanup(func() { killDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"kill", "my-session"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error for partial name, got nil")
		}

		want := "No session found: my-session"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		// Verify KillSession was NOT called for partial match
		if killer.killedName != "" {
			t.Errorf("KillSession should not be called for partial name match, but was called with %q", killer.killedName)
		}
	})

	t.Run("no confirmation prompt in CLI mode", func(t *testing.T) {
		// The kill command should not prompt for confirmation.
		// It should immediately call KillSession when session exists.
		killer := &mockSessionKiller{}
		validator := &mockSessionValidator{sessions: map[string]bool{"target": true}}
		killDeps = &KillDeps{
			Killer:    killer,
			Validator: validator,
		}
		t.Cleanup(func() { killDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"kill", "target"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// If we got here without blocking on stdin, no confirmation was needed
		if killer.killedName != "target" {
			t.Errorf("KillSession called with %q, want %q", killer.killedName, "target")
		}
	})

	t.Run("killing session inside tmux works", func(t *testing.T) {
		// When inside tmux and killing a session (even the current one),
		// tmux handles it — the command just calls kill-session.
		t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")

		killer := &mockSessionKiller{}
		validator := &mockSessionValidator{sessions: map[string]bool{"current-session": true}}
		killDeps = &KillDeps{
			Killer:    killer,
			Validator: validator,
		}
		t.Cleanup(func() { killDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"kill", "current-session"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if killer.killedName != "current-session" {
			t.Errorf("KillSession called with %q, want %q", killer.killedName, "current-session")
		}
	})

	t.Run("kill-session failure returns error", func(t *testing.T) {
		killer := &mockSessionKiller{err: fmt.Errorf("failed to kill tmux session \"my-session\": exit status 1")}
		validator := &mockSessionValidator{sessions: map[string]bool{"my-session": true}}
		killDeps = &KillDeps{
			Killer:    killer,
			Validator: validator,
		}
		t.Cleanup(func() { killDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"kill", "my-session"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
