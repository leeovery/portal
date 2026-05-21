package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// killerStub is a minimal SessionKiller for internal-package tests. It records
// the killed name and returns the configured err (nil = success).
type killerStub struct {
	killedName string
	err        error
}

func (k *killerStub) KillSession(name string) error {
	k.killedName = name
	return k.err
}

// TestKillRefreshUnderFilterPreservesFilteredList is the canonical
// regression test for the latent variant in which killAndRefresh routes
// through the production SessionsMsg path into applySessions. It exercises
// the full production keystroke path (committed filter -> 'k' -> 'y') and
// asserts the post-kill refresh leaves the filtered list rendered intact
// (with the killed row absent). Length-only assertion is insufficient;
// order-sensitive slice equality on visibleSessionNames is what locks in
// the fix and would catch a regression that re-discarded the propagated
// refilter cmd.
//
// If the SessionsMsg handler in model.go ever reverts to discarding the
// cmd returned by applySessions, this test fails with empty VisibleItems
// while filter metadata still says FilterApplied — the exact wrong-axis
// failure mode that motivated the bugfix.
func TestKillRefreshUnderFilterPreservesFilteredList(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "alphabet", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	// Post-kill universe: alphabet removed. The committed "alpha" filter
	// must continue to match {alpha} only after the refresh.
	postKill := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	// Initial lister value is irrelevant — applySessions has already
	// seeded the list via modelWithSeams. The killAndRefresh cmd
	// invokes ListSessions once to fetch the post-kill snapshot; we
	// rewire to postKill before that cmd runs.
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	killer := &killerStub{}

	m := modelWithSeamsAndLister(first, enum, reader, lister)
	m.sessionKiller = killer

	// Commit a filter narrowing the visible list to {alpha, alphabet}.
	// Mirror the drive used by TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh.
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)
	if !m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=true before kill keystrokes")
	}
	// Cursor on alphabet (the row we'll kill) so the kill target is
	// deterministically the second filtered row.
	m.sessionList.Select(1)
	si, ok := m.selectedSessionItem()
	if !ok || si.Session.Name != "alphabet" {
		t.Fatalf("test setup invariant: expected cursor on %q, got ok=%v name=%q", "alphabet", ok, si.Session.Name)
	}

	// Real-keystroke path: 'k' opens the kill-confirm modal.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	afterK, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after 'k', got %T", updated)
	}
	if afterK.modal != modalKillConfirm {
		t.Fatalf("expected modalKillConfirm after 'k', got %v", afterK.modal)
	}
	if afterK.pendingKillName != "alphabet" {
		t.Fatalf("expected pendingKillName=%q after 'k', got %q", "alphabet", afterK.pendingKillName)
	}

	// 'y' confirms the kill and emits the killAndRefresh cmd.
	updated2, killCmd := afterK.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	afterY, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model after 'y', got %T", updated2)
	}
	if killCmd == nil {
		t.Fatalf("expected non-nil cmd from kill confirmation, got nil")
	}

	// Drain the kill cmd: it calls KillSession then ListSessions and
	// returns a SessionsMsg carrying the post-kill snapshot.
	msg := killCmd()
	if killer.killedName != "alphabet" {
		t.Errorf("expected KillSession(%q), got %q", "alphabet", killer.killedName)
	}
	sessionsMsg, ok := msg.(SessionsMsg)
	if !ok {
		t.Fatalf("expected SessionsMsg from killAndRefresh cmd, got %T", msg)
	}
	if sessionsMsg.Err != nil {
		t.Fatalf("unexpected SessionsMsg error: %v", sessionsMsg.Err)
	}

	// Feed the SessionsMsg back through Update — applySessions runs and
	// returns the propagated filterItems cmd (because the list is in
	// FilterApplied state). drainCmdThroughUpdate round-trips that cmd so
	// VisibleItems() observes the refiltered slice rather than the
	// transient nil filteredItems state.
	updated3, refilterCmd := afterY.Update(sessionsMsg)
	afterRefresh, ok := updated3.(Model)
	if !ok {
		t.Fatalf("expected Model after SessionsMsg, got %T", updated3)
	}
	finalAny := drainCmdThroughUpdate(t, afterRefresh, refilterCmd)
	got, ok := finalAny.(Model)
	if !ok {
		t.Fatalf("expected Model after refilter drain, got %T", finalAny)
	}

	// Filter must still be applied with the original committed text.
	if !got.sessionList.IsFiltered() {
		t.Errorf("expected IsFiltered()=true after kill-refresh, got false")
	}
	if val := got.sessionList.FilterValue(); val != "alpha" {
		t.Errorf("expected FilterValue=%q after kill-refresh, got %q", "alpha", val)
	}
	if got.sessionList.FilterState() != list.FilterApplied {
		t.Errorf("expected FilterState=FilterApplied after kill-refresh, got %v", got.sessionList.FilterState())
	}

	// Core assertion: visible (filter-applied) slice equals the
	// initial filter applied to the post-kill universe. Order-sensitive
	// slice equality — length-only would let row-substitution
	// regressions pass.
	wantNames := []string{"alpha"}
	gotNames := visibleSessionNames(got)
	if len(gotNames) != len(wantNames) {
		t.Fatalf("expected VisibleItems=%v after kill-refresh, got %v", wantNames, gotNames)
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Errorf("expected VisibleItems=%v after kill-refresh, got %v (mismatch at idx %d)", wantNames, gotNames, i)
			break
		}
	}
}
