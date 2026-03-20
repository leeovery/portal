package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestBootstrapWait(t *testing.T) {
	t.Run("prints starting message to stderr when server was started", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, true)
		cmd.SetContext(ctx)
		stderr := new(bytes.Buffer)
		cmd.SetErr(stderr)

		waiterCalled := false
		waiter := func() { waiterCalled = true }

		bootstrapWait(cmd, waiter)

		want := "Starting tmux server...\n"
		if stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
		_ = waiterCalled // checked in separate test
	})

	t.Run("calls waiter when server was started", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, true)
		cmd.SetContext(ctx)
		cmd.SetErr(new(bytes.Buffer))

		waiterCalled := false
		waiter := func() { waiterCalled = true }

		bootstrapWait(cmd, waiter)

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

		waiterCalled := false
		waiter := func() { waiterCalled = true }

		bootstrapWait(cmd, waiter)

		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		_ = waiterCalled
	})

	t.Run("does not call waiter when server was not started", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, false)
		cmd.SetContext(ctx)
		cmd.SetErr(new(bytes.Buffer))

		waiterCalled := false
		waiter := func() { waiterCalled = true }

		bootstrapWait(cmd, waiter)

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
		waiter := func() { waiterCalled = true }

		bootstrapWait(cmd, waiter)

		if stderr.String() != "" {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		if waiterCalled {
			t.Error("expected waiter not to be called")
		}
	})
}
