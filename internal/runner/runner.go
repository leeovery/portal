// Package runner attaches the user's terminal to a session by running the
// backend's attach command as a child in a pseudo-terminal Portal owns,
// copying bytes both ways unchanged and watching the input for Portal's own
// keys, which end the attach.
package runner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"golang.org/x/sys/unix"
)

var logger = log.For("attach")

// ErrNoTerminal is Run's refusal of input or output that is not a terminal,
// or of a terminal Portal cannot open a description of its own on. Run
// returns an error wrapping it before anything starts, so the caller can run
// the attach command itself.
var ErrNoTerminal = errors.New("the attach needs a terminal")

// grace bounds each of the runner's waits but the one for the terminal: for
// the child to read what it was sent, to exit after a detach, a kill or each
// signal that ends it, and for its last output to reach the terminal.
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
	// Kill kills the named session through the backend. Run refuses a config
	// without one.
	Kill func(ctx context.Context, name string) error
	// Requests carries the ends of the attach asked for from outside it. Run
	// takes a request only while it waits for the attach's end, so one it
	// takes is one it carries out: taking it is accepting it. Nil asks for
	// none.
	Requests <-chan Request
	// Began, when set, is called once the attach has begun, before Run takes
	// any request.
	Began func()
	// Forwarded, when set, is called each time input reaches the child, on
	// the loop that forwards it, so it must not block.
	Forwarded func()
}

// Request ends the attach from outside it as the detach key does: to the
// picker, or, when Switch names a session, to that session.
type Request struct {
	// Switch is the session to attach next, empty to ask for the picker.
	Switch string
}

// Outcome is how an attach ended.
type Outcome struct {
	// Action is the action that ended the attach, zero when the child ended
	// on its own, Portal was signalled, or a switch ended it.
	Action attachkeys.Action
	// Switch is the session a switch request named, to attach next.
	Switch string
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
// or request asks for; every other end of an attach exits to the shell.
func (o Outcome) Picker() bool {
	return o.Action == attachkeys.Detach || o.Action == attachkeys.Kill
}

// Run attaches the terminal to cfg.Session until the child ends or an action
// or a request ends the attach, with the terminal in raw mode until Run
// returns. An action, a request, a signal to Portal or the context's end
// detaches the child by sending it zmx's own ctrl+\, except a kill that
// succeeds, whose session's end ends the child. A child still running after
// the grace is sent SIGHUP, then SIGKILL. A signal to Portal, or the terminal
// hanging up, at any point ends the attach with that signal's outcome.
//
// Run reads and writes the terminal through descriptions of its own, so the
// flags of the ones the shell shares never change. Unless zmx's own reset is
// known to have reached the terminal, Run resets it, waiting for the terminal
// to take that last write however long it takes.
//
// The attach is torn down however Run's goroutine leaves it, and a panic on
// that goroutine, in Kill or Began say, is raised again once it is. A command that
// switches sessions rather than attaching, or a config with no Kill, is
// refused.
func Run(ctx context.Context, cfg Config) (Outcome, error) {
	if err := check(cfg); err != nil {
		return Outcome{}, err
	}
	a, err := start(cfg)
	if err != nil {
		return Outcome{}, fmt.Errorf("attach session %q: %w", cfg.Session, err)
	}
	return a.run(ctx)
}

// run awaits the end of the attach and tears it down, however Run's
// goroutine leaves await: returning, panicking, or exiting through
// runtime.Goexit.
func (a *attach) run(ctx context.Context) (out Outcome, err error) {
	var awaited *ending
	defer func() {
		if awaited == nil {
			awaited = a.interrupted(recover())
		}
		end := a.tearDown(*awaited)
		end.report(a.cfg.Session)
		if end.panicked != nil {
			panic(end.panicked)
		}
		out, err = end.out, end.err
	}()
	a.cfg.Began()
	end := a.await(ctx)
	awaited = &end
	return
}

// interrupted is the end of an attach whose goroutine left await without one:
// panicking with r, or, with r nil, exiting through runtime.Goexit. Like
// every end but the child's own and a kill's, it detaches the child.
func (a *attach) interrupted(r any) *ending {
	end := ending{err: fmt.Errorf("attach session %q: Run's goroutine exited", a.cfg.Session), reason: "goexit"}
	if r != nil {
		end = a.panicked(r)
		end.panicked = r
	}
	a.detach()
	return &end
}

// check refuses a config Run cannot attach with.
func check(cfg Config) error {
	switch {
	case len(cfg.Command.Argv) == 0:
		return fmt.Errorf("attach session %q: the backend gave no command to run", cfg.Session)
	case cfg.Command.Switches:
		return fmt.Errorf("attach session %q: the command switches sessions instead of attaching", cfg.Session)
	case cfg.Kill == nil:
		return fmt.Errorf("attach session %q: no kill for the kill key", cfg.Session)
	}
	return nil
}

// reasonPanic is the reason of an end a panic made.
const reasonPanic = "panic"

// ending is how an attach ended: its outcome, or the error that ended it,
// and why, for the log.
type ending struct {
	out            Outcome
	err            error
	reason, detail string
	// panicked is a panic on Run's goroutine, raised again once the attach is
	// torn down.
	panicked any
}

func signalled(sig os.Signal) ending {
	s := sig.(syscall.Signal)
	return ending{out: Outcome{ExitCode: 128 + int(s)}, reason: "signal", detail: unix.SignalName(s)}
}

func cancelled(ctx context.Context) ending {
	return ending{err: context.Cause(ctx), reason: "context"}
}

// report logs the outcome: the code Run returns, or the error it returns
// instead.
func (e ending) report(session string) {
	attrs := []any{"session", session, "reason", e.reason}
	if e.err == nil {
		attrs = append(attrs, "code", e.out.ExitCode)
	}
	if e.detail != "" {
		attrs = append(attrs, "detail", e.detail)
	}
	if e.out.Switch != "" {
		attrs = append(attrs, "target", e.out.Switch)
	}
	if err := cmp.Or(e.err, e.out.Err); err != nil {
		attrs = append(attrs, "error", err)
	}
	logger.Info("outcome", attrs...)
}

// await is the one place an attach ends.
func (a *attach) await(ctx context.Context) ending {
	for {
		select {
		case <-a.winch:
			a.resize()
		case action := <-a.keys:
			return a.act(ctx, action)
		case req := <-a.cfg.Requests:
			return a.request(ctx, req)
		case <-a.exited:
			return ending{out: Outcome{ExitCode: exitCode(a.child.ProcessState)}, reason: "exit"}
		case sig := <-a.signals:
			a.detach()
			return signalled(sig)
		case end := <-a.broken:
			a.detach()
			return end
		case <-ctx.Done():
			a.detach()
			return cancelled(ctx)
		}
	}
}

// act carries out an action, whatever asked for it. A kill that succeeds
// leaves the child to end with the session; one that fails detaches it, so
// the picker can show why. A context that ends during the kill ends the
// attach as it would anywhere else.
func (a *attach) act(ctx context.Context, action attachkeys.Action) ending {
	end := ending{out: Outcome{Action: action}, reason: action.String()}
	if action == attachkeys.Kill {
		end.out.Err = a.cfg.Kill(ctx, a.cfg.Session)
		if ctx.Err() != nil {
			end = cancelled(ctx)
		} else if end.out.Err == nil {
			return end
		}
	}
	a.detach()
	return end
}

// request carries out a request from outside the attach. A switch detaches
// the child as the detach key does, naming its session in the outcome; any
// other request is the detach key's.
func (a *attach) request(ctx context.Context, req Request) ending {
	if req.Switch == "" {
		return a.act(ctx, attachkeys.Detach)
	}
	a.detach()
	return ending{out: Outcome{Switch: req.Switch}, reason: "switch"}
}

// exitCode is the status a shell would report for the child.
func exitCode(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return state.ExitCode()
}
