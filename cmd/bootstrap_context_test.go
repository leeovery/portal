package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestTmuxClient(t *testing.T) {
	t.Run("panics when no client in context", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic, got none")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("expected string panic, got %T: %v", r, r)
			}
			want := "tmuxClient: no client in context — PersistentPreRunE must run before any command that uses tmux"
			if msg != want {
				t.Errorf("panic message = %q, want %q", msg, want)
			}
		}()

		tmuxClient(cmd)
	})
}

func TestServerWasStarted(t *testing.T) {
	t.Run("returns true when context has serverStarted=true", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, true)
		cmd.SetContext(ctx)

		if !serverWasStarted(cmd) {
			t.Error("expected true, got false")
		}
	})

	t.Run("returns false when context has serverStarted=false", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, false)
		cmd.SetContext(ctx)

		if serverWasStarted(cmd) {
			t.Error("expected false, got true")
		}
	})

	t.Run("returns false when context has no serverStarted value", func(t *testing.T) {
		cmd := &cobra.Command{}

		if serverWasStarted(cmd) {
			t.Error("expected false, got true")
		}
	})

	t.Run("returns false for nil context value of wrong type", func(t *testing.T) {
		cmd := &cobra.Command{}
		ctx := context.WithValue(context.Background(), serverStartedKey, "not-a-bool")
		cmd.SetContext(ctx)

		if serverWasStarted(cmd) {
			t.Error("expected false, got true")
		}
	})
}
