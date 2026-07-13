package tui

// restore-host-terminal-windows-6-7 — pre-flight abort UI: gone flash + prune
// keeping survivors.
//
// If pre-flight (has-session over every marked session on Enter) finds any marked
// session gone — the dominant failure, a session killed between picker-load and
// Enter — the burst goroutine emits a terminal spawnAbortMsg BEFORE spawning
// anything (internal/tui/burst_progress.go: burstRunner.run pre-flights the whole
// selection and returns on the first miss). This file owns the picker-side reaction:
// the red abort banner naming the gone session(s), the gone-row flags, and the
// prune-keeping-survivors selection mutation. Nothing is spawned, no window opens,
// no self-attach — so there is deliberately no adapter/connector/tea.Quit path here.

import (
	"fmt"

	"github.com/leeovery/portal/internal/spawn"
)

// handlePreflightAbort completes the §6-7 pre-flight abort arm of the terminal
// spawnAbortMsg. The msg.Gone slice (list order, from spawn.PreflightMissing) drives:
//
//   - the red section-header abort banner (abortBannerText) naming the gone
//     session(s), composed via the shared spawn.QuoteJoin + spawn.GoneVerb helpers so
//     the picker names sessions identically to the CLI — a single gone session reads
//     `'<session>' is gone — nothing opened`, several read `'s2', 's4' are gone —
//     nothing opened` (the ⚠ glyph is added by renderPreflightAbortHeader);
//   - the transient gone-row flags (goneFlagged) the delegate consults to draw the
//     red ⚠ + `session gone` badge in place of the ●/attached badge;
//   - the prune-keeping-survivors selection mutation — the SAME prune-what's-gone
//     rule as the sticky-selection preview round-trip (pruneSelectionToLiveSessions),
//     here pruning the EXPLICIT gone set so every survivor stays marked and a second
//     Enter proceeds with the survivors rather than re-aborting in a loop.
//
// It clears burst-pending (nothing spawned → no leave-what-opened flash) and stays in
// multi-select mode (multiSelectMode untouched), refreshing the delegate so the
// survivors keep their ● and the gone row shows the red flag. No adapter, connector,
// or self-attach is touched (the goroutine aborted before spawning — §6-3).
func (m Model) handlePreflightAbort(msg spawnAbortMsg) Model {
	m.abortBannerText = fmt.Sprintf(
		"%s %s gone — nothing opened",
		spawn.QuoteJoin(msg.Gone),
		spawn.GoneVerb(len(msg.Gone)),
	)

	// Flag the gone rows (transient; cleared on dismiss/refresh) and prune them from
	// the selection, keeping every survivor marked.
	m.goneFlagged = make(map[string]struct{}, len(msg.Gone))
	for _, name := range msg.Gone {
		m.goneFlagged[name] = struct{}{}
		delete(m.selectedSessions, name)
	}

	// Clear the burst lifecycle state (burstPending false, pipe/cancel nil, counters
	// zeroed) — nothing spawned, so there is nothing to undo and no leave-what-opened
	// flash. Refresh the delegate so the ● clears from the pruned/gone row and the red
	// flag renders while the survivors keep their ●.
	(&m).resetBurstState()
	(&m).refreshSessionDelegate()
	return m
}

// clearAbortBanner dismisses the §6-7 pre-flight abort banner: it clears the banner
// text and the gone-row flags, then refreshes the delegate so the red ⚠/badge clear
// from the (former) gone row on the next frame while every surviving mark keeps its
// ●. Multi-select mode is untouched — dismissal stays in the mode (a subsequent Esc
// with no banner exits the mode as normal, per Task 5.1).
func (m *Model) clearAbortBanner() {
	m.abortBannerText = ""
	m.goneFlagged = nil
	m.refreshSessionDelegate()
}
