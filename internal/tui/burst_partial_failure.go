package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
)

// §6-6 partial-failure leave-what-opened + selection mutation.
//
// Once a burst is past pre-flight, a rare per-window hiccup can still occur — a
// transient adapter spawn-failure, a token that never arrives (ack timeout), or the
// native adapter's defensive permission-required path. On ANY non-all-confirmed
// terminal outcome Portal must NOT tear down the windows that already opened (it does
// not own those host windows — and there is deliberately no teardown seam), skip the
// trigger's self-attach so the picker stays open, unmark the sessions whose windows
// opened, and keep the failed / un-acked / un-attempted ones marked so a second Enter
// retries exactly the still-missing set. The markers were already self-cleaned by the
// burst goroutine on every terminal path (§6-3), so no clean happens here.

// burstPreSpawnErrorFlash is the §6-6 generic flash for a Burster.Run pre-spawn error
// (an os.Executable resolution failure or an ack-id generation failure) that occurred
// BEFORE any window opened. The opaque error rides only to the DEBUG log (§6-10); the
// user sees this fixed copy. The ⚠ glyph is added by the warning notice band
// (statusGlyph), NOT the message text — matching the formatSessionGoneFlash /
// formatWarningsFlash convention, so the band renders it exactly once.
const burstPreSpawnErrorFlash = "could not start opening windows"

// handleBurstPartialFailure completes the non-all-confirmed arm of the terminal
// spawnCompleteMsg (§6-6): a pre-spawn Burster.Run error, or a post-flight burst where
// at least one external window failed / timed out / hit the permission wall. It skips
// the trigger self-attach (no m.selected / no tea.Quit — the picker stays in
// multi-select mode), clears burst-pending, and surfaces one transient flash. No
// opened window is torn down (spawn.Adapter has no close seam).
func (m Model) handleBurstPartialFailure(msg spawnCompleteMsg) (Model, tea.Cmd) {
	// §7-1: the permission-required outcome is its own closed-catalog event. Detect it
	// via the shared spawn.FirstPermission (the single count-semantics chokepoint) and
	// route it to emitPermission — the dedicated INFO with NO opened/total summary —
	// matching cmd/spawn.go's logSpawnPermission branch, which skips logSpawnSummary.
	// Every other non-all-confirmed outcome (a timeout / spawn-failed partial, or a
	// pre-spawn Burster.Run error with empty Results) emits the generic batch summary:
	// opened = the confirmed external windows only (the skipped trigger self-attach is
	// NOT counted); total = N still (external set + the one trigger).
	if perm, ok := spawn.FirstPermission(msg.Results); ok {
		m.emitPermission(msg.Identity, msg.Resolution, perm.Result.Detail)
	} else {
		m.emitBurstSummary(msg.Batch, msg.Identity, msg.Resolution, msg.Results, false)
	}

	if msg.Err != nil {
		// Pre-spawn abort from Burster.Run (os.Executable / ack-id failure) BEFORE any
		// window opened → msg.Results is empty. Nothing opened → nothing to unmark, so
		// the selection is left UNCHANGED (no degenerate empty-named "failed to open").
		// The opaque msg.Err rides only to the DEBUG log (§6-10). This is the picker
		// analogue of the CLI's `return err` on the same Burster.Run error, surfaced as
		// a flash instead of an exit.
		m.setFlash(burstPreSpawnErrorFlash)
		(&m).resetBurstState()
		return m, nil
	}

	// §7-1: derive the confirmed/failed partition from the shared spawn helper rather
	// than a hand-rolled Ack loop, so the leave-what-opened mutation and the failed-
	// window flash key off the same count-semantics rule the CLI does.
	confirmed, failed := spawn.PartitionResults(msg.Results)

	(&m).applyBurstSelectionMutation(confirmed)
	// A user cancel (§6-8) converges here — same leave-what-opened mutation — but is
	// SILENT: cancellation is user-initiated, so no failed-window flash. A non-cancel
	// partial surfaces the flash, guarded so the degenerate empty-`failed` case (no
	// failed window, no permission wall — burstPartialFailureFlash returns "") renders
	// no band. resetBurstState clears burstCancelled on the way out.
	if !m.burstCancelled {
		if text := burstPartialFailureFlash(msg.Results, failed); text != "" {
			m.setFlash(text)
		}
	}
	(&m).resetBurstState()
	return m, nil
}

// applyBurstSelectionMutation implements the §6-6 leave-what-opened selection rule:
// every session whose window CONFIRMED is unmarked (its host window opened, so a retry
// must not re-open it); every other marked session — a failed / timed-out window, or
// one never attempted after the burst stopped on a permission wall — stays marked, so
// a second Enter retries exactly the still-missing set. The trigger self-attach did not
// happen and the trigger is never in the external set, so it stays marked too. The
// delegate is refreshed so the ● clears from the unmarked rows and remains on the
// still-marked set.
func (m *Model) applyBurstSelectionMutation(confirmed []string) {
	for _, name := range confirmed {
		delete(m.selectedSessions, name)
	}
	m.refreshSessionDelegate()
}

// burstPartialFailureFlash builds the §6-6 leave-what-opened flash. If ANY window hit
// the permission wall its driver-composed Guidance is surfaced verbatim, once for the
// batch — the burst already stopped on the first permission wall (§6-3), and every
// later window (same source → same target) would hit the identical wall, so the
// generic per-window failed-window copy would be misleading. With no permission wall
// and no failed window (a degenerate partial — e.g. a burst that stopped early with
// every attempted window confirmed) it returns "" so the caller renders NO band,
// avoiding the leading-space "  failed to open …". Otherwise it names every failed
// window via the shared spawn.QuoteJoin renderer. The opaque Result.Detail never
// reaches the user (DEBUG log only, §6-10). The ⚠ glyph is added by the warning notice
// band, so the message text carries none (formatSessionGoneFlash convention).
func burstPartialFailureFlash(results []spawn.WindowResult, failed []string) string {
	if perm, ok := spawn.FirstPermission(results); ok {
		return perm.Result.Guidance
	}
	if len(failed) == 0 {
		return ""
	}
	return spawn.PartialFailureMessage(failed)
}
