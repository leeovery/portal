package cmd

// Tests in this file mutate package-level state (bootstrapDeps, attachDeps) and MUST NOT use t.Parallel.

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
	bootstrapDeps = &BootstrapDeps{Bootstrapper: &mockServerBootstrapper{}}
	t.Cleanup(func() { bootstrapDeps = nil })

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

	t.Run("exact name match only rejects partial name", func(t *testing.T) {
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{"my-session-abc123": true}}
		attachDeps = &AttachDeps{
			Connector: connector,
			Validator: validator,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "my-session"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error for partial name, got nil")
		}

		want := "No session found: my-session"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		// Verify Connect was NOT called for partial match
		if connector.connectedTo != "" {
			t.Errorf("Connect should not be called for partial name match, but was called with %q", connector.connectedTo)
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

	t.Run("hook execution runs before connect", func(t *testing.T) {
		var callOrder []string
		connector := &mockSessionConnector{}
		// Wrap Connect to record ordering
		originalConnect := connector
		validator := &mockSessionValidator{sessions: map[string]bool{"my-session": true}}

		hookExecutor := HookExecutorFunc(func(sessionName string) {
			callOrder = append(callOrder, "hooks")
		})

		attachDeps = &AttachDeps{
			Connector: &orderTrackingConnector{
				inner:     originalConnect,
				callOrder: &callOrder,
			},
			Validator:    validator,
			HookExecutor: hookExecutor,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "my-session"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(callOrder) != 2 {
			t.Fatalf("expected 2 calls, got %d: %v", len(callOrder), callOrder)
		}
		if callOrder[0] != "hooks" {
			t.Errorf("expected hooks to run first, got %q", callOrder[0])
		}
		if callOrder[1] != "connect" {
			t.Errorf("expected connect to run second, got %q", callOrder[1])
		}
	})

	t.Run("hook execution receives correct session name", func(t *testing.T) {
		var receivedName string
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{"dev-project": true}}

		hookExecutor := HookExecutorFunc(func(sessionName string) {
			receivedName = sessionName
		})

		attachDeps = &AttachDeps{
			Connector:    connector,
			Validator:    validator,
			HookExecutor: hookExecutor,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "dev-project"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedName != "dev-project" {
			t.Errorf("hook executor received %q, want %q", receivedName, "dev-project")
		}
	})

	t.Run("non-existent session skips hook execution", func(t *testing.T) {
		hookCalled := false
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{}}

		hookExecutor := HookExecutorFunc(func(sessionName string) {
			hookCalled = true
		})

		attachDeps = &AttachDeps{
			Connector:    connector,
			Validator:    validator,
			HookExecutor: hookExecutor,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "nonexistent"})

		_ = rootCmd.Execute()

		if hookCalled {
			t.Error("hook executor should not be called for non-existent session")
		}
	})

	t.Run("hook executor is called when session exists", func(t *testing.T) {
		hookCalled := false
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{"empty-session": true}}

		hookExecutor := HookExecutorFunc(func(sessionName string) {
			hookCalled = true
		})

		attachDeps = &AttachDeps{
			Connector:    connector,
			Validator:    validator,
			HookExecutor: hookExecutor,
		}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "empty-session"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !hookCalled {
			t.Error("hook executor should be called when session exists")
		}
	})
}

// orderTrackingConnector wraps a SessionConnector and records call ordering.
type orderTrackingConnector struct {
	inner     SessionConnector
	callOrder *[]string
}

func (o *orderTrackingConnector) Connect(name string) error {
	*o.callOrder = append(*o.callOrder, "connect")
	return o.inner.Connect(name)
}
