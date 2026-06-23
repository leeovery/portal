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
	emitCtx := bootstrap.WithProgressEmitter(ctx, func(ev bootstrap.StepEvent) {
		// task 5-3 maps a restore per-session StepEvent onto RestoreN/M; task 5-4
		// maps Index→friendly Label. Today each step rides as a raw step event.
		p.ch <- bootstrapProgress{Step: ev}
	})

	go func() {
		defer close(p.ch)
		started, warnings, err := runner.Run(emitCtx)
		p.serverStarted = started
		p.warnings = warnings
		p.err = err
		p.ch <- bootstrapProgress{
			Done:          true,
			ServerStarted: started,
			Warnings:      warnings,
			Fatal:         err,
		}
	}()
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
			// task 5-6 maps ev.Fatal onto a loading-page error msg; task 5-7 rides
			// soft warnings onto the terminal event. Today the terminal event is the
			// plain complete marker that drives the gated transition.
			//
			// bootstrap.Warning and tui.BootstrapWarning are both aliases of
			// warning.Warning, so the slice passes straight through with no copy.
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
