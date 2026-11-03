package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func pressEnter(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
	return updated.(Model), cmd
}

func TestMultiSelectEnterN0(t *testing.T) {
	m := NewModelWithSessions([]Session{
		{Name: "alpha"},
		{Name: "bravo"},
	})

	m = enterMultiSelectEmpty(t, m)
	if !m.MultiSelectActive() || m.SelectedSessionCount() != 0 {
		t.Fatalf("precondition: expected in-mode with zero marked; active=%v count=%d",
			m.MultiSelectActive(), m.SelectedSessionCount())
	}

	m, cmd := pressEnter(t, m)

	if m.MultiSelectActive() {
		t.Errorf("N=0 Enter must exit multi-select mode (same effect as Esc)")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("N=0 Enter must leave the set empty; count = %d, want 0", got)
	}
	if got := m.Selected(); got != "" {
		t.Errorf("N=0 Enter must open nothing; Selected() = %q, want \"\"", got)
	}
	if isQuitCmd(cmd) {
		t.Errorf("N=0 Enter must NOT quit (Portal stays open)")
	}
}

func TestMultiSelectEnterN1(t *testing.T) {
	m := NewModelWithSessions([]Session{
		{Name: "alpha"},
		{Name: "bravo"},
	})

	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("alpha") || m.SelectedSessionCount() != 1 {
		t.Fatalf("precondition: expected exactly alpha marked; count=%d", m.SelectedSessionCount())
	}

	m, cmd := pressEnter(t, m)

	if got := m.Selected(); got != "alpha" {
		t.Errorf("N=1 Enter must select the one marked session; Selected() = %q, want \"alpha\"", got)
	}
	if !isQuitCmd(cmd) {
		t.Errorf("N=1 Enter must return tea.Quit (drives the single-attach connector)")
	}
}

func TestMultiSelectEnterN1IgnoresCursor(t *testing.T) {
	m := NewModelWithSessions([]Session{
		{Name: "alpha"},
		{Name: "bravo"},
	})

	m = pressSession(t, m, pressM)
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM)
	m.sessionList.Select(0)
	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("bravo") || m.SelectedSessionCount() != 1 {
		t.Fatalf("precondition: expected exactly bravo marked; count=%d", m.SelectedSessionCount())
	}

	if si, ok := m.selectedSessionItem(); !ok || si.Session.Name != "alpha" {
		t.Fatalf("precondition: cursor must be on the unmarked alpha row")
	}

	m, cmd := pressEnter(t, m)

	if got := m.Selected(); got != "bravo" {
		t.Errorf("N=1 Enter must open the MARKED session, not the highlighted cursor row; Selected() = %q, want \"bravo\"", got)
	}
	if !isQuitCmd(cmd) {
		t.Errorf("N=1 Enter must return tea.Quit")
	}
}

func TestMultiSelectEnterN2DetectionUnwired(t *testing.T) {
	m := NewModelWithSessions([]Session{
		{Name: "alpha"},
		{Name: "bravo"},
	})

	m = pressSession(t, m, pressM)
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM)
	if m.SelectedSessionCount() != 2 {
		t.Fatalf("precondition: expected two marked; count=%d", m.SelectedSessionCount())
	}

	m, cmd := pressEnter(t, m)

	if m.BurstPending() {
		t.Errorf("N≥2 Enter with detection unwired must DEFER, not dispatch a burst")
	}
	if !m.MultiSelectActive() {
		t.Errorf("N≥2 Enter must leave multi-select mode intact")
	}
	if got := m.SelectedSessionCount(); got != 2 {
		t.Errorf("N≥2 Enter must leave the selection intact; count = %d, want 2", got)
	}
	if got := m.Selected(); got != "" {
		t.Errorf("N≥2 Enter must open nothing; Selected() = %q, want \"\"", got)
	}
	if isQuitCmd(cmd) {
		t.Errorf("N≥2 Enter must NOT quit")
	}
}
