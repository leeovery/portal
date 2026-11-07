package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestDeferredBootstrapFromContext(t *testing.T) {
	t.Run("it returns the bootstrap PersistentPreRunE deferred", func(t *testing.T) {
		cmd := &cobra.Command{}
		deferred := &deferredBootstrap{runner: &recordingRunner{}}
		cmd.SetContext(context.WithValue(context.Background(), deferredBootstrapKey, deferred))

		if got := deferredBootstrapFromContext(cmd); got != deferred {
			t.Errorf("deferredBootstrapFromContext = %v, want %v", got, deferred)
		}
	})

	t.Run("it returns none from a context that carries none, or none at all", func(t *testing.T) {
		withValue := &cobra.Command{}
		withValue.SetContext(context.WithValue(context.Background(), deferredBootstrapKey, "not a bootstrap"))

		for name, cmd := range map[string]*cobra.Command{"a context with none": withValue, "no context": {}} {
			if got := deferredBootstrapFromContext(cmd); got != nil {
				t.Errorf("%s: deferredBootstrapFromContext = %v, want nil", name, got)
			}
		}
	})
}
