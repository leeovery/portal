package cmd

import (
	"context"
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
	live, err := liveSession(ctx, b, name, "its label is unchanged")
	if err != nil {
		return err
	}
	// The claim ties the label to this creation of the session, so a later
	// session under the same name starts without it. It records no dir: a
	// recorded dir is only ever where Portal minted the session, and the live
	// cwd frozen in its place would regroup the session by an exact key it
	// does not match.
	return claimSession(via, live, func(idx *sessionindex.Index) error {
		return idx.SetLabel(name, label)
	})
}

// liveSession reads the session a claim is for, wording one that fails as kill
// does, unchanged saying what the refusal leaves. One that does not answer is
// refused: what a claim ties to a creation it can't read would pass to
// whatever session next takes the name.
func liveSession(ctx context.Context, b backend.Backend, name, unchanged string) (claimed, error) {
	s, err := b.Session(ctx, name)
	if fault, ok := sessionFault(name, err, unchanged); ok {
		return claimed{}, fault
	}
	if err != nil {
		return claimed{}, err
	}
	return claimed{Name: name, Created: s.Created, PID: s.PID}, nil
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
