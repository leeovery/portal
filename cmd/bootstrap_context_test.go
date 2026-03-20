package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

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
