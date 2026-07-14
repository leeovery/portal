package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// multi_select_keymap_test.go pins task 5.5: keymap coexistence + filter-focus
// key routing inside §5 multi-select mode. It asserts the runtime gate that
// SUPPRESSES the row-action keys (k/x/r) while in the mode, keeps the browse
// keys (Space / / / s) live, preserves the filter as an inner sub-state (s/m
// literal + Enter/Esc owned by the filter input while it is focused), keeps
// q/Ctrl+C quitting, and routes Enter to the multi-select handler in-mode. The
// suppressed arms stay PRESENT (gated, not deleted) so the default-mode
// descriptor↔dispatch probes in keymap_dispatch_guard_test.go stay green.

// keyK / keyX / keyR / keyQ / keyCtrlC / keySpace / keySlash are the concrete
// key presses this task's routing distinguishes. keyEnter / keyEsc / keyS /
// pressM are declared elsewhere in the package.
var (
	keyK     = tea.KeyPressMsg{Code: 'k', Text: "k"}
	keyX     = tea.KeyPressMsg{Code: 'x', Text: "x"}
	keyR     = tea.KeyPressMsg{Code: 'r', Text: "r"}
	keyQ     = tea.KeyPressMsg{Code: 'q', Text: "q"}
	keyCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace}
	keySlash = tea.KeyPressMsg{Code: '/', Text: "/"}
	// keyN is the create-new-session-in-cwd press. §5 multi-select suppresses it
	// so a stray n cannot fire createSessionInCWD (which creates a session, quits
	// the picker, and silently discards the marked set) mid-selection.
	keyN = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

// enterMultiSelect drives a fresh Sessions model into §5 multi-select mode via a
// single `m` press, asserting the mode actually engaged.
func enterMultiSelect(t *testing.T, m Model) Model {
	t.Helper()
	m = pressSession(t, m, pressM)
	if !m.MultiSelectActive() {
		t.Fatalf("precondition: model must be in multi-select mode after m")
	}
	return m
}

// twoFlatSessions is the deterministic two-row flat set used by the routing
// tests (both names contain "a" so a query of "a" matches at least one row).
func twoFlatSessions() []tmux.Session {
	return []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
}

// TestMultiSelectSuppressesRowActions covers the in-mode suppression: k/x/r are
// no-ops while in multi-select mode (no kill modal, no page switch, no rename
// modal) and none of them leaves the mode.
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

// TestMultiSelectSuppressesNewInCWD covers the in-mode suppression of n
// (new-session-in-cwd): n is a no-op while in multi-select mode — it dispatches no
// command, creates no session, does not quit the picker, and leaves the marked set
// and the mode intact. Without the gate, n → handleNewInCWD → createSessionInCWD
// would create a session and (via the fed-back SessionCreatedMsg) quit, silently
// discarding the whole marked set. Out-of-mode n is unchanged
// (TestOutOfModeNewInCWDUnchanged).
func TestMultiSelectSuppressesNewInCWD(t *testing.T) {
	m := NewModelWithSessions(twoFlatSessions())
	creator := &recordingCreator{}
	m.sessionCreator = creator
	m.cwd = "/home/user/mydir"
	m = enterMultiSelect(t, m)
	m = pressSession(t, m, pressM) // mark alpha (highlighted row 0)
	if m.SelectedSessionCount() != 1 {
		t.Fatalf("precondition: expected one marked session before n, got %d", m.SelectedSessionCount())
	}

	updated, cmd := m.updateSessionList(keyN)
	mm := updated.(Model)

	// n must dispatch nothing: no create cmd (which would feed back a
	// SessionCreatedMsg and quit) — running any leaked cmd surfaces the create.
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

// TestOutOfModeNewInCWDUnchanged covers the parity requirement: outside multi-select
// mode n still dispatches createSessionInCWD (creating a session in the cwd) and the
// resulting SessionCreatedMsg quits the picker — unchanged by the in-mode gate.
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

	// Feeding the SessionCreatedMsg back through Update quits the picker with the
	// created session selected.
	final, quitCmd := mm.Update(created)
	fm := final.(Model)
	if !isQuitCmd(quitCmd) {
		t.Errorf("out of mode, the SessionCreatedMsg must quit the picker")
	}
	if fm.selected != created.SessionName {
		t.Errorf("selected session = %q, want %q", fm.selected, created.SessionName)
	}
}

// TestMultiSelectKeepsCoexistingKeysLive covers the keys that STAY live in the
// mode: Space opens the preview, / starts filtering, s cycles the grouping mode.
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

// TestMultiSelectFilterFocusedLiteralKeys covers the inner filter sub-state: while
// the / input is focused, s and m are literal filter characters — they type into
// the query and do NOT regroup / toggle-mark.
func TestMultiSelectFilterFocusedLiteralKeys(t *testing.T) {
	m := NewModelWithSessions(twoFlatSessions())
	m = enterMultiSelect(t, m)
	m = pressSession(t, m, keySlash)
	if !m.sessionList.SettingFilter() {
		t.Fatalf("precondition: filter input not focused after /")
	}

	beforeMode := m.sessionListMode

	// s is a literal filter char, not the grouping cycle.
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

	// m is a literal filter char, not a multi-select toggle.
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

// TestMultiSelectFilterFocusedEnterEsc covers the filter-owns-Enter/Esc rule:
// while the / input is focused, Enter commits-to-browse (Filtering→FilterApplied)
// and Esc clears the filter — multi-select's open (Enter) and exit (Esc) do NOT
// fire.
func TestMultiSelectFilterFocusedEnterEsc(t *testing.T) {
	t.Run("Enter commits-to-browse and does not open the marked set", func(t *testing.T) {
		m := NewModelWithSessions(twoFlatSessions())
		m = enterMultiSelect(t, m)
		m = pressSlash(t, m)
		m = typeKeys(t, m, "a") // matches alpha & bravo
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
		m = pressSession(t, m, pressM) // mark alpha (highlighted row 0)
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

// TestMultiSelectQuitKeys covers the unconditional quit keys: q and Ctrl+C still
// produce a quit cmd from within multi-select mode.
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

// TestMultiSelectEnterRoutesToBurstArm covers the Enter mode-branch: with the
// filter NOT focused, Enter in multi-select mode routes to handleMultiSelectEnter
// (the §6-3 N≥2 burst arm) rather than the single-attach handleSessionListEnter.
// With host-terminal detection UNWIRED here, the burst arm defers on the
// unresolved detection, so the mode + N≥2 selection are left intact and nothing
// attaches. (A resolved-supported terminal dispatches the burst — see
// burst_dispatch_test.go.)
func TestMultiSelectEnterRoutesToBurstArm(t *testing.T) {
	m := NewModelWithSessions(twoFlatSessions())
	m = enterMultiSelect(t, m)
	m = pressSession(t, m, pressM) // mark alpha (index 0)
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM) // mark bravo (index 1)
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

// TestOutOfModeRowActionsUnchanged covers the parity requirement: outside the
// mode k/x/r behave exactly as before (kill modal / Projects page / rename modal).
func TestOutOfModeRowActionsUnchanged(t *testing.T) {
	t.Run("k opens the kill confirm modal", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
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
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})

		m = pressSession(t, m, keyX)

		if m.activePage != PageProjects {
			t.Errorf("out of mode, x must switch to Projects; active page = %d", m.activePage)
		}
	})

	t.Run("r opens the rename modal", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
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
