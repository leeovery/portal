package cmd

// §10.2 concurrent cold-boot bootstrap — progress channel + goroutine wrapper.
//
// On the cold + TUI path the eleven-step orchestrator runs in a goroutine while
// Bubble Tea renders the loading page from frame one. This file owns the seam
// between the two: a buffered progress channel carrying serverStarted + one
// per-step event + a terminal done/fatal marker, plus the receiver tea.Cmd the
// model blocks on (the standard Bubble Tea external-channel pattern — a single
// blocking receive re-issued preserves exact event order even under command
// batching).
//
// The synchronous warm/CLI route never constructs a pipe: it keeps the
// serverStartedKey context delivery and the sync.Once memo untouched. Only the
// cold/TUI route routes serverStarted + progress over this channel (§10.2).

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

// bootstrapProgressBufferSize bounds the progress channel. Eleven real steps
// plus the terminal marker fit comfortably; a generous-but-bounded buffer means
// a fast orchestrator never blocks on a slow render, while the bound prevents an
// unbounded backlog. task 5-3 adds per-session restore events under the same
// label — the buffer absorbs a burst without the orchestrator stalling on send.
const bootstrapProgressBufferSize = 64

// bootstrapProgress is the event shape carried on the progress channel. Exactly
// one terminal event (Done) is sent, last, before the channel closes.
type bootstrapProgress struct {
	// Step is the per-step progress event (Index 1..11, Name the closed
	// StepName). Zero on the terminal event.
	Step bootstrap.StepEvent

	// Label is the friendly-label group placeholder (task 5-4 maps the 11 raw
	// steps onto the 5 friendly labels). Empty today.
	Label string

	// RestoreN / RestoreM are the restore per-session counter placeholders
	// (task 5-3 wires the real values). Zero today.
	RestoreN int
	RestoreM int

	// Done marks the single terminal event. On the terminal event ServerStarted
	// and Warnings carry the orchestrator's return, and Fatal (task 5-6) carries
	// any fatal error. Non-terminal step events leave these zero/nil.
	Done          bool
	ServerStarted bool
	Warnings      []bootstrap.Warning
	Fatal         error // task 5-6 — fatal cold-boot step error

	// FailedStep is the 1-based index of the aborting fatal step (task 5-6),
	// carried on the terminal event alongside Fatal. It is the last emitted step
	// index + 1: a fatal step (1, 2, 3, or 8) emits no step-complete event for
	// itself and nothing after, so the next un-emitted index is the one that
	// failed. Zero on a successful run. Carried on the EVENT (a value copy) so the
	// receiver reads it without touching the goroutine's struct fields — the 5-2
	// carry-forward race-avoidance contract.
	FailedStep int
}

// bootstrapChannelClosedMsg is returned by the receiver tea.Cmd once the
// progress channel has been drained and closed. The model's progress arm
// re-issues the receiver on each non-terminal event; this sentinel is the
// receiver's reply once the goroutine has closed the channel, so a post-close
// re-issue returns immediately rather than blocking — no goroutine leak, no
// blocked receive after the program quits.
type bootstrapChannelClosedMsg struct{}

// bootstrapProgressPipe owns the progress channel, the orchestrator goroutine,
// and the orchestrator's terminal return (serverStarted / warnings / fatal).
// The receiver tea.Cmd is handed to the loading-page model; start launches the
// goroutine that runs the orchestrator with the emitter wired through the
// context and closes the channel on return.
type bootstrapProgressPipe struct {
	ch chan bootstrapProgress

	// Terminal return, set by the goroutine before close and read after the TUI
	// program exits. Reads happen-after the channel close the receiver observed,
	// so no additional synchronisation is required: the model only reads these
	// post-program (single-threaded) once the terminal event has been ingested.
	serverStarted bool
	warnings      []bootstrap.Warning
	err           error
}

// newBootstrapProgressPipe constructs a pipe with a bounded buffered channel.
func newBootstrapProgressPipe() *bootstrapProgressPipe {
	return &bootstrapProgressPipe{
		ch: make(chan bootstrapProgress, bootstrapProgressBufferSize),
	}
}

// start launches the orchestrator goroutine. It wires the context-carried
// progress emitter so each completed step sends a non-terminal event, then sends
// exactly one terminal Done event (carrying serverStarted / warnings / fatal)
// and closes the channel — on success OR fatal — so the receiver always observes
// a close and never leaks a blocked receive.
func (p *bootstrapProgressPipe) start(ctx context.Context, runner bootstrap.Runner) {
	// lastStep tracks the highest step index emitted so far. A fatal step (1, 2,
	// 3, or 8) emits no step-complete event for itself and nothing after, so on a
	// fatal abort lastStep+1 is the index of the step that failed (task 5-6). It
	// is written and read on the single orchestrator goroutine (the emitter runs
	// synchronously inside Run, and the terminal-event read happens after Run
	// returns on the same goroutine), so no synchronisation is required.
	lastStep := 0
	emitCtx := bootstrap.WithProgressEmitter(ctx, func(ev bootstrap.StepEvent) {
		// task 5-3: a restore per-session StepEvent carries RestoreN/M (Index 6 /
		// "Restore"); other step events leave them zero. task 5-4 maps Index→
		// friendly Label. The send is ctx-guarded (carry-forward from 5-2): with
		// the per-session restore events 5-3 adds, a restore of >bufferSize
		// sessions against a TUI that stopped draining (early Quit) would block
		// the orchestrator goroutine forever on a naked send. The select makes the
		// send abandon on cancellation so the goroutine always returns.
		if ev.Index > lastStep {
			lastStep = ev.Index
		}
		p.send(ctx, bootstrapProgress{
			Step:     ev,
			RestoreN: ev.RestoreN,
			RestoreM: ev.RestoreM,
		})
	})

	go func() {
		defer close(p.ch)
		started, warnings, err := runner.Run(emitCtx)
		p.serverStarted = started
		p.warnings = warnings
		p.err = err
		// On a fatal abort, the failed step is the first un-emitted index
		// (lastStep+1). Carried on the EVENT so the receiver reads a value copy,
		// never the goroutine's struct fields (the 5-2 carry-forward race-avoidance
		// contract). Zero on success (err == nil).
		failedStep := 0
		if err != nil {
			failedStep = lastStep + 1
		}
		p.send(ctx, bootstrapProgress{
			Done:          true,
			ServerStarted: started,
			Warnings:      warnings,
			Fatal:         err,
			FailedStep:    failedStep,
		})
	}()
}

// send delivers one progress event, abandoning the send if ctx is cancelled.
// The ctx-guarded select (carry-forward from task 5-2) keeps both the per-step
// emit and the per-session restore emit (task 5-3) non-blocking on cancellation:
// if the TUI stopped draining (early Quit cancels the program's context), a
// burst of >bootstrapProgressBufferSize restore events would otherwise wedge the
// orchestrator goroutine on a full channel forever. On cancellation the event is
// dropped — the receiver is no longer consuming, so the drop is benign.
func (p *bootstrapProgressPipe) send(ctx context.Context, ev bootstrapProgress) {
	select {
	case p.ch <- ev:
	case <-ctx.Done():
	}
}

// receiver returns the tea.Cmd the loading-page model blocks on. It performs a
// single blocking channel receive and maps the event onto a tea.Msg:
//   - a non-terminal step event → tui.BootstrapProgressMsg (the model's arm
//     re-issues this receiver, pulling the next event in order)
//   - the terminal Done event → tui.BootstrapCompleteMsg (drives the gated
//     transition; the model does NOT re-issue, so the next receive is never made
//     by the model — but a defensive re-issue would still terminate, see below)
//   - a closed channel → bootstrapChannelClosedMsg (stops the loop; no leak)
func (p *bootstrapProgressPipe) receiver() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-p.ch
		if !ok {
			return bootstrapChannelClosedMsg{}
		}
		if ev.Done {
			// task 5-6 (§10.5): a fatal terminal event maps onto a
			// tui.BootstrapFatalMsg so the loading-page model enters the error state
			// (failed step ✗ + one-line message) and openTUI extracts ev.Fatal for
			// the non-zero exit — NOT a BootstrapCompleteMsg (which would dismiss the
			// page into the picker). The failed step + message ride the EVENT (value
			// copies), not the pipe's struct fields, per the 5-2 carry-forward.
			if ev.Fatal != nil {
				return fatalMsgFromEvent(ev)
			}
			// task 5-7 rides soft warnings onto the terminal event. bootstrap.Warning
			// and tui.BootstrapWarning are both aliases of warning.Warning, so the
			// slice passes straight through with no copy.
			return tui.BootstrapCompleteMsg{Warnings: ev.Warnings}
		}
		return tui.BootstrapProgressMsg{
			Index:    ev.Step.Index,
			Name:     ev.Step.Name,
			Label:    ev.Label,
			RestoreN: ev.RestoreN,
			RestoreM: ev.RestoreM,
		}
	}
}

// fatalMsgFromEvent builds the §10.5 loading-page fatal message from a terminal
// fatal event. The one-line message is the FatalError.UserMessage (the single
// user-facing line the spec mandates); on the off-chance ev.Fatal is not a
// *bootstrap.FatalError (the orchestrator always wraps fatals, so this is
// defensive), it falls back to err.Error(). The underlying error rides through on
// Err so openTUI can errors.As it back to *bootstrap.FatalError for the code-1
// exit classification.
func fatalMsgFromEvent(ev bootstrapProgress) tui.BootstrapFatalMsg {
	message := ev.Fatal.Error()
	var fatal *bootstrap.FatalError
	if errors.As(ev.Fatal, &fatal) {
		message = fatal.UserMessage
	}
	return tui.BootstrapFatalMsg{
		FailedStep: ev.FailedStep,
		Message:    message,
		Err:        ev.Fatal,
	}
}

// ServerStarted reports the orchestrator's serverStarted return, set by the
// goroutine before the channel closed. Read post-program (cold/TUI route).
func (p *bootstrapProgressPipe) ServerStarted() bool { return p.serverStarted }

// Warnings reports the soft warnings the orchestrator accumulated (task 5-7
// rides these through the terminal event; this accessor is the post-program
// drain seam).
func (p *bootstrapProgressPipe) Warnings() []bootstrap.Warning { return p.warnings }

// Err reports the orchestrator's fatal error, if any (task 5-6 surfaces it as a
// loading-page error state). Read post-program.
func (p *bootstrapProgressPipe) Err() error { return p.err }
