package cmd

// Tests in this file mutate package-level state (bootstrapDeps, attachDeps) and MUST NOT use t.Parallel.

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

// mockSessionConnector records Connect calls for testing. When order is
// non-nil it also appends "connect" to the shared call-order recorder so a
// test can assert the write-strictly-before-connect ordering.
type mockSessionConnector struct {
	connectedTo string
	err         error
	order       *[]string
}

func (m *mockSessionConnector) Connect(name string) error {
	m.connectedTo = name
	if m.order != nil {
		*m.order = append(*m.order, "connect")
	}
	return m.err
}

// mockSessionValidator checks whether a session exists.
type mockSessionValidator struct {
	sessions map[string]bool
}

func (m *mockSessionValidator) HasSession(name string) bool {
	return m.sessions[name]
}

// ackWrite records a single AckWriter.Write(batch, token) call.
type ackWrite struct {
	batch string
	token string
}

// mockAckWriter records Write calls (satisfying spawn.AckWriter) and, when
// order is non-nil, appends "write" to the shared call-order recorder so a
// test can assert the write happens strictly before the connect.
type mockAckWriter struct {
	calls []ackWrite
	err   error
	order *[]string
}

func (m *mockAckWriter) Write(batch, token string) error {
	m.calls = append(m.calls, ackWrite{batch: batch, token: token})
	if m.order != nil {
		*m.order = append(*m.order, "write")
	}
	return m.err
}

func TestAttachCommand(t *testing.T) {
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
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
}

func TestAttachSpawnAck(t *testing.T) {
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("it writes the ack marker after the session-exists check and before connect", func(t *testing.T) {
		var order []string
		connector := &mockSessionConnector{order: &order}
		validator := &mockSessionValidator{sessions: map[string]bool{"s1": true}}
		ackWriter := &mockAckWriter{order: &order}
		attachDeps = &AttachDeps{Connector: connector, Validator: validator, AckWriter: ackWriter}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "s1", "--spawn-ack", "b1:t1"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(ackWriter.calls) != 1 {
			t.Fatalf("Write call count = %d, want 1", len(ackWriter.calls))
		}
		if got := ackWriter.calls[0]; got.batch != "b1" || got.token != "t1" {
			t.Errorf("Write(%q, %q), want (%q, %q)", got.batch, got.token, "b1", "t1")
		}
		if connector.connectedTo != "s1" {
			t.Errorf("Connect called with %q, want %q", connector.connectedTo, "s1")
		}
		want := []string{"write", "connect"}
		if !slices.Equal(order, want) {
			t.Errorf("call order = %v, want %v (write strictly before connect)", order, want)
		}
	})

	t.Run("it still execs the attach when the marker write fails (best-effort)", func(t *testing.T) {
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{"s1": true}}
		ackWriter := &mockAckWriter{err: fmt.Errorf("set-option failed")}
		attachDeps = &AttachDeps{Connector: connector, Validator: validator, AckWriter: ackWriter}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "s1", "--spawn-ack", "b1:t1"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("expected no error on best-effort write failure, got %v", err)
		}

		if len(ackWriter.calls) != 1 {
			t.Errorf("Write call count = %d, want 1", len(ackWriter.calls))
		}
		if connector.connectedTo != "s1" {
			t.Errorf("Connect called with %q, want %q (best-effort must still exec)", connector.connectedTo, "s1")
		}
	})

	t.Run("it returns a usage error (exit 2) for a malformed --spawn-ack value", func(t *testing.T) {
		for _, val := range []string{"bogus", "b1:", ":t1"} {
			t.Run(val, func(t *testing.T) {
				connector := &mockSessionConnector{}
				validator := &mockSessionValidator{sessions: map[string]bool{"s1": true}}
				ackWriter := &mockAckWriter{}
				attachDeps = &AttachDeps{Connector: connector, Validator: validator, AckWriter: ackWriter}
				t.Cleanup(func() { attachDeps = nil })

				resetRootCmd()
				rootCmd.SetArgs([]string{"attach", "s1", "--spawn-ack", val})

				err := rootCmd.Execute()
				if err == nil {
					t.Fatalf("expected a UsageError for %q, got nil", val)
				}
				var usageErr *UsageError
				if !errors.As(err, &usageErr) {
					t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
				}
				if len(ackWriter.calls) != 0 {
					t.Errorf("Write called %d times for malformed flag, want 0", len(ackWriter.calls))
				}
				if connector.connectedTo != "" {
					t.Errorf("Connect called with %q for malformed flag, want no call", connector.connectedTo)
				}
			})
		}
	})

	t.Run("it writes no marker and takes the no-session path when the session is gone", func(t *testing.T) {
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{}}
		ackWriter := &mockAckWriter{}
		attachDeps = &AttachDeps{Connector: connector, Validator: validator, AckWriter: ackWriter}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "ghost", "--spawn-ack", "b1:t1"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		want := "No session found: ghost"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		if len(ackWriter.calls) != 0 {
			t.Errorf("Write called %d times for a gone session, want 0", len(ackWriter.calls))
		}
		if connector.connectedTo != "" {
			t.Errorf("Connect called with %q for a gone session, want no call", connector.connectedTo)
		}
	})

	t.Run("it leaves plain attach unchanged when --spawn-ack is absent", func(t *testing.T) {
		connector := &mockSessionConnector{}
		validator := &mockSessionValidator{sessions: map[string]bool{"s1": true}}
		ackWriter := &mockAckWriter{}
		attachDeps = &AttachDeps{Connector: connector, Validator: validator, AckWriter: ackWriter}
		t.Cleanup(func() { attachDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"attach", "s1"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "s1" {
			t.Errorf("Connect called with %q, want %q", connector.connectedTo, "s1")
		}
		if len(ackWriter.calls) != 0 {
			t.Errorf("ack writer touched on plain attach: %d Write calls, want 0", len(ackWriter.calls))
		}
	})
}
