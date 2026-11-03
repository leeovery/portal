package cmd

import (
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill [name]",
	Short: "Kill a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		b, err := sessionBackend()
		if err != nil {
			return err
		}
		if err := b.Kill(cmd.Context(), name); err != nil {
			return killError(name, err)
		}

		if err := updateSessionIndex("cli", func(idx *sessionindex.Index) error {
			idx.Remove(name)
			return nil
		}); err != nil {
			return fmt.Errorf("killed %s, but could not remove it from the session index: %w", name, err)
		}
		return nil
	},
}

// killError words the backend's refusals for the user; any other failure is
// returned as the backend gave it.
func killError(name string, err error) error {
	switch {
	case errors.Is(err, backend.ErrNoSuchSession):
		return fmt.Errorf("No session found: %s", name) //nolint:staticcheck // user-facing message per spec
	case errors.Is(err, backend.ErrTimeout):
		return fmt.Errorf("session %s is not answering; the kill may still land", name)
	}
	return err
}

func init() {
	// kill takes exactly one positional, so there is nothing left to complete
	// once it is present.
	killCmd.ValidArgsFunction = func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeSessionNames(toComplete)
	}

	rootCmd.AddCommand(killCmd)
}
