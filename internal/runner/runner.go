// Package runner attaches the user's terminal to a session by running the
// backend's attach command as a child in a pseudo-terminal Portal owns,
// copying bytes both ways unchanged and watching the input for Portal's own
// keys, which end the attach.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"golang.org/x/sys/unix"
)

var logger = log.For("attach")

// ErrNoTerminal is Run's refusal of input that is not a terminal, returned
// before anything starts, so the caller can run the attach command itself.
var ErrNoTerminal = errors.New("attach input is not a terminal")

// grace bounds each of the runner's waits: for the child to read what it was
// sent, to exit after a detach, a kill or each signal that ends it, and for a
// copy loop to stop.
var grace = 2 * time.Second

// Scanner finds Portal's bound keys in the terminal's input.
type Scanner interface {
	// Scan looks for a bound key in one read of terminal input. forward is the
	// input before the first bound key, to pass on unchanged; found reports a
	// key, and action is what it asks for. Input after the key is dropped:
	// every action ends the attach.
	Scan(read []byte) (forward []byte, action attachkeys.Action, found bool)
}

// Config is one attach.
type Config struct {
	// Session is the session attached to.
	Session string
	// Command attaches a terminal to Session: the backend's AttachCommand.
	Command backend.Command
	// In and Out are the user's terminal.
	In, Out *os.File
	// Scanner finds Portal's keys in what the user types.
	Scanner Scanner
	// Kill kills the named session through the backend.
	Kill func(ctx context.Context, name string) error
}

// Outcome is how an attach ended.
type Outcome struct {
	// Action is the action that ended the attach, zero when the child ended
	// on its own or Portal was signalled.
	Action attachkeys.Action
	// ExitCode is the status to exit to the shell with, as a shell reports
	// one: the child's own when it ended on its own, 128 plus the signal's
	// number when a signal ended the child or Portal's attach, and 0
	// otherwise.
	ExitCode int
	// Err is why the action failed, such as a kill the backend refused, for
	// the picker to show. The attach ended all the same.
	Err error
}

// Picker reports whether the picker follows, which only Portal's own action
// asks for; every other end of an attach exits to the shell.
func (o Outcome) Picker() bool {
	return o.Action == attachkeys.Detach || o.Action == attachkeys.Kill
}

// Run attaches the terminal to cfg.Session until the child ends or an action
// ends the attach, with the terminal in raw mode until Run returns. An
// action, a signal to Portal or the context's end detaches the child by
// sending it zmx's own ctrl+\, except a kill that succeeds, whose session's
// end ends the child. A child still running after the grace is sent SIGHUP,
// then SIGKILL.
func Run(ctx context.Context, cfg Config) (Outcome, error) {
	if !term.IsTerminal(cfg.In.Fd()) {
		return Outcome{}, ErrNoTerminal
	}
	if len(cfg.Command.Argv) == 0 {
		return Outcome{}, fmt.Errorf("attach session %q: the backend gave no command to run", cfg.Session)
	}
	a, err := start(cfg)
	if err != nil {
		return Outcome{}, fmt.Errorf("attach session %q: %w", cfg.Session, err)
	}
	defer a.close()
	return a.await(ctx)
}

// await is the one place an attach ends.
func (a *attach) await(ctx context.Context) (Outcome, error) {
	for {
		select {
		case <-a.winch:
			a.resize()
		case action := <-a.requests:
			return a.act(ctx, action), nil
		case <-a.exited:
			return a.report(Outcome{ExitCode: exitCode(a.child.ProcessState)}, "exit", ""), nil
		case sig := <-a.signals:
			a.detach()
			s := sig.(syscall.Signal)
			return a.report(Outcome{ExitCode: 128 + int(s)}, "signal", unix.SignalName(s)), nil
		case err := <-a.broken:
			a.detach()
			return Outcome{}, fmt.Errorf("attach session %q: %w", a.cfg.Session, err)
		case <-ctx.Done():
			a.detach()
			return Outcome{}, context.Cause(ctx)
		}
	}
}

// act carries out an action, whatever asked for it. A kill that succeeds
// leaves the child to end with the session; one that fails detaches it, so
// the picker can show why.
func (a *attach) act(ctx context.Context, action attachkeys.Action) Outcome {
	out := Outcome{Action: action}
	if action == attachkeys.Kill {
		out.Err = a.cfg.Kill(ctx, a.cfg.Session)
	}
	if action != attachkeys.Kill || out.Err != nil {
		a.detach()
	}
	return a.report(out, actionNames[action], "")
}

// actionNames match the actions' keys in prefs.json.
var actionNames = map[attachkeys.Action]string{
	attachkeys.Detach:        "detach",
	attachkeys.DetachToShell: "detach_to_shell",
	attachkeys.Kill:          "kill",
}

func (a *attach) report(out Outcome, reason, detail string) Outcome {
	attrs := []any{"session", a.cfg.Session, "reason", reason, "code", out.ExitCode}
	if detail != "" {
		attrs = append(attrs, "detail", detail)
	}
	if out.Err != nil {
		attrs = append(attrs, "error", out.Err)
	}
	logger.Info("outcome", attrs...)
	return out
}

// exitCode is the status a shell would report for the child.
func exitCode(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return state.ExitCode()
}
