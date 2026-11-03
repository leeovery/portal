package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/spf13/cobra"
)

var listDeps *ListDeps

type ListDeps struct {
	IsTTY func() bool
}

func isTTY() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

const listColumnGap = "    "

// formatSessionLong is name, label, attached state and dir. A session that
// did not answer is known only by its name, so that is all it shows.
func formatSessionLong(s backend.Session, idx *sessionindex.Index) string {
	if !s.Answered {
		return s.Name
	}
	status := "detached"
	if s.Attached() {
		status = "attached"
	}
	fields := []string{s.Name, idx.Label(s.Name), status}
	if s.Dir != "" {
		fields = append(fields, s.Dir)
	}
	return strings.Join(fields, listColumnGap)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		shortFlag, _ := cmd.Flags().GetBool("short")
		longFlag, _ := cmd.Flags().GetBool("long")

		if shortFlag && longFlag {
			return fmt.Errorf("--short and --long are mutually exclusive")
		}

		b, err := sessionBackend()
		if err != nil {
			return err
		}
		sessions, err := userSessions(cmd.Context(), b)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			return nil
		}

		useLong := (listToTTY() || longFlag) && !shortFlag
		format := func(s backend.Session) string { return s.Name }
		if useLong {
			idx, err := readSessionIndex()
			if err != nil {
				return err
			}
			format = func(s backend.Session) string { return formatSessionLong(s, &idx) }
		}

		w := cmd.OutOrStdout()
		for _, s := range sessions {
			if _, err := fmt.Fprintln(w, format(s)); err != nil {
				return err
			}
		}
		return nil
	},
}

func listToTTY() bool {
	if listDeps != nil {
		return listDeps.IsTTY()
	}
	return isTTY()
}

func init() {
	listCmd.Flags().Bool("short", false, "Output session names only")
	listCmd.Flags().Bool("long", false, "Output label, attached state and directory too")
	rootCmd.AddCommand(listCmd)
}
