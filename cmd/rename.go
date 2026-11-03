package cmd

import (
	"errors"
	"strings"

	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/spf13/cobra"
)

var errLabelRefused = errors.New("label refused: it holds a control or formatting character, such as a newline, tab or escape")

var renameCmd = &cobra.Command{
	Use:   "rename [text...]",
	Short: "Set the current session's label; no text resets it to the session's name",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := currentSession()
		if err != nil {
			return err
		}
		label := strings.Join(args, " ")
		err = updateSessionIndex("cli", func(idx *sessionindex.Index) error {
			return idx.SetLabel(name, label)
		})
		if errors.Is(err, sessionindex.ErrLabelUnprintable) {
			return errLabelRefused
		}
		return err
	},
}

func init() {
	rootCmd.AddCommand(renameCmd)
}
