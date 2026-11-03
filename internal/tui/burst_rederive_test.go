package tui

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

func TestBurstDispatch_RederivesLiveMarkedSetOnDeferredResolve(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
		{Name: "charlie"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m.detectDispatched = true

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, _ = pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("precondition: N≥2 Enter while detection is in flight must DEFER, not dispatch")
	}
	if len(adapter.Calls) != 0 {
		t.Fatalf("precondition: no window may open while deferred, got %d", len(adapter.Calls))
	}

	m = markRow(t, m, 0)
	m = markRow(t, m, 2)
	if m.SelectedSessionCount() != 2 {
		t.Fatalf("precondition: expected 2 marked after the toggle, got %d", m.SelectedSessionCount())
	}

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

func TestBurstDispatch_AllUnmarkedDuringDefer_NoOp(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m.detectDispatched = true

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, _ = pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("precondition: N≥2 Enter while detection is in flight must DEFER, not dispatch")
	}

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
