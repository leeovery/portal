package tui

// restore-host-terminal-windows-7-5 — re-derive the marked set at burst decision
// time so a deferred N≥2 Enter cannot open a stale selection.
//
// When an N≥2 Enter lands before async terminal detection resolves, beginBurst
// DEFERS without engaging the burst input-lock (burstPending stays false), so a
// subsequent `m` toggle during the tiny defer window is processed normally and
// mutates selectedSessions. These white-box tests pin that the burst opens the
// LIVE marked set as it stands when terminalDetectedMsg resolves — not a stale
// snapshot captured at Enter time — and that unmarking everything during the
// window is a safe no-op rather than a panic.
//
// No t.Parallel: consistent with the rest of the tui test surface.

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// TestBurstDispatch_RederivesLiveMarkedSetOnDeferredResolve is the core §7-5
// assertion: an N≥2 Enter deferred while detection is in flight, followed by a
// mark toggle during the defer window, spawns the POST-toggle live selection when
// terminalDetectedMsg resolves — a session unmarked during the window is NOT
// opened, and one newly marked IS.
func TestBurstDispatch_RederivesLiveMarkedSetOnDeferredResolve(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	// Detection dispatched but not yet resolved (in-flight → the Enter defers).
	m.detectDispatched = true

	// Mark alpha + bravo → snapshot-at-Enter would be external=[alpha], trigger=bravo.
	m = pressSession(t, m, pressM)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, _ = pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("precondition: N≥2 Enter while detection is in flight must DEFER, not dispatch")
	}
	if len(adapter.Calls) != 0 {
		t.Fatalf("precondition: no window may open while deferred, got %d", len(adapter.Calls))
	}

	// Mark toggle DURING the defer window: unmark alpha, mark charlie. Live selection
	// is now {bravo, charlie} → external=[bravo], trigger=charlie.
	m = markRow(t, m, 0) // unmark alpha
	m = markRow(t, m, 2) // mark charlie
	if m.SelectedSessionCount() != 2 {
		t.Fatalf("precondition: expected 2 marked after the toggle, got %d", m.SelectedSessionCount())
	}

	// Detection resolves → the deferred burst re-derives the LIVE set and dispatches it.
	updated, cmd := m.Update(terminalDetectedMsg{identity: ghosttyIdentity()})
	m = updated.(Model)
	if !m.BurstPending() {
		t.Fatal("resolving detection must dispatch the deferred burst (supported → dispatch)")
	}

	if got := m.BurstTrigger(); got != "charlie" {
		t.Errorf("BurstTrigger = %q, want charlie (the live post-toggle set, not the stale snapshot)", got)
	}
	if got := m.BurstExternal(); !slices.Equal(got, []string{"bravo"}) {
		t.Errorf("BurstExternal = %v, want [bravo] (the live post-toggle set, not the stale [alpha])", got)
	}

	m = drainBatchToModel(t, m, cmd)
	if len(adapter.Calls) != 1 {
		t.Fatalf("OpenWindow called %d times, want 1 (external = [bravo])", len(adapter.Calls))
	}
	if got := spawnedSession(t, adapter.Calls[0]); got != "bravo" {
		t.Errorf("deferred burst opened %q, want bravo (the newly-marked live external); alpha (unmarked in the window) must NOT open", got)
	}
}

// TestBurstDispatch_AllUnmarkedDuringDefer_NoOp pins the edge case: if EVERY
// session is unmarked during the pre-detection defer window, the deferred Enter
// resolves to a safe no-op — no burst dispatches, no window opens, and the model
// does not panic indexing an empty ordered set.
func TestBurstDispatch_AllUnmarkedDuringDefer_NoOp(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m.detectDispatched = true

	m = pressSession(t, m, pressM)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, _ = pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("precondition: N≥2 Enter while detection is in flight must DEFER, not dispatch")
	}

	// Unmark both during the defer window → live selection is empty.
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	if m.SelectedSessionCount() != 0 {
		t.Fatalf("precondition: expected 0 marked after unmarking, got %d", m.SelectedSessionCount())
	}

	updated, cmd := m.Update(terminalDetectedMsg{identity: ghosttyIdentity()})
	m = updated.(Model)

	if m.BurstPending() {
		t.Error("an all-unmarked deferred Enter must NOT dispatch a burst")
	}
	if cmd != nil {
		t.Error("an all-unmarked deferred Enter must be a no-op (nil cmd)")
	}
	if len(adapter.Calls) != 0 {
		t.Errorf("no window may open when the live marked set is empty, got %d", len(adapter.Calls))
	}
}
