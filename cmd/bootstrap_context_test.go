package cmd

import (
	"context"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

func TestTmuxClient(t *testing.T) {
	t.Run("it falls back to the default client with none staged", func(t *testing.T) {
		fallback := tmux.NewClient(commandertest.Quiet())
		withFuncSeam(t, &defaultTmuxClient, func() *tmux.Client { return fallback })
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())

		if got := tmuxClient(cmd); got != fallback {
			t.Errorf("tmuxClient = %p, want the default %p", got, fallback)
		}
	})

	t.Run("it fails loudly under the test poison rather than reach the real server", func(t *testing.T) {
		t.Setenv("TMUX", "")
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())

		defer func() {
			if r := recover(); r != refusedDefaultTmuxClient {
				t.Errorf("recovered %v, want the poison's panic %q", r, refusedDefaultTmuxClient)
			}
		}()
		tmuxClient(cmd)
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
