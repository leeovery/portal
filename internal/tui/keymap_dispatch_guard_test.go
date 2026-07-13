package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// keymap_dispatch_guard_test.go closes the descriptor↔dispatch drift gap that
// the per-page keymap descriptors (sessionsKeymap / projectsKeymap /
// previewKeymap) leave open by construction: the descriptors are the single
// source of truth ONLY for the footer + help DISPLAY, while the live key
// DISPATCH (updateSessionList / updateProjectsPage / handlePreviewKey) is a
// separate hand-coded switch over literal strings with no compiler-enforced
// link back to the descriptor. So a descriptor and its dispatch can silently
// diverge — the descriptor can advertise a key the dispatch no longer honours,
// or the dispatch can bind a key the descriptor omits, and nothing fails to
// compile.
//
// These guards make that divergence a TEST failure. For each page they assert a
// two-way correspondence:
//
//  1. Every non-help descriptor Key has a probe that drives the page's LIVE
//     Update and asserts the dispatch HONOURS that key (produces the documented
//     bound effect, not a passthrough no-op). A descriptor Key with no probe
//     fails ("descriptor advertises a key the guard cannot tie to dispatch").
//  2. Every probed (bound) dispatch key appears in the descriptor. A probe for a
//     key absent from the descriptor fails ("dispatch binds a key the descriptor
//     omits").
//
// Removing a dispatch arm (e.g. deleting `case isRuneKey(msg, "k")` from
// updateSessionList) makes that key's effect assertion fail; adding a descriptor
// Key with no matching probe makes the descriptor-coverage assertion fail. Both
// directions bite.
//
// The `?` help self-entry is the only allow-listed exception: it is the
// RightAligned display-only footer hint whose dispatch (opening the modal /
// toggling the preview overlay) is already pinned by the dedicated
// TestSessionsKeymapRevision / TestProjectsRetainedActionParity /
// TestPreviewHelp* suites. The allow-list is the single RightAligned entry per
// page — deliberately minimal, and derived from the descriptor's own
// RightAligned flag rather than a hand-listed glyph so it cannot silently widen.

// dispatchProbe pairs a descriptor Key glyph with the live key press that should
// reach its dispatch arm and an assertion that the page's Update honoured it.
// honour returns true when the dispatch produced the bound effect; a false (or a
// passthrough no-op) is a divergence.
type dispatchProbe struct {
	// press is the concrete key event the live dispatch must honour for this
	// descriptor Key.
	press tea.KeyPressMsg
	// honour drives the page's Update with press from a fresh model and reports
	// whether the documented bound effect occurred. t is threaded so a probe can
	// build its own page-specific fixture model.
	honour func(t *testing.T) bool
}

// assertDescriptorDispatchParity is the shared two-way correspondence check: for
// the given page descriptor it (1) requires every non-help (non-RightAligned)
// entry to have a probe whose dispatch honours the key, and (2) requires every
// probe key to appear in the descriptor. The RightAligned `?` help entry is the
// sole allow-listed display-only exception (its dispatch is covered elsewhere).
func assertDescriptorDispatchParity(t *testing.T, page string, entries []keymapEntry, probes map[string]dispatchProbe) {
	t.Helper()

	// Direction 1: every non-help descriptor Key is honoured by dispatch.
	descriptorKeys := map[string]bool{}
	for _, e := range entries {
		if e.RightAligned {
			// Allow-listed: the `?` help self-entry is a display-only footer hint;
			// its dispatch is pinned by the dedicated help-modal suites.
			continue
		}
		descriptorKeys[e.Key] = true
		probe, ok := probes[e.Key]
		if !ok {
			t.Errorf("%s: descriptor advertises key %q with no dispatch probe — either dispatch does not honour it or the guard is stale", page, e.Key)
			continue
		}
		if !probe.honour(t) {
			t.Errorf("%s: descriptor key %q is NOT honoured by the live dispatch (no bound effect) — descriptor↔dispatch drift", page, e.Key)
		}
	}

	// Direction 2: every probed (bound) dispatch key appears in the descriptor.
	for key := range probes {
		if !descriptorKeys[key] {
			t.Errorf("%s: dispatch binds key %q but the descriptor omits it — descriptor↔dispatch drift", page, key)
		}
	}
}

// TestSessionsDescriptorDispatchParity guards that the sessionsKeymap descriptor
// and the live updateSessionList dispatch stay in lockstep.
func TestSessionsDescriptorDispatchParity(t *testing.T) {
	probes := map[string]dispatchProbe{
		// ↑↓ navigate — the arrows move the list cursor (dispatch delegates to
		// the list, but the binding is owned by this page's Update path).
		"↑↓": {press: tea.KeyPressMsg{Code: tea.KeyDown}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			start := m.sessionList.Index()
			m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
			return m.sessionList.Index() == start+1
		}},
		// ^↑/↓ page — paging stays bound on the session list KeyMap.
		"^↑/↓": {press: tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			return len(m.sessionList.KeyMap.NextPage.Keys()) > 0 &&
				len(m.sessionList.KeyMap.PrevPage.Keys()) > 0
		}},
		// ⏎ attach — routes to handleSessionListEnter, which selects the
		// highlighted row and quits (the attach handoff). The quit cmd is the
		// bound effect: removing the Enter arm makes this fail.
		"⏎": {press: tea.KeyPressMsg{Code: tea.KeyEnter}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
			return isQuitCmd(cmd) && updated.(Model).selected == "alpha"
		}},
		// / filter — focuses the list filter input.
		"/": {press: tea.KeyPressMsg{Code: '/', Text: "/"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m = pressSession(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
			return m.sessionList.SettingFilter()
		}},
		// ␣ preview — opens the scrollback preview page.
		"␣": {press: tea.KeyPressMsg{Code: tea.KeySpace}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.enumerator = keymapParityEnumerator{}
			m.reader = keymapParityReader{}
			m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
			return m.activePage == pagePreview
		}},
		// s switch view — advances the grouping mode (stays on Sessions).
		"s": {press: tea.KeyPressMsg{Code: 's', Text: "s"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			before := m.sessionListMode
			m = pressSession(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
			return m.activePage == PageSessions && m.sessionListMode != before
		}},
		// m multi-select — the first m enters §5 multi-select mode.
		"m": {press: tea.KeyPressMsg{Code: 'm', Text: "m"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
			return m.MultiSelectActive()
		}},
		// n new in cwd — routes to handleNewInCWD (no modal, stays on Sessions,
		// emits a createSession cmd via the wired creator).
		"n": {press: tea.KeyPressMsg{Code: 'n', Text: "n"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.sessionCreator = sessionsGuardCreator{}
			_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'n', Text: "n"})
			return cmd != nil
		}},
		// r rename — opens the rename modal.
		"r": {press: tea.KeyPressMsg{Code: 'r', Text: "r"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.sessionRenamer = keymapParityRenamer{}
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
			return m.modal == modalRename
		}},
		// k kill — opens the kill confirm modal.
		"k": {press: tea.KeyPressMsg{Code: 'k', Text: "k"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.sessionKiller = keymapParityKiller{}
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
			return m.modal == modalKillConfirm
		}},
		// q quit — produces a quit cmd.
		"q": {press: tea.KeyPressMsg{Code: 'q', Text: "q"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'q', Text: "q"})
			return isQuitCmd(cmd)
		}},
		// x projects — toggles to the Projects page.
		"x": {press: tea.KeyPressMsg{Code: 'x', Text: "x"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
			return m.activePage == PageProjects
		}},
	}

	assertDescriptorDispatchParity(t, "sessions", sessionsKeymap(), probes)
}

// TestProjectsDescriptorDispatchParity guards that the projectsKeymap descriptor
// and the live updateProjectsPage dispatch stay in lockstep.
func TestProjectsDescriptorDispatchParity(t *testing.T) {
	probes := map[string]dispatchProbe{
		// ↑↓ navigate — the arrows move the project list cursor.
		"↑↓": {press: tea.KeyPressMsg{Code: tea.KeyDown}, honour: func(t *testing.T) bool {
			m := projectsNavModel(t)
			start := m.projectList.Index()
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
			return m.projectList.Index() == start+1
		}},
		// ^↑/↓ page — paging stays bound on the project list KeyMap.
		"^↑/↓": {press: tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}, honour: func(t *testing.T) bool {
			m := projectsNavModel(t)
			return len(m.projectList.KeyMap.NextPage.Keys()) > 0 &&
				len(m.projectList.KeyMap.PrevPage.Keys()) > 0
		}},
		// ⏎ new session — routes to handleProjectEnter (emits createSession cmd).
		"⏎": {press: tea.KeyPressMsg{Code: tea.KeyEnter}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: tea.KeyEnter})
			return cmd != nil
		}},
		// x sessions — toggles to the Sessions page.
		"x": {press: tea.KeyPressMsg{Code: 'x', Text: "x"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
			return m.activePage == PageSessions
		}},
		// e edit — opens the edit project modal.
		"e": {press: tea.KeyPressMsg{Code: 'e', Text: "e"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'e', Text: "e"})
			return m.modal == modalEditProject
		}},
		// / filter — focuses the project list filter input.
		"/": {press: tea.KeyPressMsg{Code: '/', Text: "/"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
			return m.projectList.SettingFilter()
		}},
		// d delete — opens the delete project confirm modal.
		"d": {press: tea.KeyPressMsg{Code: 'd', Text: "d"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
			return m.modal == modalDeleteProject
		}},
		// n new in cwd — routes to handleNewInCWD (emits createSession cmd).
		"n": {press: tea.KeyPressMsg{Code: 'n', Text: "n"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: 'n', Text: "n"})
			return cmd != nil
		}},
		// q quit — produces a quit cmd.
		"q": {press: tea.KeyPressMsg{Code: 'q', Text: "q"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: 'q', Text: "q"})
			return isQuitCmd(cmd)
		}},
		// esc back — quits when no filter is applied (the progressive-back path).
		"esc": {press: tea.KeyPressMsg{Code: tea.KeyEscape}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: tea.KeyEscape})
			return isQuitCmd(cmd)
		}},
	}

	assertDescriptorDispatchParity(t, "projects", projectsKeymap(), probes)
}

// TestPreviewDescriptorDispatchParity guards that the previewKeymap descriptor
// and the live preview Update (handlePreviewKey + viewport delegation) stay in
// lockstep.
func TestPreviewDescriptorDispatchParity(t *testing.T) {
	probes := map[string]dispatchProbe{
		// ↑/↓ scroll — delegated to the viewport (not preview-owned): the key must
		// NOT be intercepted by handlePreviewKey, so it falls through to scroll.
		"↑/↓": {press: tea.KeyPressMsg{Code: tea.KeyDown}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			handled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyDown})
			return !handled // scroll is viewport-delegated, not preview-intercepted
		}},
		// ^↑/↓ page — likewise delegated to the viewport's page keys.
		"^↑/↓": {press: tea.KeyPressMsg{Code: tea.KeyPgDown}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			handled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
			return !handled // paging is viewport-delegated, not preview-intercepted
		}},
		// Home/End top/bottom — preview-owned jumps (handlePreviewKey binds both).
		"Home/End": {press: tea.KeyPressMsg{Code: tea.KeyHome}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			homeHandled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyHome})
			endHandled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyEnd})
			return homeHandled && endHandled
		}},
		// ←→ window — preview-owned prev/next window (intercepted before viewport).
		"←→": {press: tea.KeyPressMsg{Code: tea.KeyLeft}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			leftHandled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyLeft})
			rightHandled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyRight})
			return leftHandled && rightHandled
		}},
		// ⇥ pane — preview-owned next pane (Tab, intercepted before viewport).
		"⇥": {press: tea.KeyPressMsg{Code: tea.KeyTab}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			handled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyTab})
			return handled
		}},
		// ⏎ attach — preview-owned attach (Enter, intercepted before viewport).
		"⏎": {press: tea.KeyPressMsg{Code: tea.KeyEnter}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			handled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			return handled
		}},
		// ␣ back — preview-owned back to sessions (Space → previewDismissedMsg).
		"␣": {press: tea.KeyPressMsg{Code: tea.KeySpace}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			handled, _, cmd := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeySpace})
			if !handled || cmd == nil {
				return false
			}
			_, ok := cmd().(previewDismissedMsg)
			return ok
		}},
	}

	assertDescriptorDispatchParity(t, "preview", previewKeymap(), probes)
}

// sessionsGuardModel builds a fresh Sessions-page model seeded with multiple flat
// rows so the cursor can actually move (the nav probe needs a movable cursor).
func sessionsGuardModel(t *testing.T) Model {
	t.Helper()
	return NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	})
}

// previewGuardModel builds a single-window single-pane previewModel for driving
// handlePreviewKey directly.
func previewGuardModel(t *testing.T) previewModel {
	t.Helper()
	return newPreviewHelpModel(t, theme.Dark, false)
}

// sessionsGuardCreator is a non-nil session creator so the n new-in-cwd probe
// reaches handleNewInCWD and produces a createSession cmd.
type sessionsGuardCreator struct{}

func (sessionsGuardCreator) CreateFromDir(string, []string) (string, error) { return "new", nil }

// isQuitCmd reports whether cmd resolves to a tea.QuitMsg.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}
