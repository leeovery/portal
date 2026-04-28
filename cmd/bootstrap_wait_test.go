package cmd

// Tests in this file mutate package-level state (waiterFunc, bootstrapDeps) and MUST NOT use t.Parallel.

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
)

// installStubWaiter replaces the package-level waiterFunc with stub for the
// duration of the test, restoring the original on cleanup.
func installStubWaiter(t *testing.T, stub func(*cobra.Command)) {
	t.Helper()
	prev := waiterFunc
	waiterFunc = stub
	t.Cleanup(func() { waiterFunc = prev })
}

func TestBootstrapWait(t *testing.T) {
	t.Run("prints starting message to stderr when server was started", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, true)
		cmd.SetContext(ctx)
		stderr := new(bytes.Buffer)
		cmd.SetErr(stderr)

		installStubWaiter(t, func(*cobra.Command) {})

		bootstrapWait(cmd)

		want := "Starting tmux server...\n"
		if stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	})

	t.Run("calls waiter when server was started", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, true)
		cmd.SetContext(ctx)
		cmd.SetErr(new(bytes.Buffer))

		waiterCalled := false
		installStubWaiter(t, func(*cobra.Command) { waiterCalled = true })

		bootstrapWait(cmd)

		if !waiterCalled {
			t.Error("expected waiter to be called")
		}
	})

	t.Run("does not print message when server was not started", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, false)
		cmd.SetContext(ctx)
		stderr := new(bytes.Buffer)
		cmd.SetErr(stderr)

		installStubWaiter(t, func(*cobra.Command) {})

		bootstrapWait(cmd)

		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
	})

	t.Run("does not call waiter when server was not started", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, false)
		cmd.SetContext(ctx)
		cmd.SetErr(new(bytes.Buffer))

		waiterCalled := false
		installStubWaiter(t, func(*cobra.Command) { waiterCalled = true })

		bootstrapWait(cmd)

		if waiterCalled {
			t.Error("expected waiter not to be called")
		}
	})

	t.Run("does not print message when context has no serverStarted", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		stderr := new(bytes.Buffer)
		cmd.SetErr(stderr)

		waiterCalled := false
		installStubWaiter(t, func(*cobra.Command) { waiterCalled = true })

		bootstrapWait(cmd)

		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		if waiterCalled {
			t.Error("expected waiter not to be called")
		}
	})
}
