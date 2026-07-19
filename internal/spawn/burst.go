package spawn

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/session"
)

// spawnAckTimeout is the per-window budget for a spawned window's token ack: the
// wall-clock window the burster waits for one spawned window's
// @portal-spawn-<batch>-<token> marker to appear before classifying that window
// as a failed spawn.
//
// It must comfortably cover the whole spawn→confirm chain for a single window:
//
//	~260ms   the osascript open (measured: 4 sequential opens ~1.05s / ~260ms
//	         each — see spec § Sequential vs parallel), plus
//	         the spawned window's own abridged `portal attach` (warm-command
//	         fast-path, NO full bootstrap) up to the point it writes its token
//	         marker just before exec.
//
// ~8s gives generous headroom over that sub-second path — the same spirit as the
// documented daemon self-supervision hysteresis constant (a build-time budget
// confirmed against real abridged-attach timing, not a hard SLA). It is
// deliberately per-window: each window's timer starts at ITS OWN spawn, so the
// cumulative delay of earlier sequential windows never eats a later window's
// budget (see awaitToken / Burster.Run). Tunable — changing this one constant
// changes every window's ack budget.
const spawnAckTimeout = 8 * time.Second

// defaultAckPoll is the interval between ack-marker Collect probes while awaiting
// a window's token. Small relative to spawnAckTimeout so a token that lands early
// is confirmed promptly, but coarse enough to keep the poll count bounded.
const defaultAckPoll = 75 * time.Millisecond

// AckOutcome is the closed per-window confirmation vocabulary the burster tags
// each spawned window with — exactly the spec's `ack` attr values.
type AckOutcome string

const (
	// AckConfirmed — the window's token marker appeared within its budget.
	AckConfirmed AckOutcome = "confirmed"
	// AckTimeout — the window opened but its token never appeared before the
	// per-window spawnAckTimeout elapsed (a missing marker at timeout = a failed
	// spawn).
	AckTimeout AckOutcome = "timeout"
	// AckFailed — the adapter itself reported no window opened, so there is
	// nothing to await.
	AckFailed AckOutcome = "failed"
)

// WindowResult is the outcome of attempting one external window: the target's
// value (a session name for an attach surface, the literal dir for a mint
// surface — carried in Session), its opaque per-window ack Token, the adapter's
// Result, and the resolved token-ack classification (Ack). The burster returns
// one per external surface, in list order.
type WindowResult struct {
	Session string
	Token   string
	Result  Result
	Ack     AckOutcome
}

// Burster is the N−1 external half of the spawn burst: it generates a batch id +
// one opaque token per external window, opens each window sequentially through
// Adapter, and confirms each by watching Ack for that window's token within a
// per-window Timeout. The Nth self-attach is the caller's concern.
//
// Every seam is injectable so the whole flow is unit-testable under a fake clock
// with no real time, tmux, or osascript: Ack (the read-side marker channel),
// Exe/Getenv (attach-argv composition), NewID (raw id generator wrapped by
// NewSpawnID per id), Timeout/Poll (the per-window ack budget + poll cadence),
// and Now/Sleep (the clock). NewBurster applies production defaults.
type Burster struct {
	Adapter Adapter
	Ack     AckCollector
	Exe     ExecutableResolver
	Getenv  func(string) string
	NewID   func() (string, error)
	Timeout time.Duration
	Poll    time.Duration
	Now     func() time.Time
	Sleep   func(time.Duration)
}

// NewBurster wires a Burster to its adapter + ack channel + composition seams and
// applies production defaults for the id generator, per-window timeout, poll
// cadence, and clock. Production passes the resolved native adapter, the shared
// server-option ack channel, os.Executable, and os.Getenv.
func NewBurster(adapter Adapter, ack AckCollector, exe ExecutableResolver, getenv func(string) string) *Burster {
	return &Burster{
		Adapter: adapter,
		Ack:     ack,
		Exe:     exe,
		Getenv:  getenv,
		NewID:   session.NewNanoIDGenerator(),
		Timeout: spawnAckTimeout,
		Poll:    defaultAckPoll,
		Now:     time.Now,
		Sleep:   time.Sleep,
	}
}

// Run opens one external host-terminal window per surface in external, in list
// order, and returns the batch id plus one WindowResult per window. Each surface
// carries the resolved open target (attach an existing session, or mint a fresh
// session at a literal dir); the burster only SPAWNS each window's `open` argv —
// minting happens in the window at exec time (via `open --path`), never here.
//
// The picker's own executable is resolved ONCE up front (an unresolvable
// executable aborts the whole burst before any window opens: return "", nil,
// err). ALL ids are generated up front too — the batch id and one token per
// external surface — so a generation failure aborts before any window opens
// (Task 3.1's "never an empty/malformed id" propagates here). Composing each
// argv from the once-resolved exePath via the pure composeOpenArgv builder keeps
// behaviour identical to resolving per window while avoiding a redundant
// os.Executable read for every window.
//
// Each window is then, sequentially: composed, opened, and — if the adapter
// reported success — awaited for its token via awaitToken (a per-window timer
// starting at THIS window's spawn). A window the adapter could not open is
// AckFailed and never awaited. The loop does not stop early on a failed or
// timed-out window (every window that can open does open); the SOLE early-stop
// is permission-required — the macOS Automation grant is per-(source, target),
// so once window k hits the wall every later window would hit the identical
// wall, and windows k+1…N−1 are never composed or handed to the adapter.
//
// progress (nil-tolerant) is invoked as progress(i+1, len(external)) after each
// window's ack classification — the picker burst pipe streams it to the loading
// UI, while the CLI passes nil for byte-identical Phase-2/3 behaviour. ctx is
// checked between windows AND inside awaitToken's poll loop (Task 6.8 drives the
// cancel): on cancellation the burst stops iterating and returns what it has
// collected so far (a nil error — a cancelled burst is a shutdown, not a
// failure). A context.Background() ctx never cancels, so the CLI path is
// unchanged.
func (b *Burster) Run(ctx context.Context, external []Surface, progress func(done, total int)) (batch string, results []WindowResult, err error) {
	exePath, err := b.Exe()
	if err != nil {
		return "", nil, err
	}
	path := b.Getenv("PATH")

	batch, err = NewSpawnID(b.NewID)
	if err != nil {
		return "", nil, err
	}
	tokens := make([]string, len(external))
	for i := range external {
		token, terr := NewSpawnID(b.NewID)
		if terr != nil {
			return "", nil, terr
		}
		tokens[i] = token
	}

	results = make([]WindowResult, 0, len(external))
	for i, surface := range external {
		// Between-windows cancel check (Task 6.8): a cancelled burst stops before
		// composing or opening the next window.
		if ctx.Err() != nil {
			break
		}
		token := tokens[i]
		argv := composeOpenArgv(exePath, path, surface, batch, token)
		result := b.Adapter.OpenWindow(argv)

		ack := AckFailed
		if result.OK() {
			ack = awaitToken(ctx, b, batch, token)
		}
		results = append(results, WindowResult{Session: surface.Value, Token: token, Result: result, Ack: ack})

		if progress != nil {
			progress(i+1, len(external))
		}

		// The sole early-stop: a permission wall on this window means every later
		// window (same source → same target) would hit it too, so stop rather than
		// grind through k+1…N−1. Timeout / spawn-failed do NOT break (Task 3.6).
		if result.Outcome == OutcomePermissionRequired {
			break
		}
	}
	return batch, results, nil
}

// awaitToken polls b.Ack.Collect for token, returning AckConfirmed the moment it
// appears and AckTimeout once b.Timeout elapses without it. The per-window timer
// starts here (start := b.Now()), right after this window's OpenWindow — so the
// cumulative sequential delay of earlier windows never eats this window's budget.
//
// A Collect error is treated as "token not present yet" (the loop is bounded by
// the timer, so a persistently failing enumeration classifies as AckTimeout —
// the same treatment as a genuinely missing marker). All timing flows through
// the injected Now/Sleep/Poll/Timeout, so the loop is deterministic under a fake
// clock.
//
// It also honours ctx cancellation (Task 6.8): a cancelled context ends the poll
// immediately as AckTimeout (the caller's between-windows check then stops the
// burst). A context.Background() ctx never cancels, so the CLI path polls exactly
// as before.
func awaitToken(ctx context.Context, b *Burster, batch, token string) AckOutcome {
	start := b.Now()
	for {
		if ctx.Err() != nil {
			return AckTimeout
		}
		if tokens, cerr := b.Ack.Collect(batch); cerr == nil {
			if _, ok := tokens[token]; ok {
				return AckConfirmed
			}
		}
		b.Sleep(b.Poll)
		if b.Now().Sub(start) >= b.Timeout {
			return AckTimeout
		}
	}
}
