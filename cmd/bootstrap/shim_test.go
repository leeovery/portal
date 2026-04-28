package bootstrap

import (
	"context"
	"errors"
	"testing"
)

// fakeServerBootstrapper records EnsureServer invocations and returns a
// configurable (started, err) tuple. Used to verify NewShim's Runner
// delegates verbatim to EnsureServer without performing any other steps.
type fakeServerBootstrapper struct {
	calls   int
	started bool
	err     error
}

func (f *fakeServerBootstrapper) EnsureServer() (bool, error) {
	f.calls++
	return f.started, f.err
}

func TestNewShim_RunDelegatesToEnsureServer(t *testing.T) {
	t.Run("returns started flag and nil err verbatim", func(t *testing.T) {
		b := &fakeServerBootstrapper{started: true}
		runner := NewShim(b)

		started, _, err := runner.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !started {
			t.Errorf("started = false, want true")
		}
		if b.calls != 1 {
			t.Errorf("EnsureServer call count = %d, want 1", b.calls)
		}
	})

	t.Run("returns started=false verbatim", func(t *testing.T) {
		b := &fakeServerBootstrapper{started: false}
		runner := NewShim(b)

		started, _, err := runner.Run(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if started {
			t.Errorf("started = true, want false")
		}
	})

	t.Run("propagates EnsureServer error verbatim without wrapping", func(t *testing.T) {
		sentinel := errors.New("ensure server boom")
		b := &fakeServerBootstrapper{err: sentinel}
		runner := NewShim(b)

		started, _, err := runner.Run(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != sentinel { //nolint:errorlint // shim returns the error verbatim, not wrapped
			t.Errorf("error = %v, want sentinel %v (verbatim, not wrapped)", err, sentinel)
		}
		if started {
			t.Errorf("started = true on error path, want false")
		}
	})
}

// TestOrchestratorSatisfiesRunner pins the implicit interface relationship
// cmd/root.go relies on: an *Orchestrator must be assignable to Runner.
func TestOrchestratorSatisfiesRunner(t *testing.T) {
	var _ Runner = (*Orchestrator)(nil)
	_ = t
}

func TestNewShim_NilBootstrapperReturnsNoOpRunner(t *testing.T) {
	runner := NewShim(nil)
	if runner == nil {
		t.Fatal("NewShim(nil) returned a nil Runner")
	}

	started, _, err := runner.Run(context.Background())
	if err != nil {
		t.Errorf("expected nil error from no-op shim, got %v", err)
	}
	if started {
		t.Errorf("expected started=false from no-op shim, got true")
	}
}
