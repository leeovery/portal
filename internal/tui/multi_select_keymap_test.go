package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

var (
	keyK     = tea.KeyPressMsg{Code: 'k', Text: "k"}
	keyX     = tea.KeyPressMsg{Code: 'x', Text: "x"}
	keyR     = tea.KeyPressMsg{Code: 'r', Text: "r"}
	keyQ     = tea.KeyPressMsg{Code: 'q', Text: "q"}
	keyCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace}
	keySlash = tea.KeyPressMsg{Code: '/', Text: "/"}
	keyN     = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

func enterMultiSelect(t *testing.T, m Model) Model {
	t.Helper()
	m = pressSession(t, m, pressM)
	if !m.MultiSelectActive() {
		t.Fatalf("precondition: model must be in multi-select mode after m")
	}
	return m
}

func enterMultiSelectEmpty(t *testing.T, m Model) Model {
	t.Helper()
	m = pressSession(t, m, pressM)
	m = pressSession(t, m, pressM)
	if !m.MultiSelectActive() {
		t.Fatalf("precondition: model must be in multi-select mode after m")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Fatalf("precondition: enter-then-clear must yield an empty set, got %d", got)
	}
	return m
}

func twoFlatSessions() []Session {
	return []Session{
		{Name: "alpha", Answered: true},
		{Name: "bravo", Answered: true},
	}
}

func TestMultiSelectSuppressesRowActions(t *testing.T) {
	t.Run("k opens no kill modal in mode", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m.sessionKiller = keymapParityKiller{}
		m = enterMultiSelect(t, m)

		m = pressSession(t, m, keyK)

		if m.modal != modalNone {
			t.Errorf("k must be a no-op in multi-select mode; modal = %v, want modalNone", m.modal)
		}
		if !m.MultiSelectActive() {
			t.Errorf("k must not exit multi-select mode")
		}
	})

	t.Run("x does not switch to Projects in mode", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m = enterMultiSelect(t, m)

		m = pressSession(t, m, keyX)

		if m.activePage != PageSessions {
			t.Errorf("x must be a no-op in multi-select mode; active page = %d, want PageSessions", m.activePage)
		}
		if !m.MultiSelectActive() {
			t.Errorf("x must not exit multi-select mode")
		}
	})

	t.Run("r opens no rename modal in mode", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m.sessionRenamer = keymapParityRenamer{}
		m = enterMultiSelect(t, m)

		m = pressSession(t, m, keyR)

		if m.modal != modalNone {
			t.Errorf("r must be a no-op in multi-select mode; modal = %v, want modalNone", m.modal)
		}
		if !m.MultiSelectActive() {
			t.Errorf("r must not exit multi-select mode")
		}
	})
}

func TestMultiSelectSuppressesNewInCWD(t *testing.T) {
	m := NewModelWithSessions(twoFlatSessions())
	creator := &recordingCreator{}
	m.sessionCreator = creator
	m.cwd = "/home/user/mydir"
	m = enterMultiSelect(t, m)
	if m.SelectedSessionCount() != 1 {
		t.Fatalf("precondition: expected one marked session before n, got %d", m.SelectedSessionCount())
	}

	updated, cmd := m.updateSessionList(keyN)
	mm := updated.(Model)

	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Errorf("n in multi-select mode dispatched a command producing %T; want no command", msg)
		}
	}
	if creator.dir != "" {
		t.Errorf("n must not create a session in multi-select mode; CreateFromDir called with dir %q", creator.dir)
	}
	if got := mm.SelectedSessionCount(); got != 1 {
		t.Errorf("n must preserve the marked set; count = %d, want 1", got)
	}
	if !mm.MultiSelectActive() {
		t.Errorf("n must not exit multi-select mode")
	}
	if mm.activePage != PageSessions {
		t.Errorf("n must not leave the Sessions page; active page = %d, want PageSessions", mm.activePage)
	}
}

func TestOutOfModeNewInCWDUnchanged(t *testing.T) {
	m := NewModelWithSessions(twoFlatSessions())
	creator := &recordingCreator{}
	m.sessionCreator = creator
	m.cwd = "/home/user/mydir"

	updated, cmd := m.updateSessionList(keyN)
	mm := updated.(Model)

	if cmd == nil {
		t.Fatalf("out of mode, n must dispatch createSessionInCWD; got nil cmd")
	}
	created, ok := cmd().(SessionCreatedMsg)
	if !ok {
		t.Fatalf("out of mode, n must produce a SessionCreatedMsg")
	}
	if creator.dir != "/home/user/mydir" {
		t.Errorf("out of mode, n must create in the cwd; CreateFromDir dir = %q, want %q", creator.dir, "/home/user/mydir")
	}

	final, quitCmd := mm.Update(created)
	fm := final.(Model)
	if !isQuitCmd(quitCmd) {
		t.Errorf("out of mode, the SessionCreatedMsg must quit the picker")
	}
	if fm.selected != created.SessionName {
		t.Errorf("selected session = %q, want %q", fm.selected, created.SessionName)
	}
}

func TestMultiSelectKeepsCoexistingKeysLive(t *testing.T) {
	t.Run("Space opens the preview in mode", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m.enumerator = keymapParityEnumerator{}
		m.reader = keymapParityReader{}
		m = enterMultiSelect(t, m)

		m = pressSession(t, m, keySpace)

		if m.activePage != pagePreview {
			t.Errorf("Space must open the preview in multi-select mode; active page = %d, want pagePreview", m.activePage)
		}
	})

	t.Run("/ starts filtering in mode", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m = enterMultiSelect(t, m)

		m = pressSession(t, m, keySlash)

		if m.sessionList.FilterState() != list.Filtering {
			t.Errorf("/ must start filtering in multi-select mode; filter state = %v, want Filtering", m.sessionList.FilterState())
		}
	})

	t.Run("s cycles the grouping mode in mode", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m = enterMultiSelect(t, m)
		before := m.sessionListMode

		m = pressSession(t, m, keyS)

		if m.sessionListMode == before {
			t.Errorf("s must cycle the grouping mode in multi-select mode; mode unchanged at %v", before)
		}
		if m.activePage != PageSessions {
			t.Errorf("s must stay on Sessions (grouping cycle, not a page switch); active page = %d", m.activePage)
		}
		if !m.MultiSelectActive() {
			t.Errorf("s must not exit multi-select mode")
		}
	})
}

func TestMultiSelectFilterFocusedLiteralKeys(t *testing.T) {
	m := NewModelWithSessions(twoFlatSessions())
	m = enterMultiSelectEmpty(t, m)
	m = pressSession(t, m, keySlash)
	if !m.sessionList.SettingFilter() {
		t.Fatalf("precondition: filter input not focused after /")
	}

	beforeMode := m.sessionListMode

	m = pressSession(t, m, keyS)
	if m.sessionListMode != beforeMode {
		t.Errorf("s must be a literal filter char while filtering, not a grouping cycle; mode changed to %v", m.sessionListMode)
	}
	if got := m.sessionList.FilterValue(); got != "s" {
		t.Errorf("s must type into the filter query; FilterValue = %q, want %q", got, "s")
	}
	if !m.sessionList.SettingFilter() {
		t.Errorf("filter input must stay focused after a literal s")
	}

	m = pressSession(t, m, pressM)
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("m must be a literal filter char while filtering, not a mark toggle; count = %d, want 0", got)
	}
	if got := m.sessionList.FilterValue(); got != "sm" {
		t.Errorf("m must type into the filter query; FilterValue = %q, want %q", got, "sm")
	}
	if !m.MultiSelectActive() {
		t.Errorf("a literal m while filtering must not disturb the mode")
	}
	if !m.sessionList.SettingFilter() {
		t.Errorf("filter input must stay focused after a literal m")
	}
}

func TestMultiSelectFilterFocusedEnterEsc(t *testing.T) {
	t.Run("Enter commits-to-browse and does not open the marked set", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m = enterMultiSelect(t, m)
		m = pressSlash(t, m)
		m = typeKeys(t, m, "a")
		if m.sessionList.FilterState() != list.Filtering {
			t.Fatalf("precondition: filter state = %v, want Filtering", m.sessionList.FilterState())
		}

		updated, cmd := m.updateSessionList(keyEnter)
		mm := updated.(Model)

		if mm.sessionList.FilterState() != list.FilterApplied {
			t.Errorf("focused-filter Enter must commit-to-browse; filter state = %v, want FilterApplied", mm.sessionList.FilterState())
		}
		if isQuitCmd(cmd) {
			t.Errorf("focused-filter Enter must not fire multi-select open (no quit cmd)")
		}
		if mm.selected != "" {
			t.Errorf("focused-filter Enter must not select a session; selected = %q, want empty", mm.selected)
		}
		if !mm.MultiSelectActive() {
			t.Errorf("committing the filter must leave the mode intact")
		}
	})

	t.Run("Esc clears the filter and does not exit the mode", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m = enterMultiSelect(t, m)
		if m.SelectedSessionCount() != 1 {
			t.Fatalf("precondition: expected one marked session before filtering")
		}
		m = pressSlash(t, m)
		m = typeKeys(t, m, "a")
		if m.sessionList.FilterState() != list.Filtering {
			t.Fatalf("precondition: filter state = %v, want Filtering", m.sessionList.FilterState())
		}

		updated, cmd := m.updateSessionList(keyEsc)
		mm := updated.(Model)

		if mm.sessionList.FilterState() != list.Unfiltered {
			t.Errorf("focused-filter Esc must clear the filter; filter state = %v, want Unfiltered", mm.sessionList.FilterState())
		}
		if !mm.MultiSelectActive() {
			t.Errorf("focused-filter Esc must NOT exit multi-select mode")
		}
		if got := mm.SelectedSessionCount(); got != 1 {
			t.Errorf("focused-filter Esc must not clear the selection set; count = %d, want 1", got)
		}
		if isQuitCmd(cmd) {
			t.Errorf("focused-filter Esc must not quit")
		}
	})
}

func TestMultiSelectQuitKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"q", keyQ},
		{"Ctrl+C", keyCtrlC},
	} {
		t.Run(tc.name+" quits from within the mode", func(t *testing.T) {
			m := NewModelWithSessions(twoFlatSessions())
			m = enterMultiSelect(t, m)

			_, cmd := m.updateSessionList(tc.key)

			if !isQuitCmd(cmd) {
				t.Errorf("%s must quit from within multi-select mode", tc.name)
			}
		})
	}
}

func TestMultiSelectEnterRoutesToBurstArm(t *testing.T) {
	m := NewModelWithSessions(twoFlatSessions())
	m = enterMultiSelect(t, m)
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM)
	if m.SelectedSessionCount() != 2 {
		t.Fatalf("precondition: expected two marked sessions, got %d", m.SelectedSessionCount())
	}

	updated, cmd := m.updateSessionList(keyEnter)
	mm := updated.(Model)

	if isQuitCmd(cmd) {
		t.Errorf("Enter in multi-select mode must route to handleMultiSelectEnter, not the single-attach quit")
	}
	if mm.selected != "" {
		t.Errorf("Enter in multi-select mode must not perform a single attach; selected = %q, want empty", mm.selected)
	}
	if !mm.MultiSelectActive() {
		t.Errorf("the deferred N>=2 Enter (detection unwired) must leave the mode intact")
	}
	if got := mm.SelectedSessionCount(); got != 2 {
		t.Errorf("the deferred N>=2 Enter (detection unwired) must leave the selection intact; count = %d, want 2", got)
	}
}

func TestOutOfModeRowActionsUnchanged(t *testing.T) {
	t.Run("k opens the kill confirm modal", func(t *testing.T) {
		m := NewModelWithSessions([]Session{{Name: "alpha", Answered: true}})
		m.sessionKiller = keymapParityKiller{}

		m = pressSession(t, m, keyK)

		if m.modal != modalKillConfirm {
			t.Errorf("out of mode, k must open the kill confirm modal; modal = %v", m.modal)
		}
		if m.pendingKillName != "alpha" {
			t.Errorf("kill target = %q, want alpha", m.pendingKillName)
		}
	})

	t.Run("x switches to the Projects page", func(t *testing.T) {
		m := NewModelWithSessions([]Session{{Name: "alpha", Answered: true}})

		m = pressSession(t, m, keyX)

		if m.activePage != PageProjects {
			t.Errorf("out of mode, x must switch to Projects; active page = %d", m.activePage)
		}
	})

	t.Run("r opens the rename modal", func(t *testing.T) {
		m := NewModelWithSessions([]Session{{Name: "alpha", Answered: true}})
		m.sessionRenamer = keymapParityRenamer{}

		m = pressSession(t, m, keyR)

		if m.modal != modalRename {
			t.Errorf("out of mode, r must open the rename modal; modal = %v", m.modal)
		}
		if m.renameTarget != "alpha" {
			t.Errorf("rename target = %q, want alpha", m.renameTarget)
		}
	})
}
