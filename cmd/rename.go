package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/spf13/cobra"
)

var errLabelRefused = errors.New("label refused: it holds a control or formatting character, such as a newline, tab or escape")

var renameCmd = &cobra.Command{
	Use:   "rename [text...]",
	Short: "Set the current session's label; no text resets it to the session's name",
	Example: `  portal rename api work
  portal rename -- -wip    a label starting with - goes after --`,
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := sessionBackend()
		if err != nil {
			return err
		}
		name, err := currentSessionIn(b)
		if err != nil {
			return err
		}
		live, err := liveSession(cmd.Context(), b, name)
		if err != nil {
			return err
		}
		label := strings.Join(args, " ")
		// The claim ties the label to this creation of the session, so a later
		// session under the same name starts without it.
		err = updateSessionIndex("cli", func(idx *sessionindex.Index) error {
			idx.Claim(name, live.Dir, live.Created)
			return idx.SetLabel(name, label)
		})
		if errors.Is(err, sessionindex.ErrLabelUnprintable) {
			return errLabelRefused
		}
		return err
	},
}

// liveSession reads the session rename labels. One that does not answer is
// refused: a label written without its creation time would pass to whatever
// session next takes the name.
func liveSession(ctx context.Context, b backend.Backend, name string) (backend.Session, error) {
	s, err := b.Session(ctx, name)
	if errors.Is(err, backend.ErrTimeout) {
		return backend.Session{}, fmt.Errorf("session %s is not answering; its label is unchanged", name)
	}
	return s, err
}

func init() {
	rootCmd.AddCommand(renameCmd)
}
