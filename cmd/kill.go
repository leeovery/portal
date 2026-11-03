package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill [name]",
	Short: "Kill a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := sessionBackend()
		if err != nil {
			return err
		}
		return killSession(cmd.Context(), b, args[0], "cli")
	},
}

// killSession kills the session and removes its index entry. A session already
// gone keeps no entry either; one that may still be running keeps its own. via
// is the surface asking, as updateSessionIndex records it.
func killSession(ctx context.Context, b backend.Backend, name, via string) error {
	killed := b.Kill(ctx, name)
	if killed != nil && !errors.Is(killed, backend.ErrNoSuchSession) {
		return killError(name, killed)
	}
	removed := updateSessionIndex(via, func(idx *sessionindex.Index) error {
		idx.Remove(name)
		return nil
	})
	switch {
	case killed != nil && removed != nil:
		return fmt.Errorf("%w, and its session index entry could not be removed: %w", killError(name, killed), removed)
	case killed != nil:
		return killError(name, killed)
	case removed != nil:
		return fmt.Errorf("killed %s, but could not remove it from the session index: %w", name, removed)
	}
	return nil
}

// killError words the backend's refusals for the user; any other failure is
// returned as the backend gave it.
func killError(name string, err error) error {
	if fault, ok := sessionFault(name, err, "the kill may still land"); ok {
		return fault
	}
	return err
}

// sessionFault words err as a failure of the session name itself, for the
// user: gone, held under another spelling, refusing connections, or not
// answering, where unanswered says what that leaves. It reports whether err
// was one; any other failure is not the session's.
func sessionFault(name string, err error, unanswered string) (error, bool) {
	if gone, ok := sessionGoneError(name, err); ok {
		return gone, true
	}
	switch {
	// Before the timeout it wraps: a crashed daemon never answers.
	case errors.Is(err, zmx.ErrRefusing):
		return fmt.Errorf("session %s refuses connections: %s", name, crashedDaemon), true
	case errors.Is(err, backend.ErrTimeout):
		return fmt.Errorf("session %s is not answering; %s", name, unanswered), true
	}
	return nil, false
}

// crashedDaemon words a session whose daemon refuses connections, for kill
// and doctor alike.
const crashedDaemon = "its daemon has crashed; `zmx list` clears its socket"

// sessionGoneError words a refusal of name as one the backend does not hold,
// naming the spelling it does hold where there is one, and reports whether
// err was such a refusal.
func sessionGoneError(name string, err error) (error, bool) {
	if held, ok := errors.AsType[*backend.HeldAsError](err); ok {
		return fmt.Errorf("No session found: %s; did you mean %s?", name, held.Held), true //nolint:staticcheck // user-facing message per spec
	}
	if errors.Is(err, backend.ErrNoSuchSession) {
		return fmt.Errorf("No session found: %s", name), true //nolint:staticcheck // user-facing message per spec
	}
	return nil, false
}

// pickerKiller kills from the picker as portal kill does, but for the
// session the kill key asked to kill, which counts as killed once gone (see
// goneCountsKilled).
type pickerKiller struct {
	ctx context.Context
	b   backend.Backend
	// asked is the session the kill key asked to kill, empty for none.
	asked string
}

func (k pickerKiller) KillSession(name string) error {
	b := k.b
	if name == k.asked {
		b = goneCountsKilled{b}
	}
	return killSession(k.ctx, b, name, "tui")
}

// goneCountsKilled is a backend whose Kill counts a session already gone as
// killed, for the kill key's prompt: the session can end, or be killed
// elsewhere, between the key and the kill, and the key's intent is met. A
// name held under another spelling is still refused: that session lives.
type goneCountsKilled struct {
	backend.Backend
}

func (b goneCountsKilled) Kill(ctx context.Context, name string) error {
	err := b.Backend.Kill(ctx, name)
	if _, held := errors.AsType[*backend.HeldAsError](err); !held && errors.Is(err, backend.ErrNoSuchSession) {
		return nil
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
