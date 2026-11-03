package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func drainCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from preview Esc, got nil")
	}
	return cmd()
}

func pressSpaceThenEsc(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after Space, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("test setup invariant: expected pagePreview after Space, got %v", got.activePage)
	}
	updated2, cmd := got.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	got2, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model after Esc, got %T", updated2)
	}
	msg := drainCmd(t, cmd)
	updated3, _ := got2.Update(msg)
	got3, ok := updated3.(Model)
	if !ok {
		t.Fatalf("expected Model after dismiss msg, got %T", updated3)
	}
	return got3
}

func TestPreviewEscReturnsToSessionsPage(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Attached: false},
	}
	m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))

	got := pressSpaceThenEsc(t, m)

	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions after Esc, got %v", got.activePage)
	}
}

func TestPreviewEscPreservesListCursor(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Attached: false},
		{Name: "bravo", Attached: false},
		{Name: "charlie", Attached: false},
		{Name: "delta", Attached: false},
	}
	m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))
	m.sessionList.Select(3)
	if got := m.sessionList.Index(); got != 3 {
		t.Fatalf("test setup invariant: expected Index()=3 before Space, got %d", got)
	}

	got := pressSpaceThenEsc(t, m)

	if idx := got.sessionList.Index(); idx != 3 {
		t.Errorf("expected sessionList.Index()=3 after Space->Esc, got %d", idx)
	}
}

func TestPreviewEscPreservesNoFilterState(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Attached: false},
		{Name: "bravo", Attached: false},
	}
	m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))
	if m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=false before Space")
	}
	beforeFilterValue := m.sessionList.FilterValue()

	got := pressSpaceThenEsc(t, m)

	if got.sessionList.IsFiltered() {
		t.Errorf("expected IsFiltered()=false after Space->Esc, got true")
	}
	if got.sessionList.FilterValue() != beforeFilterValue {
		t.Errorf("expected FilterValue()=%q, got %q", beforeFilterValue, got.sessionList.FilterValue())
	}
}

func TestPreviewEscPreservesCommittedFilter(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Attached: false},
		{Name: "alphabet", Attached: false},
		{Name: "bravo", Attached: false},
	}
	m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)
	if !m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=true before Space")
	}
	if got := m.sessionList.FilterValue(); got != "alpha" {
		t.Fatalf("test setup invariant: expected FilterValue()=%q before Space, got %q", "alpha", got)
	}

	got := pressSpaceThenEsc(t, m)

	if !got.sessionList.IsFiltered() {
		t.Errorf("expected IsFiltered()=true after Space->Esc, got false")
	}
	if val := got.sessionList.FilterValue(); val != "alpha" {
		t.Errorf("expected FilterValue()=%q after Space->Esc, got %q", "alpha", val)
	}
}

func TestSecondEscClearsCommittedFilterViaListDefault(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Attached: false},
		{Name: "alphabet", Attached: false},
		{Name: "bravo", Attached: false},
	}
	m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)

	afterFirstEsc := pressSpaceThenEsc(t, m)
	if afterFirstEsc.activePage != PageSessions {
		t.Fatalf("test setup invariant: expected PageSessions after first Esc, got %v", afterFirstEsc.activePage)
	}
	if !afterFirstEsc.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected committed filter to survive first Esc, got cleared")
	}

	updated, _ := afterFirstEsc.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after second Esc, got %T", updated)
	}
	if got.sessionList.IsFiltered() {
		t.Errorf("expected committed filter cleared by second Esc, got IsFiltered()=true")
	}
	if got.sessionList.FilterState() == list.FilterApplied {
		t.Errorf("expected FilterState != FilterApplied after second Esc, got FilterApplied")
	}
}

func TestPreviewReopenAfterDismissConstructsFreshPreviewModel(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Attached: false},
	}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)

	afterFirst := pressSpaceThenEsc(t, m)
	if n := len(historyCalls(history)); n != 1 {
		t.Fatalf("expected 1 History read after first open, got %d", n)
	}

	updated, _ := afterFirst.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after second Space, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Errorf("expected pagePreview after second Space, got %v", got.activePage)
	}
	if n := len(historyCalls(history)); n != 2 {
		t.Errorf("expected History reads to bump to 2 after re-open, got %d", n)
	}
}
