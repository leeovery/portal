package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// errNotInSession refuses a command that acts on the session it runs in, run
// outside one.
var errNotInSession = errors.New("not inside a Portal session")

// currentSession is the name of the session this process runs in, the key of
// everything Portal stores about it.
func currentSession() (string, error) {
	b, err := sessionBackend()
	if err != nil {
		return "", err
	}
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
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := currentSession()
		if err != nil {
			return err
		}
		idx, err := readSessionIndex()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), idx.Label(name))
		return err
	},
}

func init() {
	rootCmd.AddCommand(currentCmd)
}
