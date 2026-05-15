package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Sessions-page inline-flash tick-based auto-clear infrastructure (spec
// § Inline flash — feature-local infrastructure > Clear conditions,
// § Replacement on rapid successive bails).
//
// The state primitives (setFlash, clearFlash, flashText, flashGen) live
// on Model in model.go. This file groups the tick-clear plumbing:
//   - flashAutoClearDuration: how long a flash lingers before auto-clear.
//   - flashTickMsg: the Bubble Tea message carrying a captured generation.
//   - flashTickCmd: builds a tea.Cmd that fires a flashTickMsg after the
//     auto-clear duration, capturing the generation at schedule time.
//
// Generation-guard rationale: rapid successive bails must not let a
// stale in-flight tick from a prior flash early-clear the current one.
// Each tick captures m.flashGen at schedule time; on fire the Update
// handler compares the captured gen against the live m.flashGen and
// clears only on match. setFlash bumps flashGen monotonically, so any
// superseded tick mismatches and is silently dropped.
//
// No caller schedules a tick in this task (2-3) — tasks 2-5 and 2-6
// wire the scheduling at bail and replacement-bail moments.

// flashAutoClearDuration is how long an inline flash lingers before the
// tick-based auto-clear fires. Spec § Inline flash > Clear conditions
// notes "~3s as a reasonable default" — long enough to read, short
// enough not to linger.
const flashAutoClearDuration = 3 * time.Second

// flashTickMsg is the Bubble Tea message emitted by a scheduled
// flashTickCmd after flashAutoClearDuration has elapsed. Gen carries
// the model's flashGen value at the moment the tick was scheduled; the
// Update handler compares this against the live flashGen so a tick
// belonging to a superseded flash cannot early-clear the current one.
type flashTickMsg struct {
	// Gen is the flashGen value captured at flashTickCmd construction.
	Gen uint64
}

// flashTickCmd returns a tea.Cmd that, after flashAutoClearDuration,
// emits a flashTickMsg carrying the provided gen. The gen is captured
// by value at call time so each scheduled tick is bound to the exact
// generation that scheduled it; later setFlash calls bump the live gen
// without affecting any pending tick's captured value.
func flashTickCmd(gen uint64) tea.Cmd {
	return tea.Tick(flashAutoClearDuration, func(time.Time) tea.Msg {
		return flashTickMsg{Gen: gen}
	})
}
