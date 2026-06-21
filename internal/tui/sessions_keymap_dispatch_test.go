package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// keymapParityKiller / keymapParityRenamer / keymapParityEnumerator are minimal
// dispatch-routing stubs: the parity tests assert which handler a key reaches,
// not the handler's downstream effect, so the stubs only need to be non-nil.
type keymapParityKiller struct{}

func (keymapParityKiller) KillSession(string) error { return nil }

type keymapParityRenamer struct{}

func (keymapParityRenamer) RenameSession(string, string) error { return nil }

type keymapParityEnumerator struct{}

func (keymapParityEnumerator) ListWindowsAndPanesInSession(string) ([]tmux.WindowGroup, error) {
	return []tmux.WindowGroup{{WindowIndex: 0, WindowName: "w", PaneIndices: []int{0}}}, nil
}

type keymapParityReader struct{}

func (keymapParityReader) Tail(string) ([]byte, error) { return nil, nil }

// sessionsDispatchModel builds a Sessions-page Model seeded with three flat
// session rows for exercising the updateSessionList rune/key dispatch directly.
func sessionsDispatchModel(t *testing.T) Model {
	t.Helper()
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	})
	return m
}

func pressSession(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	updated, _ := m.updateSessionList(msg)
	return updated.(Model)
}

// TestSessionsKeymapRevision locks the §12.2 keymap revision in the live
// updateSessionList dispatch: the p→Projects alias is gone (x is the sole
// Sessions↔Projects toggle), s stays the Sessions-only grouping cycle (and a
// literal filter char while / is focused), nav is ↑/↓ with paging Ctrl+↑/↓ and
// no vim/uppercase/page-jump aliases reach Sessions, and ? remains swallowed
// (not bound to open help).
func TestSessionsKeymapRevision(t *testing.T) {
	t.Run("it no longer dispatches p to the Projects page", func(t *testing.T) {
		m := sessionsDispatchModel(t)
		if m.activePage != PageSessions {
			t.Fatalf("precondition: want PageSessions, got %d", m.activePage)
		}
		m = pressSession(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
		if m.activePage != PageSessions {
			t.Errorf("p must NOT navigate to Projects (§12.2 drops the p alias); active page = %d", m.activePage)
		}
	})

	t.Run("it dispatches x to the Projects page (sole Sessions↔Projects toggle)", func(t *testing.T) {
		m := sessionsDispatchModel(t)
		m = pressSession(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
		if m.activePage != PageProjects {
			t.Errorf("x must toggle to Projects; active page = %d", m.activePage)
		}
	})

	t.Run("it dispatches s to the grouping cycle on Sessions", func(t *testing.T) {
		m := sessionsDispatchModel(t)
		before := m.sessionListMode
		m = pressSession(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
		if m.activePage != PageSessions {
			t.Errorf("s must stay on Sessions (grouping cycle, not a page switch); active page = %d", m.activePage)
		}
		if m.sessionListMode == before {
			t.Errorf("s must advance the grouping mode; mode unchanged at %v", before)
		}
	})

	t.Run("it treats s as a literal filter character while the / input is focused", func(t *testing.T) {
		m := sessionsDispatchModel(t)
		// Focus the filter input (the / binding), then press s.
		m = pressSession(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		if !m.sessionList.SettingFilter() {
			t.Fatalf("precondition: filter input not focused after /")
		}
		before := m.sessionListMode
		m = pressSession(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
		if m.sessionListMode != before {
			t.Errorf("s must be a literal filter char while filtering, not a grouping cycle; mode changed to %v", m.sessionListMode)
		}
		if !m.sessionList.SettingFilter() {
			t.Errorf("filter input should stay focused after a literal s")
		}
	})

	t.Run("it binds ? to open the help modal (no list self-toggle)", func(t *testing.T) {
		// §12.2 / §8.5: Phase 3 binds ? to OUR per-page help modal, replacing the
		// prior swallow. The key is still consumed (no cmd, no page change) so
		// bubbles/list never toggles its own help.
		m := sessionsDispatchModel(t)
		_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: '?', Text: "?"})
		if cmd != nil {
			t.Errorf("? must be consumed (return nil cmd), got a non-nil cmd")
		}
		after := pressSession(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
		if after.activePage != PageSessions {
			t.Errorf("? must not change the active page; got %d", after.activePage)
		}
		if after.modal != modalHelp {
			t.Errorf("? must open the help modal (§8.5); modal = %v, want modalHelp", after.modal)
		}
	})

	t.Run("it does not navigate via vim/uppercase/page-jump aliases on Sessions", func(t *testing.T) {
		bannedNav := []tea.KeyPressMsg{
			{Code: 'j', Text: "j"},
			{Code: 'h', Text: "h"},
			{Code: 'l', Text: "l"},
			{Code: 'g', Text: "g"},
			{Code: 'G', Text: "G"},
			{Code: 'b', Text: "b"},
			{Code: 'u', Text: "u"},
			{Code: 'f', Text: "f"},
			{Code: 'd', Text: "d"},
			{Code: tea.KeyPgUp},
			{Code: tea.KeyPgDown},
			{Code: tea.KeyHome},
			{Code: tea.KeyEnd},
		}
		for _, k := range bannedNav {
			m := sessionsDispatchModel(t)
			start := m.sessionList.Index()
			m = pressSession(t, m, k)
			if m.sessionList.Index() != start {
				t.Errorf("key %+v must not move the Sessions cursor (§12.2: arrows only); index %d → %d", k, start, m.sessionList.Index())
			}
			if m.activePage != PageSessions {
				t.Errorf("key %+v must not change the page; got %d", k, m.activePage)
			}
		}
	})

	t.Run("it moves the cursor with ↑/↓ only", func(t *testing.T) {
		m := sessionsDispatchModel(t)
		start := m.sessionList.Index()
		m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
		if m.sessionList.Index() != start+1 {
			t.Errorf("↓ must move the cursor down one; index %d → %d", start, m.sessionList.Index())
		}
		m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
		if m.sessionList.Index() != start {
			t.Errorf("↑ must move the cursor back up one; index %d", m.sessionList.Index())
		}
	})
}

// TestSessionsRetainedActionParity traces every retained action's dispatch
// target after the §12.2 revision — the only behaviour change is p no longer
// reaching Projects; k/r/n/Enter/Space/Esc/Ctrl+C/q must route exactly as
// before.
func TestSessionsRetainedActionParity(t *testing.T) {
	t.Run("k routes to kill (opens the kill confirm modal)", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m.sessionKiller = keymapParityKiller{}
		m = pressSession(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
		if m.modal != modalKillConfirm {
			t.Errorf("k must open the kill confirm modal; modal = %v", m.modal)
		}
		if m.pendingKillName != "alpha" {
			t.Errorf("kill target = %q, want alpha", m.pendingKillName)
		}
	})

	t.Run("r routes to rename (opens the rename modal)", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m.sessionRenamer = keymapParityRenamer{}
		m = pressSession(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
		if m.modal != modalRename {
			t.Errorf("r must open the rename modal; modal = %v", m.modal)
		}
		if m.renameTarget != "alpha" {
			t.Errorf("rename target = %q, want alpha", m.renameTarget)
		}
	})

	t.Run("space routes to preview", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m.enumerator = keymapParityEnumerator{}
		m.reader = keymapParityReader{}
		m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
		if m.activePage != pagePreview {
			t.Errorf("space must open the preview page; active page = %d", m.activePage)
		}
	})

	t.Run("Enter routes to attach (handleSessionListEnter)", func(t *testing.T) {
		// With no previewAttacher wired, handleSessionListEnter returns (m, nil)
		// without changing page or modal — the same no-op path as before. The
		// assertion is that Enter still routes to that handler (page unchanged,
		// no modal opened), not to any other action.
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.activePage != PageSessions {
			t.Errorf("Enter must stay on Sessions in the no-attacher path; active page = %d", m.activePage)
		}
		if m.modal != modalNone {
			t.Errorf("Enter must not open a modal; modal = %v", m.modal)
		}
	})

	t.Run("q and Ctrl+C quit", func(t *testing.T) {
		for _, k := range []tea.KeyPressMsg{
			{Code: 'q', Text: "q"},
			{Code: 'c', Mod: tea.ModCtrl},
		} {
			m := sessionsDispatchModel(t)
			_, cmd := m.updateSessionList(k)
			if cmd == nil {
				t.Errorf("key %+v must produce a quit cmd, got nil", k)
				continue
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Errorf("key %+v must quit, got a non-quit cmd", k)
			}
		}
	})

	t.Run("Esc quits when no filter is applied", func(t *testing.T) {
		m := sessionsDispatchModel(t)
		_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
		if cmd == nil {
			t.Fatalf("Esc with no filter must quit, got nil cmd")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("Esc with no filter must quit")
		}
	})
}
