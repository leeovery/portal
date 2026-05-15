package tui

import (
	"fmt"
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

// isActionableKey reports whether a tea.KeyMsg is an actionable
// keystroke — i.e. one that should clear an active inline flash as a
// side effect (spec § Inline flash > Clear conditions, § Flash
// interaction with filter input).
//
// Defensive shape: a KeyMsg with a non-zero Type (any named key like
// KeyEnter, KeyEsc, KeyDown, or KeyRunes which is negative) OR a
// non-empty Runes slice counts as actionable. The zero-zero shape
// (Type=0 == keyNUL, Runes=nil) is treated as non-actionable — a
// defensive guard against unusual library-emitted no-op KeyMsgs.
// In practice every real keystroke satisfies one of these conditions,
// so this is effectively "any non-empty KeyMsg" while remaining safe
// against the pathological empty case.
//
// Non-KeyMsg events (WindowSizeMsg, FocusMsg, BlurMsg, MouseMsg) never
// reach a `case tea.KeyMsg` branch, so the flash is unaffected by them
// without any code here.
func isActionableKey(msg tea.KeyMsg) bool {
	return msg.Type != 0 || len(msg.Runes) > 0
}

// formatSessionGoneFlash returns the spec-exact wording for the
// session-killed-externally bail flash: `session "<name>" no longer exists`
// (spec § Session-killed-externally bail path > Behaviour). Literal
// double-quote bytes wrap the name — never %q — so output is byte-exact
// regardless of name content (spaces, dashes, unicode, etc.).
//
// No trailing punctuation. No paraphrase. Callers must not modify the
// returned string before passing it to setFlash.
func formatSessionGoneFlash(name string) string {
	return fmt.Sprintf(`session "%s" no longer exists`, name)
}
