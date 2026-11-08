package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
)

// The ⚠ glyph is added by the warning notice band, not the message text.
const burstPreSpawnErrorFlash = "could not start opening windows"

// Leave-what-opened: opened windows are never torn down (Portal does not own the
// host windows and the adapter has no close seam), and the trigger self-attach is
// skipped so the picker stays open.
func (m Model) handleBurstPartialFailure(msg spawnCompleteMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		// Pre-spawn abort: nothing opened, so there is no summary and the
		// selection is left unchanged.
		m.emitNotStarted(msg.Err)
		m.setFlash(burstPreSpawnErrorFlash)
		(&m).resetBurstState()
		return m, nil
	}

	if perm, ok := spawn.FirstPermission(msg.Results); ok {
		m.emitPermission(msg.Identity, msg.Resolution, perm.Result.Detail)
	} else {
		m.emitBurstSummary(msg.Batch, msg.Identity, msg.Resolution, msg.Results, false)
	}

	confirmed, _ := spawn.PartitionResults(msg.Results)

	(&m).applyBurstSelectionMutation(confirmed)
	if !m.burstCancelled {
		if text := burstPartialFailureFlash(msg.Results); text != "" {
			m.setFlash(text)
		}
	}
	(&m).resetBurstState()
	return m, nil
}

// Confirmed sessions unmark so a second Enter retries exactly the
// still-missing set.
func (m *Model) applyBurstSelectionMutation(confirmed []string) {
	for _, name := range confirmed {
		delete(m.selectedSessions, name)
	}
	m.refreshSessionDelegate()
}

// A permission wall surfaces its Guidance once for the batch — every later
// window would hit the identical wall. "" means no band.
func burstPartialFailureFlash(results []spawn.WindowResult) string {
	if perm, ok := spawn.FirstPermission(results); ok {
		return perm.Result.Guidance
	}
	confirmed, failed := spawn.PartitionResults(results)
	if len(failed) == 0 {
		return ""
	}
	return spawn.PartialFailureMessage(failed, len(confirmed) > 0)
}
