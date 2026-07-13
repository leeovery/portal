package tui

// restore-host-terminal-windows-6-3 — the §6 N≥2 picker-burst async pipe.
//
// This file mirrors cmd/bootstrap_progress.go's channel + goroutine + re-issued
// receiver pattern for the picker's spawn burst. On an N≥2 Enter the model
// launches the burst IN A GOROUTINE (dispatchBurst) so Update never blocks: the
// goroutine pre-flights the selection, opens the N-1 external windows through the
// resolved adapter, streams one progress event per window over a bounded channel,
// then sends exactly one terminal event (a pre-flight abort OR the burst outcome)
// and closes the channel. The receiver tea.Cmd performs a single blocking receive
// re-issued per non-terminal event — the standard Bubble Tea external-channel
// pattern, preserving exact event order even under command batching.

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
)

// burstProgressBufferSize bounds the burst progress channel. A burst emits one
// non-terminal event per external window plus a single terminal event; a
// generous-but-bounded buffer means a fast burster never blocks on a slow render,
// while the bound prevents an unbounded backlog. The send is ctx-guarded so a
// cancelled burst (Task 6-8) never wedges the goroutine on a full channel.
const burstProgressBufferSize = 64

// burstProgress is the event shape carried on the picker-burst progress channel,
// mirroring cmd/bootstrap_progress.go's bootstrapProgress. Exactly one terminal
// event (Done=true) is sent, last, before the channel closes:
//   - a non-terminal progress event carries DoneCount/Total (external windows
//     opened so far / the external count)
//   - the terminal event carries EITHER Gone (a pre-flight abort — NOTHING
//     spawned; task 6-7 owns the abort UI) OR the burst outcome
//     (Batch/Results/Identity/Resolution/Err).
type burstProgress struct {
	DoneCount int
	Total     int

	Done       bool
	Batch      string
	Results    []spawn.WindowResult
	Identity   spawn.Identity
	Resolution spawn.Resolution
	Gone       []string
	Err        error
}

// spawnProgressMsg is the non-terminal burst tea.Msg: Done external windows
// opened of Total. Its Update arm re-issues the receiver so the next channel event
// is pulled (mirroring BootstrapProgressMsg).
type spawnProgressMsg struct {
	Done  int
	Total int
}

// spawnCompleteMsg is the terminal burst tea.Msg for a burst that RAN (pre-flight
// passed): the batch id, per-window results, the resolved identity/resolution, and
// any pre-window abort error. The self-attach to the trigger + the selection
// mutation land in tasks 6-4/6-6; this task records the outcome and clears pending.
type spawnCompleteMsg struct {
	Batch      string
	Results    []spawn.WindowResult
	Identity   spawn.Identity
	Resolution spawn.Resolution
	Err        error
}

// spawnAbortMsg is the terminal burst tea.Msg for a pre-flight abort: one or more
// selected sessions vanished between marking and Enter, so NOTHING was spawned.
// Task 6-7 owns the abort UI; this task clears pending.
type spawnAbortMsg struct {
	Gone []string
}

// burstChannelClosedMsg is the receiver's reply once the burst channel has been
// drained and closed — the post-terminal sentinel that stops the receive loop
// without blocking (mirroring bootstrapChannelClosedMsg).
type burstChannelClosedMsg struct{}

// burstProgressPipe owns the burst progress channel and the goroutine's cancel
// func. The receiver tea.Cmd is handed to the Update loop; start launches the
// burst goroutine, which streams a per-window progress event plus one terminal
// event, then closes the channel.
type burstProgressPipe struct {
	ch     chan burstProgress
	cancel context.CancelFunc
}

// newBurstProgressPipe constructs a pipe with a bounded buffered channel.
func newBurstProgressPipe() *burstProgressPipe {
	return &burstProgressPipe{ch: make(chan burstProgress, burstProgressBufferSize)}
}

// start launches the burst goroutine. run is the goroutine body (built by
// burstRunner.run): it receives an emit func that ctx-guards each send. start
// closes the channel on return — on the pre-flight abort, the burst outcome, OR a
// cancellation — so the receiver always observes a close and never leaks a blocked
// receive.
func (p *burstProgressPipe) start(ctx context.Context, run func(ctx context.Context, emit func(burstProgress))) {
	go func() {
		defer close(p.ch)
		run(ctx, func(ev burstProgress) { p.send(ctx, ev) })
	}()
}

// send delivers one burst event, abandoning the send if ctx is cancelled (Task
// 6-8) — mirroring bootstrapProgressPipe.send. On cancellation the event is
// dropped; the receiver is no longer consuming, so the drop is benign and the
// goroutine always returns.
func (p *burstProgressPipe) send(ctx context.Context, ev burstProgress) {
	select {
	case p.ch <- ev:
	case <-ctx.Done():
	}
}

// receiver returns the tea.Cmd the Update loop blocks on. It performs a single
// blocking channel receive and maps the event onto a tea.Msg:
//   - a non-terminal progress event → spawnProgressMsg (its arm re-issues this
//     receiver, pulling the next event in order)
//   - a terminal event with Gone non-empty → spawnAbortMsg (a pre-flight abort)
//   - a terminal event otherwise → spawnCompleteMsg (the burst outcome)
//   - a closed channel → burstChannelClosedMsg (stops the loop; no leak)
func (p *burstProgressPipe) receiver() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-p.ch
		if !ok {
			return burstChannelClosedMsg{}
		}
		if ev.Done {
			if len(ev.Gone) > 0 {
				return spawnAbortMsg{Gone: ev.Gone}
			}
			return spawnCompleteMsg{
				Batch:      ev.Batch,
				Results:    ev.Results,
				Identity:   ev.Identity,
				Resolution: ev.Resolution,
				Err:        ev.Err,
			}
		}
		return spawnProgressMsg{Done: ev.DoneCount, Total: ev.Total}
	}
}

// burstRunner bundles everything the burst goroutine needs. Its run method is the
// goroutine body handed to burstProgressPipe.start.
type burstRunner struct {
	burster       *spawn.Burster
	ackChannel    spawn.AckChannelFull
	sessionExists func(string) bool
	external      []string
	trigger       string
	identity      spawn.Identity
	resolution    spawn.Resolution
}

// run is the burst goroutine body:
//  1. Pre-flight EVERY selected session (the external set AND the trigger's
//     self-attach target). If any vanished between marking and Enter, emit a
//     terminal abort event and STOP — nothing spawns (task 6-7 owns the abort UI).
//  2. Otherwise open the N-1 external windows via the burster, streaming a progress
//     event per window, self-clean the batch's ack markers on ALL paths (CLI
//     parity — bounded, harmless leaks self-expire with the tmux server), then emit
//     the terminal outcome event.
func (r burstRunner) run(ctx context.Context, emit func(burstProgress)) {
	all := append(slices.Clone(r.external), r.trigger)
	if gone := spawn.PreflightMissing(all, r.sessionExists); len(gone) > 0 {
		emit(burstProgress{Done: true, Gone: gone})
		return
	}
	batch, results, err := r.burster.Run(ctx, r.external, func(done, total int) {
		emit(burstProgress{DoneCount: done, Total: total})
	})
	_ = r.ackChannel.Clean(batch)
	emit(burstProgress{
		Done:       true,
		Batch:      batch,
		Results:    results,
		Identity:   r.identity,
		Resolution: r.resolution,
		Err:        err,
	})
}

// WithSessionExists wires the §6 burst pre-flight has-session probe. Nil-tolerant:
// a nil probe leaves the burst pre-flight unwired (the offline capture harness and
// unit tests that never drive a burst). Production passes client.HasSession.
func WithSessionExists(fn func(string) bool) Option {
	return func(m *Model) { m.sessionExists = fn }
}

// WithAckChannel wires the §6 burst token-ack channel (Collect + Clean).
// Nil-tolerant. Production passes spawn.NewServerOptionAckChannel(client, client).
func WithAckChannel(ack spawn.AckChannelFull) Option {
	return func(m *Model) { m.ackChannel = ack }
}

// WithSpawnExe wires the §6 burst executable resolver (the picker's own binary).
// Nil-tolerant. Production passes os.Executable.
func WithSpawnExe(exe spawn.ExecutableResolver) Option {
	return func(m *Model) { m.spawnExe = exe }
}

// WithSpawnGetenv wires the §6 burst PATH reader for attach-argv composition.
// Nil-tolerant. Production passes os.Getenv.
func WithSpawnGetenv(fn func(string) string) Option {
	return func(m *Model) { m.spawnGetenv = fn }
}

// burstAllConfirmed reports whether a terminal spawnCompleteMsg represents a
// FULL-success burst (§6-4): the pre-flight passed (msg.Err == nil), every
// external window produced a result (len == the external set), and every result
// confirmed its token ack. Only then does the trigger window self-attach; any
// other outcome is partial/permission (§6-6) or would be an abort (§6-7). An
// empty external set cannot reach here — N=1 is task 5-7 — so the length check
// never has to reason about a vacuous all-confirmed.
func (m Model) burstAllConfirmed(msg spawnCompleteMsg) bool {
	if msg.Err != nil || len(msg.Results) != len(m.burstExternal) {
		return false
	}
	for _, r := range msg.Results {
		if r.Ack != spawn.AckConfirmed {
			return false
		}
	}
	return true
}

// resetBurstState clears the burst lifecycle fields after a terminal outcome: it
// exits burst-pending, releases the goroutine's pipe + cancel references, and
// zeroes the progress counters + captured results. Used by the §6-4 full-success
// self-attach arm before the tea.Quit handoff.
func (m *Model) resetBurstState() {
	m.burstPending = false
	m.burstPipe = nil
	m.burstCancel = nil
	m.burstTotal = 0
	m.burstDone = 0
	m.burstResults = nil
	m.burstBatch = ""
}

// BurstPending reports whether an async §6 spawn burst is in flight — dispatched
// but not yet terminal (spawnCompleteMsg/spawnAbortMsg) — for testing.
func (m Model) BurstPending() bool { return m.burstPending }

// BurstExternal returns the net-N external set (the marked set minus the trigger)
// the burst opens, in list order, for testing.
func (m Model) BurstExternal() []string { return m.burstExternal }

// BurstTrigger returns the burst's self-attach target — the list-order-last marked
// session — for testing.
func (m Model) BurstTrigger() string { return m.burstTrigger }

// BurstTotal returns N — the full marked-set size including the self-attach
// target — recorded at dispatch, for testing.
func (m Model) BurstTotal() int { return m.burstTotal }

// orderedMarkedSessions walks the session list top-to-bottom and returns the
// marked session names in list order, DE-DUPED by name so a multi-tag By-Tag
// session (which spans one row per tag) appears exactly once at its first list
// position. It walks Items() (the full backing set, not the filtered
// VisibleItems) so every marked session is included regardless of an applied
// filter — "Enter opens the marked set only".
func (m Model) orderedMarkedSessions() []string {
	var ordered []string
	seen := make(map[string]struct{}, len(m.selectedSessions))
	for _, item := range m.sessionList.Items() {
		si, ok := item.(SessionItem)
		if !ok {
			continue
		}
		name := si.Session.Name
		if _, marked := m.selectedSessions[name]; !marked {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		ordered = append(ordered, name)
	}
	return ordered
}

// beginBurst starts the §6-3 N≥2 spawn burst from the list-ordered marked set. It
// gates on the async terminal detection (§6-1): while detection is still in flight
// it DEFERS — stashing the ordered snapshot so the terminalDetectedMsg arm resolves
// the branch — and, if detection was never even dispatched, dispatches it now so
// the defer can actually resolve (otherwise a warm Projects-landing → x → Sessions
// → mark → Enter would defer forever). Once detection has resolved it branches
// immediately via decideBurst.
func (m Model) beginBurst(ordered []string) (Model, tea.Cmd) {
	if !m.detectResolved {
		m.pendingBurstEnter = true
		m.pendingBurstOrdered = ordered
		// Dispatch detection now if it was never kicked off, so the deferred burst
		// resolves rather than hanging. maybeDispatchDetectionCmd is a no-op (nil) when
		// detection is already dispatched or unwired.
		var cmd tea.Cmd
		if !m.detectDispatched {
			cmd = (&m).maybeDispatchDetectionCmd()
		}
		return m, cmd
	}
	return m.decideBurst(ordered)
}

// decideBurst runs the post-detection branch of the N≥2 Enter (detection
// resolved). A supported native/config resolution dispatches the async burst; an
// unsupported resolution is the §6-9 atomic no-op — this terminal cannot open host
// windows, so nothing spawns and the mode + selection stay intact (6-9 adds the
// re-asserted banner). It branches on the cached RESOLUTION, not IsNull(): a
// recognised-but-undriven terminal is non-NULL yet unsupported.
func (m Model) decideBurst(ordered []string) (Model, tea.Cmd) {
	m.pendingBurstEnter = false
	m.pendingBurstOrdered = nil
	if m.DetectUnsupported() {
		return m, nil
	}
	return m.dispatchBurst(ordered)
}

// dispatchBurst launches the async spawn burst for the list-ordered marked set: it
// splits net-N (trigger = self-attach target = the last row; external = the N-1
// opened windows), resolves the adapter from the cached identity (guaranteed
// non-nil on a supported resolution), builds the burster + progress pipe, records
// the burst lifecycle state, launches the goroutine, and returns the receiver
// tea.Cmd the Update loop blocks on.
func (m Model) dispatchBurst(ordered []string) (Model, tea.Cmd) {
	trigger := ordered[len(ordered)-1]
	external := ordered[:len(ordered)-1]

	adapter, resolution := m.resolve(m.detectIdentity)
	burster := spawn.NewBurster(adapter, m.ackChannel, m.spawnExe, m.spawnGetenv)

	ctx, cancel := context.WithCancel(context.Background())
	pipe := newBurstProgressPipe()
	pipe.cancel = cancel

	m.burstPipe = pipe
	m.burstCancel = cancel
	m.burstPending = true
	m.burstTrigger = trigger
	m.burstExternal = external
	m.burstTotal = len(ordered)
	m.burstDone = 0
	m.burstIdentity = m.detectIdentity
	m.burstResolution = resolution

	runner := burstRunner{
		burster:       burster,
		ackChannel:    m.ackChannel,
		sessionExists: m.sessionExists,
		external:      external,
		trigger:       trigger,
		identity:      m.detectIdentity,
		resolution:    resolution,
	}
	pipe.start(ctx, runner.run)
	return m, pipe.receiver()
}
