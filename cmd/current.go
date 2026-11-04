package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/backend"
	"github.com/spf13/cobra"
)

// errNotInSession refuses a command that acts on the session it runs in, run
// outside one.
var errNotInSession = errors.New("not inside a Portal session")

// errNoCurrentSession is current's answer outside a session: nothing printed
// and exit 1, so a footer reading it shows nothing.
var errNoCurrentSession = errors.New("no current session")

// currentSession is the name of the session this process runs in, the key of
// everything Portal stores about it.
func currentSession() (string, error) {
	b, err := sessionBackend()
	if err != nil {
		return "", err
	}
	return currentSessionIn(b)
}

// currentSessionIn is currentSession on a backend the caller already holds.
func currentSessionIn(b backend.Backend) (string, error) {
	name := b.Current()
	if name == "" {
		return "", errNotInSession
	}
	return name, nil
}

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Print the current session's label",
	Args:  cobra.NoArgs,
	// Outside a session it decides before binding anything, so a footer
	// refreshing outside one never prints a binding's failure.
	RunE: func(cmd *cobra.Command, args []string) error {
		name := currentName()
		if name == "" {
			return errNoCurrentSession
		}
		b, err := sessionBackend()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), currentShown(cmd.Context(), b, name))
		return err
	},
}

// currentShown is what the session this process runs in shows. Footers and
// Claude's --name read it, so a session that cannot be read, or an index that
// cannot, costs only the label: the name still names the session.
func currentShown(ctx context.Context, b backend.Backend, name string) string {
	live, err := b.Session(ctx, name)
	if err != nil {
		return name
	}
	shown, _ := readShownIndex()
	return shown.show(live).DisplayName()
}

func init() {
	rootCmd.AddCommand(currentCmd)
}
