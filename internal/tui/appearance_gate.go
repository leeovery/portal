package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tui/theme"
)

// appearanceDetectTimeout is the upper bound on how long the first real paint
// waits for the OSC 11 BackgroundColorMsg reply before falling through to the
// dark fallback (§2.6 detect-or-timeout first-paint gate).
//
// Chosen value: 50ms. Rationale (the spec pins only "tens of ms"): terminals
// that answer OSC 11 do so in single-digit ms, so 50ms gives a comfortable
// margin for the answer to win the race on a real terminal — the correct canvas
// lands on frame one. It also stays invisible against the multi-hundred-ms cold
// bootstrap (the §10 loading path gates the same way) and well under the ~100ms
// "instant" perception threshold, so a non-responding terminal's brief blank
// wait is never perceived as a flash. Smaller (e.g. 10ms) risks racing a slow
// terminal's answer and flipping to the dark fallback prematurely; larger (e.g.
// 200ms) starts to approach the perceptible-pause threshold.
const appearanceDetectTimeout = 50 * time.Millisecond

// appearanceTimeoutMsg is the detect-or-timeout deadline message. It is emitted
// by the tea.Tick armed in Init (auto mode only) after appearanceDetectTimeout.
// When it wins the race against the OSC 11 BackgroundColorMsg, the appearance
// gate resolves to the dark fallback (§2.6). It is ignored once the mode is
// already resolved (the loser of the race never re-resolves — no flip).
type appearanceTimeoutMsg struct{}

// appearanceGate is the reusable detect-or-timeout first-paint mechanism (§2.6 /
// §10.2). It owns the resolved canvas mode and the "may the real canvas paint
// yet?" flag, so a page that gates its first paint (the foundation Sessions
// screen now; the §10 cold-path loading page in Phase 5) shares one resolution
// path rather than re-implementing the race.
//
// The contract is single-resolution: once armed, whichever of the OSC 11 reply
// or the timeout fires FIRST resolves the mode and flips resolved to true; every
// later signal is ignored, so the canvas is painted exactly once and never flips.
//
// A pinned appearance (light/dark, §2.6 override) constructs the gate already
// resolved and unarmable — detection and the timeout wait are skipped entirely.
type appearanceGate struct {
	// mode is the resolved light/dark canvas the owned canvas (§1) is painted
	// for. Dark is the zero value (the §2.6 no-answer fallback), so an unresolved
	// auto gate already carries the fallback canvas; it is simply not painted
	// until the gate resolves.
	mode theme.Mode
	// pending reports whether the detect-or-timeout window is OPEN (the first real
	// paint must wait). It is named negatively on purpose: the zero value (false)
	// means "not pending" = resolved, so a zero-value gate (a struct-literal test
	// model) paints immediately. arm() opens the window (pending=true) on an auto
	// gate; the OSC 11 reply or the timeout closes it. resolved() is the positive
	// read used everywhere else.
	pending bool
	// pinned marks a light/dark appearance override. A pinned gate is never armed
	// (arm is a no-op), so the pin's mode survives and no timeout tick is issued.
	pinned bool
}

// resolved reports whether the first real paint may proceed — the positive read
// of the (negatively-stored) pending flag.
func (g appearanceGate) resolved() bool {
	return !g.pending
}

// newAppearanceGate builds the gate for the given appearance preference (§2.6).
// A pinned light/dark appearance resolves immediately (mode set, pinned=true,
// not pending) so the correct canvas paints from frame one with no detection and
// no wait. Auto is constructed RESOLVED to the dark fallback so a directly
// constructed model paints immediately; production opens the detect-or-timeout
// window explicitly via arm() (see Build), which only un-resolves a non-pinned
// gate.
func newAppearanceGate(appearance prefs.Appearance) appearanceGate {
	switch appearance {
	case prefs.AppearanceLight:
		return appearanceGate{mode: theme.Light, pinned: true}
	case prefs.AppearanceDark:
		return appearanceGate{mode: theme.Dark, pinned: true}
	default:
		// Auto: resolved to the dark fallback by default (pending=false); arm()
		// opens the detect-or-timeout window.
		return appearanceGate{mode: theme.Dark}
	}
}

// arm opens the detect-or-timeout window on a non-pinned (auto) gate: it marks
// the gate pending so View holds the neutral blank frame and Init issues the
// timeout tick, until the OSC 11 reply or the timeout resolves it. It is a no-op
// on a pinned gate (the pin keeps painting from frame one). Production (Build)
// arms the gate for auto appearance; the foundation Sessions screen and the §10
// loading page (Phase 5) share this one mechanism.
func (g *appearanceGate) arm() {
	if g.pinned {
		return
	}
	g.pending = true
}

// timeoutCmd is the tea.Cmd that arms the detect-or-timeout deadline tick. It
// returns nil for an already-resolved gate (a pin, or an auto gate whose window
// is not open) so no spurious wait is issued; for an open (pending) gate it
// schedules the appearanceTimeoutMsg after appearanceDetectTimeout.
func (g appearanceGate) timeoutCmd() tea.Cmd {
	if g.resolved() {
		return nil
	}
	return tea.Tick(appearanceDetectTimeout, func(time.Time) tea.Msg {
		return appearanceTimeoutMsg{}
	})
}

// resolveDark resolves an open gate to the dark fallback (the no-answer /
// timeout outcome, §2.6) and reports whether this call performed the resolution.
// It is a no-op (returning false) once already resolved, so a late timeout that
// lost the race never re-resolves — no second resolution, no flip.
func (g *appearanceGate) resolveDark() bool {
	return g.resolve(theme.Dark)
}

// resolveFromDark resolves an open gate from an OSC 11 reply: dark→Dark,
// light→Light (§2.6). It reports whether this call performed the resolution and
// is a no-op (returning false) once already resolved, so a late reply that lost
// the race never flips the painted canvas.
func (g *appearanceGate) resolveFromDark(isDark bool) bool {
	if isDark {
		return g.resolve(theme.Dark)
	}
	return g.resolve(theme.Light)
}

// resolve is the single-resolution core: it sets the mode and closes the window
// on the FIRST call only (i.e. while the window is open / pending), returning
// true when it performed the resolution and false otherwise. This is the no-flip
// invariant — the canvas mode is fixed exactly once per open window.
func (g *appearanceGate) resolve(mode theme.Mode) bool {
	if g.resolved() {
		return false
	}
	g.mode = mode
	g.pending = false
	return true
}
