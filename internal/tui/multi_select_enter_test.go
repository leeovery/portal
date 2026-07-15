package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// pressEnter drives the §5 multi-select Enter commit through updateSessionList,
// returning both the updated model and the returned cmd so a test can assert the
// N-count boundary (quit vs no-op) as well as the model state.
func pressEnter(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
	return updated.(Model), cmd
}

// TestMultiSelectEnterN0 covers the N=0 boundary: Enter with nothing marked is a
// no-op that exits the mode and stays in the picker (same effect as Esc) — it
// opens nothing and does NOT quit.
func TestMultiSelectEnterN0(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})

	// Enter the mode and clear the auto-marked row (double-`m`) → zero marked.
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

// TestMultiSelectEnterN1 covers the N=1 boundary: Enter with exactly one marked
// session selects that session's name and quits, degenerating to the existing
// single-attach commit (mirroring handleSessionListEnter).
func TestMultiSelectEnterN1(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})

	// Enter mode: mark-on-entry marks the highlighted row (alpha, index 0).
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

// TestMultiSelectEnterN1IgnoresCursor covers the cursor-irrelevance rule: with a
// highlighted-but-unmarked row (alpha) and exactly one OTHER session marked
// (bravo), N=1 Enter opens the MARKED session (bravo), not the highlighted one.
func TestMultiSelectEnterN1IgnoresCursor(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})

	// Enter mode (cursor on alpha, index 0 — mark-on-entry marks alpha), move to
	// bravo (index 1) and mark it, then unmark the auto-marked alpha so exactly bravo
	// remains marked.
	m = pressSession(t, m, pressM)
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM) // mark bravo
	m.sessionList.Select(0)
	m = pressSession(t, m, pressM) // unmark the auto-marked alpha
	if !m.IsSessionSelected("bravo") || m.SelectedSessionCount() != 1 {
		t.Fatalf("precondition: expected exactly bravo marked; count=%d", m.SelectedSessionCount())
	}

	// The cursor rests on alpha — highlighted but unmarked.
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

// TestMultiSelectEnterN2DetectionUnwired covers the N≥2 boundary when host-terminal
// detection is UNWIRED (no detector — every existing test model and the offline
// capture harness): the §6-3 burst arm DEFERS on the unresolved detection and,
// with no detector to dispatch, never resolves — so the mode and the selection stay
// intact, nothing opens, and Enter does not quit. (A resolved-supported terminal
// dispatches the async burst instead — see burst_dispatch_test.go.)
func TestMultiSelectEnterN2DetectionUnwired(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})

	// Enter mode (mark-on-entry marks alpha at index 0) and mark bravo too.
	m = pressSession(t, m, pressM)
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM) // marks bravo
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
