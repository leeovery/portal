package tui

import (
	"context"
	"log/slog"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
)

// 64 comfortably exceeds any realistic marked-set, so the terminal event's
// naked send always finds buffer space or a waiting receiver.
const burstProgressBufferSize = 64

// Exactly one terminal event (Done=true) is sent, last, before the channel
// closes, carrying either Gone (pre-flight abort — nothing spawned) or the outcome.
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

type spawnProgressMsg struct {
	Done  int
	Total int
}

type spawnCompleteMsg struct {
	Batch      string
	Results    []spawn.WindowResult
	Identity   spawn.Identity
	Resolution spawn.Resolution
	Err        error
}

type spawnAbortMsg struct {
	Gone []string
}

type burstChannelClosedMsg struct{}

type burstProgressPipe struct {
	ch     chan burstProgress
	cancel context.CancelFunc
}

func newBurstProgressPipe() *burstProgressPipe {
	return &burstProgressPipe{ch: make(chan burstProgress, burstProgressBufferSize)}
}

func (p *burstProgressPipe) start(ctx context.Context, run func(ctx context.Context, emit func(burstProgress))) {
	go func() {
		defer close(p.ch)
		run(ctx, func(ev burstProgress) { p.send(ctx, ev) })
	}()
}

// The terminal event must be a naked send: guarding it with ctx races ctx.Done
// on cancel and leaves the picker permanently input-locked. It cannot block —
// last send before the deferred close, with a receiver consuming.
func (p *burstProgressPipe) send(ctx context.Context, ev burstProgress) {
	if ev.Done {
		p.ch <- ev
		return
	}
	select {
	case p.ch <- ev:
	case <-ctx.Done():
	}
}

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

type burstRunner struct {
	burster       *spawn.Burster
	ackChannel    spawn.AckChannelFull
	sessionExists func(string) bool
	external      []string
	trigger       string
	identity      spawn.Identity
	resolution    spawn.Resolution
	logger        *slog.Logger
}

func (r burstRunner) run(ctx context.Context, emit func(burstProgress)) {
	all := append(slices.Clone(r.external), r.trigger)
	if gone := spawn.PreflightMissing(all, r.sessionExists); len(gone) > 0 {
		emit(burstProgress{Done: true, Gone: gone})
		return
	}
	batch, results, err := r.burster.Run(ctx, spawn.AttachSurfaces(r.external), nil, func(done, total int) {
		emit(burstProgress{DoneCount: done, Total: total})
	})
	if err := r.ackChannel.Clean(batch); err != nil {
		spawn.LogAckCleanFailed(r.logger, batch, err)
	}
	emit(burstProgress{
		Done:       true,
		Batch:      batch,
		Results:    results,
		Identity:   r.identity,
		Resolution: r.resolution,
		Err:        err,
	})
}

func WithSessionExists(fn func(string) bool) Option {
	return func(m *Model) { m.sessionExists = fn }
}

func WithAckChannel(ack spawn.AckChannelFull) Option {
	return func(m *Model) { m.ackChannel = ack }
}

func WithSpawnExe(exe spawn.ExecutableResolver) Option {
	return func(m *Model) { m.spawnExe = exe }
}

func WithSpawnEnv(env spawn.WindowEnv) Option {
	return func(m *Model) { m.spawnEnv = env }
}

func (m Model) burstAllConfirmed(msg spawnCompleteMsg) bool {
	_, failed := spawn.PartitionResults(msg.Results)
	return msg.Err == nil && len(msg.Results) == len(m.burstExternal) && len(failed) == 0
}

func (m *Model) resetBurstState() {
	m.burstPending = false
	m.burstPipe = nil
	m.burstCancel = nil
	m.burstTotal = 0
	m.burstDone = 0
	m.burstCancelled = false
}

// Seeds the Opening band without wiring a goroutine, pipe, or cancel.
func WithInitialBurstOpening(done, total int) Option {
	return func(m *Model) {
		if done == 0 && total == 0 {
			return
		}
		m.burstPending = true
		m.burstDone = done
		m.burstTotal = total
	}
}

func (m Model) BurstPending() bool { return m.burstPending }

func (m Model) BurstExternal() []string { return m.burstExternal }

func (m Model) BurstTrigger() string { return m.burstTrigger }

func (m Model) BurstTotal() int { return m.burstTotal }

// BurstDone advances 0…N-1, never reaching N: the trigger self-attaches silently.
func (m Model) BurstDone() int { return m.burstDone }

// Deliberately keeps burstPending true: the goroutine still sends its terminal
// event, so the picker stays input-locked until that lands and never wedges.
func (m Model) cancelBurst() (tea.Model, tea.Cmd) {
	if m.burstCancel != nil {
		m.burstCancel()
	}
	m.burstCancelled = true
	// A seeded capture frame has pending set without a pipe.
	if m.burstPipe == nil {
		return m, nil
	}
	return m, m.burstPipe.receiver()
}

// Walks Items(), not VisibleItems, so marks survive an applied filter;
// de-duped because a multi-tag By-Tag session spans one row per tag.
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

// No snapshot is stashed — the marked set is re-derived at resolution, so
// toggles during the detection window are honoured.
func (m Model) beginBurst(ordered []string) (Model, tea.Cmd) {
	if !m.detectResolved {
		m.pendingBurstEnter = true
		var cmd tea.Cmd
		if !m.detectDispatched {
			cmd = (&m).maybeDispatchDetectionCmd()
		}
		return m, cmd
	}
	return m.decideBurst(ordered)
}

// Branches on the cached resolution, not IsNull(): a recognised-but-undriven
// terminal is non-NULL yet unsupported. The unsupported arm pre-flights first
// so a gone session surfaces and is pruned rather than the banner masking it.
func (m Model) decideBurst(ordered []string) (Model, tea.Cmd) {
	m.pendingBurstEnter = false
	// Empty set: no-op — dispatchBurst would panic indexing the trigger.
	if len(ordered) == 0 {
		return m, nil
	}
	if m.DetectUnsupported() {
		if m.sessionExists != nil {
			if gone := spawn.PreflightMissing(ordered, m.sessionExists); len(gone) > 0 {
				return m.handlePreflightAbort(spawnAbortMsg{Gone: gone}), nil
			}
		}
		m.emitUnsupportedNoop(m.detectIdentity)
		m.setFlash(unsupportedFlashText(m.detectIdentity))
		return m, nil
	}
	return m.dispatchBurst(ordered)
}

func unsupportedFlashText(id spawn.Identity) string {
	return spawn.UnsupportedNoopMessage(id)
}

// Reads adapter/resolution from the detection-time cache, never a fresh
// re-resolve: a config-script recipe re-stats its script on every resolve, so a
// script deleted after detection would slip the gate and nil-panic the burst.
func (m Model) dispatchBurst(ordered []string) (Model, tea.Cmd) {
	external, trigger := spawn.SplitNetN(ordered)

	adapter, resolution := m.detectAdapter, m.detectResolution
	if adapter == nil {
		m.emitUnsupportedNoop(m.detectIdentity)
		m.setFlash(unsupportedFlashText(m.detectIdentity))
		return m, nil
	}
	burster := spawn.NewBurster(adapter, m.ackChannel, m.spawnExe, m.spawnEnv)

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

	runner := burstRunner{
		burster:       burster,
		ackChannel:    m.ackChannel,
		sessionExists: m.sessionExists,
		external:      external,
		trigger:       trigger,
		identity:      m.detectIdentity,
		resolution:    resolution,
		logger:        m.spawnLogger,
	}
	pipe.start(ctx, runner.run)
	return m, pipe.receiver()
}
