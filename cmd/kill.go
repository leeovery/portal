package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
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
		ctx := bootstrap.WithProgressEmitter(cmd.Context(), waitNotice(cmd.ErrOrStderr()))
		err = killSession(ctx, b, args[0], "cli")
		if _, ok := errors.AsType[discardedError](err); ok {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), discardedCopy+"\n", args[0])
			return nil
		}
		return err
	},
}

// discardedCopy is portal kill's one line for a saved session that wasn't
// running, which it discards as it would kill a live one: the session named.
const discardedCopy = "Discarded %s, which wasn't running: Portal won't bring it back."

// discardedError is killSession's answer for a session not running whose
// saved entry it removed. portal kill counts that a kill, a saved session
// discarded; the picker, which lists only live sessions, reports the session
// gone, as gone carries it.
type discardedError struct{ gone error }

func (e discardedError) Error() string { return e.gone.Error() }

func (e discardedError) Unwrap() error { return e.gone }

// killSession kills the session and forgets it: its index entry, and what
// Portal keeps beside it. A session not running whose entry it removes is
// discarded, a discardedError; with no entry either, the session is not
// found. One held under another spelling, or that may still be running, keeps
// everything. A session not running is judged again under bootstrap.lock,
// held through its discard, so the discard never races x <name> bringing it
// back: live by then, it is killed as a live session is. ctx's progress
// emitter hears of a long wait for the lock. via is the surface asking, as
// the index write records it.
func killSession(ctx context.Context, b backend.Backend, name, via string) error {
	killed := b.Kill(ctx, name)
	if notRunning(killed) {
		release, err := holdStateBootstrapLock(ctx)
		if err != nil {
			return err
		}
		defer release()
		killed = b.Kill(ctx, name)
	}
	if killed != nil && !notRunning(killed) {
		return killError(name, killed)
	}
	removed, err := forgetSession(name, via)
	switch {
	case killed == nil && err != nil:
		return fmt.Errorf("killed %s, but could not remove it from the session index: %w", name, err)
	case killed == nil:
		return nil
	case err != nil:
		return fmt.Errorf("%w, and its session index entry could not be removed: %w", killError(name, killed), err)
	case !removed:
		return killError(name, killed)
	}
	return discardedError{gone: killError(name, killed)}
}

// notRunning reports whether err, a kill's, is the backend's refusal of a
// name no session holds, under it or another spelling.
func notRunning(err error) bool {
	return errors.Is(err, backend.ErrNoSuchSession) && !heldUnderAnotherSpelling(err)
}

// holdStateBootstrapLock takes the state directory's bootstrap.lock as
// holdBootstrapLock does.
func holdStateBootstrapLock(ctx context.Context) (release func(), err error) {
	dir, err := state.Dir()
	if err != nil {
		return nil, err
	}
	return holdBootstrapLock(ctx, dir)
}

// heldUnderAnotherSpelling reports whether err is the backend's refusal of a
// name it holds only under another spelling: that session lives.
func heldUnderAnotherSpelling(err error) bool {
	_, held := errors.AsType[*backend.HeldAsError](err)
	return held
}

// forgetSession removes the session's index entry and forgets what Portal
// keeps beside it, under the same lock, whether or not the index held one: a
// hook or flag may be left with no entry to go with it. It reports whether
// the index held one.
func forgetSession(name, via string) (removed bool, err error) {
	dir, err := state.EnsureDir()
	if err != nil {
		return false, err
	}
	err = updateSessionIndexLanding(dir, via, func(idx *sessionindex.Index) error {
		idx.Remove(name)
		return nil
	}, forgetting(dir, via, func(outcome sessionindex.Outcome) []sessionindex.Entry {
		if len(outcome.Removed) == 0 {
			return []sessionindex.Entry{{Name: name}}
		}
		removed = true
		return outcome.Removed
	}))
	return removed, err
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
	// ErrRefusing wraps ErrTimeout, so it is asked first.
	case errors.Is(err, zmx.ErrRefusing):
		return fmt.Errorf("session %s refuses connections: %s", name, stillHeld), true
	case errors.Is(err, backend.ErrTimeout):
		return fmt.Errorf("session %s is not answering; %s", name, unanswered), true
	}
	return nil, false
}

// stillHeld words a session whose socket refuses connections while a zmx
// process may still hold it, for kill and doctor alike: a daemon with its
// listen backlog full refuses for a moment.
const stillHeld = "a zmx process may still hold it; try again in a moment"

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

// pickerKiller kills from the picker as portal kill does, but for a session
// gone by the kill, which it reports gone: the picker lists live sessions.
type pickerKiller struct {
	ctx context.Context
	b   backend.Backend
}

func (k pickerKiller) KillSession(name string) error {
	return killSession(k.ctx, k.b, name, "tui")
}

// askedKiller kills on y at the prompt the kill key asked for, counting the
// session already gone as killed.
func askedKiller(ctx context.Context, b backend.Backend) pickerKiller {
	return pickerKiller{ctx: ctx, b: goneCountsKilled{b}}
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
	if notRunning(err) {
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
