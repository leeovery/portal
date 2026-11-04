package cmd

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

const bootstrapProgressBufferSize = 64

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

	// Written by the goroutine before it closes the channel and read only after
	// the program exits, so the close the receiver observed is the ordering edge
	// and no further synchronisation is needed.
	warnings []bootstrap.Warning
	err      error
}

func newBootstrapProgressPipe() *bootstrapProgressPipe {
	return &bootstrapProgressPipe{
		ch: make(chan bootstrapProgress, bootstrapProgressBufferSize),
	}
}

// start closes the channel on success and fatal alike, so the receiver always
// observes a close and never leaks a blocked receive.
func (p *bootstrapProgressPipe) start(ctx context.Context, runner bootstrap.Runner) {
	emitCtx := bootstrap.WithProgressEmitter(ctx, func(ev bootstrap.StepEvent) {
		p.send(ctx, bootstrapProgress{
			Step:     ev,
			RestoreN: ev.RestoreN,
			RestoreM: ev.RestoreM,
		})
	})

	go func() {
		defer close(p.ch)
		warnings, err := runner.Run(emitCtx)
		p.warnings = warnings
		p.err = err
		p.send(ctx, bootstrapProgress{
			Done:       true,
			Warnings:   warnings,
			Fatal:      err,
			FailedStep: failedStep(err),
		})
	}()
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

func (p *bootstrapProgressPipe) Warnings() []bootstrap.Warning { return p.warnings }

func (p *bootstrapProgressPipe) Err() error { return p.err }
