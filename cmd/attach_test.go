package cmd

import (
	"fmt"
	"testing"
)

// mockSessionConnector records Connect calls for testing.
type mockSessionConnector struct {
	connectedTo string
	err         error
}

func (m *mockSessionConnector) Connect(name string) error {
	m.connectedTo = name
	return m.err
}

// mockSessionValidator checks whether a session exists.
type mockSessionValidator struct {
	sessions map[string]bool
}

func (m *mockSessionValidator) HasSession(name string) bool {
	return m.sessions[name]
}

func TestAttachCommand(t *testing.T) {
	t.Run("inside tmux uses switch-client", func(t *testing.T) {
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{"my-session": true}}
		attachDeps = &AttachDeps{
			Connector: connector,
			Validator: validator,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "my-session"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "my-session" {
			t.Errorf("Connect called with %q, want %q", connector.connectedTo, "my-session")
		}
	})

	t.Run("outside tmux uses connect (exec attach-session)", func(t *testing.T) {
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{"work-session": true}}
		attachDeps = &AttachDeps{
			Connector: connector,
			Validator: validator,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "work-session"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "work-session" {
			t.Errorf("Connect called with %q, want %q", connector.connectedTo, "work-session")
		}
	})

	t.Run("switch-client failure returns error", func(t *testing.T) {
		connector := &mockSessionConnector{err: fmt.Errorf("failed to switch to session \"dead-session\": session not found")}
		validator := &mockSessionValidator{sessions: map[string]bool{"dead-session": true}}
		attachDeps = &AttachDeps{
			Connector: connector,
			Validator: validator,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "dead-session"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("non-existent session returns not found error", func(t *testing.T) {
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{}}
		attachDeps = &AttachDeps{
			Connector: connector,
			Validator: validator,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "nonexistent"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		want := "No session found: nonexistent"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		// Verify Connect was NOT called
		if connector.connectedTo != "" {
			t.Errorf("Connect should not be called for non-existent session, but was called with %q", connector.connectedTo)
		}
	})
}
