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
		return setSessionLabel(cmd.Context(), b, name, strings.Join(args, " "), "cli")
	},
}

// setSessionLabel labels the live session name; empty text, or the name
// itself, resets the label to the name. via is the surface asking, as
// updateSessionIndex records it.
func setSessionLabel(ctx context.Context, b backend.Backend, name, label, via string) error {
	live, err := liveSession(ctx, b, name)
	if err != nil {
		return err
	}
	// The claim ties the label to this creation of the session, so a later
	// session under the same name starts without it.
	return updateSessionIndex(via, func(idx *sessionindex.Index) error {
		idx.Claim(name, live.Dir, live.Created)
		return idx.SetLabel(name, label)
	})
}

// liveSession reads the session rename labels, wording a missing one as kill
// does. One that does not answer is refused: a label written without its
// creation time would pass to whatever session next takes the name.
func liveSession(ctx context.Context, b backend.Backend, name string) (backend.Session, error) {
	s, err := b.Session(ctx, name)
	if gone, ok := sessionGoneError(name, err); ok {
		return backend.Session{}, gone
	}
	if errors.Is(err, backend.ErrTimeout) {
		return backend.Session{}, fmt.Errorf("session %s is not answering; its label is unchanged", name)
	}
	return s, err
}

// pickerLabeller labels from the picker as portal rename does.
type pickerLabeller struct {
	ctx context.Context
	b   backend.Backend
}

func (l pickerLabeller) SetLabel(name, label string) error {
	return setSessionLabel(l.ctx, l.b, name, label, "tui")
}

func init() {
	rootCmd.AddCommand(renameCmd)
}
