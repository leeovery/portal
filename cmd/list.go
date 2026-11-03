package cmd

import (
	"cmp"
	"fmt"
	"os"
	"strings"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/tui"
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

const listNotAnswering = "not answering"

// formatSessionLong is name, label, attached state and dir: the recorded one,
// as search and grouping read it, else the session's cwd. A session that did
// not answer is known only by its name, so it says so rather than leave its
// other fields to read as missing.
func formatSessionLong(s tui.Session) string {
	if s.NotAnswering {
		return s.Name + listColumnGap + listNotAnswering
	}
	status := "detached"
	if s.Attached {
		status = "attached"
	}
	fields := []string{s.Name, s.DisplayName(), status}
	if dir := cmp.Or(s.Dir, s.Cwd); dir != "" {
		fields = append(fields, dir)
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
			idx, err := listLabels(longFlag)
			if err != nil {
				return err
			}
			format = func(s backend.Session) string { return formatSessionLong(shownSession(s, &idx)) }
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

// listLabels is the index the long form takes its labels from. Asked for
// with --long, an index that cannot be read fails the listing. A terminal's
// default long form loses only its labels to one, so it reads as empty.
func listLabels(asked bool) (sessionindex.Index, error) {
	idx, err := readSessionIndex()
	if err != nil && !asked {
		return sessionindex.Index{}, nil
	}
	return idx, err
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
