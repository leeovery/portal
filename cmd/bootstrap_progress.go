package cmd

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

const bootstrapProgressBufferSize = 64

// unwindWait bounds the wait, once the picker has quit, for the bootstrap it
// ran to return on its context's end.
const unwindWait = 2 * time.Second

// Exactly one terminal event (Done) is sent, last, before the channel closes.
type bootstrapProgress struct {
	Step bootstrap.StepEvent

	RestoreN int
	RestoreM int

	Done     bool
	Warnings []bootstrap.Warning
	Fatal    error

	// FailedStep is the index of the step a fatal names, zero on a successful
	// run and for a fatal before any step, such as bootstrap.lock's.
	FailedStep int
}

// bootstrapChannelClosedMsg is the receiver's reply once the goroutine has
// closed the channel, so a post-close re-issue returns instead of blocking.
type bootstrapChannelClosedMsg struct{}

type bootstrapProgressPipe struct {
	ch chan bootstrapProgress

	// cancel ends the bootstrap's context, and done closes once it returns.
	cancel     context.CancelFunc
	done       chan struct{}
	unwindWait time.Duration
}

func newBootstrapProgressPipe() *bootstrapProgressPipe {
	return &bootstrapProgressPipe{
		ch:         make(chan bootstrapProgress, bootstrapProgressBufferSize),
		done:       make(chan struct{}),
		unwindWait: unwindWait,
	}
}

// start closes the channel on success and fatal alike, so the receiver always
// observes a close and never leaks a blocked receive.
func (p *bootstrapProgressPipe) start(ctx context.Context, runner bootstrap.Runner) {
	ctx, p.cancel = context.WithCancel(ctx)
	emitCtx := bootstrap.WithProgressEmitter(ctx, func(ev bootstrap.StepEvent) {
		p.send(ctx, bootstrapProgress{
			Step:     ev,
			RestoreN: ev.RestoreN,
			RestoreM: ev.RestoreM,
		})
	})

	go func() {
		defer close(p.done)
		defer close(p.ch)
		warnings, err := runner.Run(emitCtx)
		p.send(ctx, bootstrapProgress{
			Done:       true,
			Warnings:   warnings,
			Fatal:      err,
			FailedStep: failedStep(err),
		})
	}()
}

// unwind ends the bootstrap's context and waits up to unwindWait for it to
// return: the picker has quit, as on Ctrl+C from its loading page, and a
// restore cut short lowers the restoring flag on its way out, which left
// raised would stand the saver down until the next command. A bootstrap past
// the bound is left to the process's exit, as a kill leaves one; either way
// it writes no latch, so the next command finishes its work.
func (p *bootstrapProgressPipe) unwind() {
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(p.unwindWait):
	}
}

// failedStep is the step err, a bootstrap's fatal, names, zero for any other.
func failedStep(err error) int {
	if fatal, ok := errors.AsType[*bootstrap.FatalError](err); ok {
		return fatal.Step
	}
	return 0
}

// send abandons the event if ctx is cancelled. Without the guard, a restore
// burst larger than the buffer against a TUI that stopped draining (early Quit)
// wedges the goroutine on a full channel forever; the drop is benign, because
// nothing is consuming.
func (p *bootstrapProgressPipe) send(ctx context.Context, ev bootstrapProgress) {
	select {
	case p.ch <- ev:
	case <-ctx.Done():
	}
}

// One blocking receive, re-issued per event: that is what preserves exact event
// order even under Bubble Tea's command batching.
func (p *bootstrapProgressPipe) receiver() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-p.ch
		if !ok {
			return bootstrapChannelClosedMsg{}
		}
		if ev.Done {
			// A fatal must not become a BootstrapCompleteMsg, which would dismiss the
			// loading page into the picker.
			if ev.Fatal != nil {
				return fatalMsgFromEvent(ev)
			}
			return tui.BootstrapCompleteMsg{Warnings: ev.Warnings}
		}
		// Only the step index rides the wire; the friendly label is derived
		// consumer-side, which owns that mapping.
		return tui.BootstrapProgressMsg{
			Index:    ev.Step.Index,
			RestoreN: ev.RestoreN,
			RestoreM: ev.RestoreM,
			Waiting:  ev.Step.Waiting,
		}
	}
}

// fatalMsgFromEvent carries the underlying error through on Err so the caller
// can errors.As it back for exit classification. The err.Error() branch is
// defensive: the orchestrator always wraps fatals.
func fatalMsgFromEvent(ev bootstrapProgress) tui.BootstrapFatalMsg {
	message := ev.Fatal.Error()
	if fatal, ok := errors.AsType[*bootstrap.FatalError](ev.Fatal); ok {
		message = fatal.UserMessage
	}
	return tui.BootstrapFatalMsg{
		FailedStep: ev.FailedStep,
		Message:    message,
		Err:        ev.Fatal,
	}
}
