package cmd

import (
	"context"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

func TestTmuxClient(t *testing.T) {
	t.Run("it falls back to the default client, starting no server", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())

		if tmuxClient(cmd) == nil {
			t.Fatal("tmuxClient returned nil with no client in the context")
		}
	})

	t.Run("it returns the client staged in the context", func(t *testing.T) {
		rec := commandertest.Quiet()
		staged := tmux.NewClient(rec)

		if got := tmuxClient(cmdWithClient(staged)); got != staged {
			t.Errorf("tmuxClient = %p, want the staged %p", got, staged)
		}
		if len(rec.Calls()) != 0 {
			t.Errorf("tmuxClient ran %v, want nothing", rec.Calls())
		}
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
