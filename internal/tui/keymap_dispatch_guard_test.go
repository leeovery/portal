package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type dispatchProbe struct {
	press  tea.KeyPressMsg
	honour func(t *testing.T) bool
}

func assertDescriptorDispatchParity(t *testing.T, page string, entries []keymapEntry, probes map[string]dispatchProbe) {
	t.Helper()

	for _, drift := range descriptorDispatchDrift(t, entries, probes) {
		t.Errorf("%s: %s", page, drift)
	}
}

func descriptorDispatchDrift(t *testing.T, entries []keymapEntry, probes map[string]dispatchProbe) []string {
	t.Helper()

	var drift []string

	descriptorKeys := map[string]bool{}
	for _, e := range entries {
		if e.RightAligned {
			continue
		}
		descriptorKeys[e.Key] = true
		probe, ok := probes[e.Key]
		if !ok {
			drift = append(drift, fmt.Sprintf("descriptor advertises key %q with no dispatch probe — either dispatch does not honour it or the guard is stale", e.Key))
			continue
		}
		if !probe.honour(t) {
			drift = append(drift, fmt.Sprintf("descriptor key %q is NOT honoured by the live dispatch (no bound effect) — descriptor↔dispatch drift", e.Key))
		}
	}

	for key := range probes {
		if !descriptorKeys[key] {
			drift = append(drift, fmt.Sprintf("dispatch binds key %q but the descriptor omits it — descriptor↔dispatch drift", key))
		}
	}
	return drift
}

func TestSessionsDescriptorDispatchParity(t *testing.T) {
	probes := map[string]dispatchProbe{
		"↑↓": {press: tea.KeyPressMsg{Code: tea.KeyDown}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			start := m.sessionList.Index()
			m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
			return m.sessionList.Index() == start+1
		}},
		"^↑/↓": {press: tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			return len(m.sessionList.KeyMap.NextPage.Keys()) > 0 &&
				len(m.sessionList.KeyMap.PrevPage.Keys()) > 0
		}},
		"⏎": {press: tea.KeyPressMsg{Code: tea.KeyEnter}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
			return isQuitCmd(cmd) && updated.(Model).selected == "alpha"
		}},
		"/": {press: tea.KeyPressMsg{Code: '/', Text: "/"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m = pressSession(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
			return m.sessionList.SettingFilter()
		}},
		"␣": {press: tea.KeyPressMsg{Code: tea.KeySpace}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.history = newPreviewHistory(keymapParityHistory{})
			m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
			return m.activePage == pagePreview
		}},
		"s": {press: tea.KeyPressMsg{Code: 's', Text: "s"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			before := m.sessionListMode
			m = pressSession(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
			return m.activePage == PageSessions && m.sessionListMode != before
		}},
		"m": {press: tea.KeyPressMsg{Code: 'm', Text: "m"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
			return m.MultiSelectActive()
		}},
		"n": {press: tea.KeyPressMsg{Code: 'n', Text: "n"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.sessionCreator = sessionsGuardCreator{}
			_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'n', Text: "n"})
			return cmd != nil
		}},
		"r": {press: tea.KeyPressMsg{Code: 'r', Text: "r"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.sessionLabeller = keymapParityLabeller{}
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
			return m.modal == modalRename
		}},
		"k": {press: tea.KeyPressMsg{Code: 'k', Text: "k"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m.sessionKiller = keymapParityKiller{}
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
			return m.modal == modalKillConfirm
		}},
		"q": {press: tea.KeyPressMsg{Code: 'q', Text: "q"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'q', Text: "q"})
			return isQuitCmd(cmd)
		}},
		"x": {press: tea.KeyPressMsg{Code: 'x', Text: "x"}, honour: func(t *testing.T) bool {
			m := sessionsGuardModel(t)
			m = pressSession(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
			return m.activePage == PageProjects
		}},
		"t": {press: tea.KeyPressMsg{Code: 't', Text: "t"}, honour: func(t *testing.T) bool {
			m := themeGuardModel(t, sessionsGuardModel(t))
			m = pressSession(t, m, tea.KeyPressMsg{Code: 't', Text: "t"})
			return m.themePanel.open
		}},
	}

	assertDescriptorDispatchParity(t, "sessions", sessionsKeymap(), probes)
}

func TestProjectsDescriptorDispatchParity(t *testing.T) {
	probes := map[string]dispatchProbe{
		"↑↓": {press: tea.KeyPressMsg{Code: tea.KeyDown}, honour: func(t *testing.T) bool {
			m := projectsNavModel(t)
			start := m.projectList.Index()
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
			return m.projectList.Index() == start+1
		}},
		"^↑/↓": {press: tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}, honour: func(t *testing.T) bool {
			m := projectsNavModel(t)
			return len(m.projectList.KeyMap.NextPage.Keys()) > 0 &&
				len(m.projectList.KeyMap.PrevPage.Keys()) > 0
		}},
		"⏎": {press: tea.KeyPressMsg{Code: tea.KeyEnter}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: tea.KeyEnter})
			return cmd != nil
		}},
		"x": {press: tea.KeyPressMsg{Code: 'x', Text: "x"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
			return m.activePage == PageSessions
		}},
		"e": {press: tea.KeyPressMsg{Code: 'e', Text: "e"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'e', Text: "e"})
			return m.modal == modalEditProject
		}},
		"/": {press: tea.KeyPressMsg{Code: '/', Text: "/"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
			return m.projectList.SettingFilter()
		}},
		"d": {press: tea.KeyPressMsg{Code: 'd', Text: "d"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
			return m.modal == modalDeleteProject
		}},
		"n": {press: tea.KeyPressMsg{Code: 'n', Text: "n"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: 'n', Text: "n"})
			return cmd != nil
		}},
		"q": {press: tea.KeyPressMsg{Code: 'q', Text: "q"}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: 'q', Text: "q"})
			return isQuitCmd(cmd)
		}},
		"esc": {press: tea.KeyPressMsg{Code: tea.KeyEscape}, honour: func(t *testing.T) bool {
			m := projectsDispatchModel(t)
			_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: tea.KeyEscape})
			return isQuitCmd(cmd)
		}},
		"t": {press: tea.KeyPressMsg{Code: 't', Text: "t"}, honour: func(t *testing.T) bool {
			m := themeGuardModel(t, projectsDispatchModel(t))
			m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 't', Text: "t"})
			return m.themePanel.open
		}},
	}

	assertDescriptorDispatchParity(t, "projects", projectsKeymap(), probes)
}

func TestHelpModalDescriptorDispatchParity(t *testing.T) {
	probes := map[string]dispatchProbe{
		helpScrollKey: {press: tea.KeyPressMsg{Code: tea.KeyDown}, honour: func(t *testing.T) bool {
			top := helpOpenAt(t, shortModalWidth, shortModalHeight)
			scrolled := pressed(t, top, tea.KeyPressMsg{Code: tea.KeyDown})
			return drawn(scrolled) != drawn(top) && drawn(pressed(t, scrolled, tea.KeyPressMsg{Code: tea.KeyUp})) == drawn(top)
		}},
		"esc": {press: tea.KeyPressMsg{Code: tea.KeyEscape}, honour: func(t *testing.T) bool {
			m := pressed(t, helpOpenAt(t, shortModalWidth, shortModalHeight), tea.KeyPressMsg{Code: tea.KeyEscape})
			return m.modal == modalNone
		}},
	}

	assertDescriptorDispatchParity(t, "help modal", helpModalKeymap(), probes)
}

func TestPreviewDescriptorDispatchParity(t *testing.T) {
	probes := map[string]dispatchProbe{
		"j/k": {press: tea.KeyPressMsg{Code: 'j', Text: "j"}, honour: func(t *testing.T) bool {
			return previewKeysScrollApart(t, tea.KeyPressMsg{Code: 'j', Text: "j"}, tea.KeyPressMsg{Code: 'k', Text: "k"})
		}},
		"^↑/↓": {press: tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}, honour: func(t *testing.T) bool {
			return previewKeysScrollApart(t, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})
		}},
		"↑↓": {press: tea.KeyPressMsg{Code: tea.KeyDown}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			for code, want := range map[rune]int{tea.KeyUp: -1, tea.KeyDown: +1} {
				handled, _, cmd := m.handlePreviewKey(tea.KeyPressMsg{Code: code})
				if !handled || cmd == nil {
					return false
				}
				if step, ok := cmd().(previewStepSessionMsg); !ok || step.delta != want {
					return false
				}
			}
			return true
		}},
		"Home/End": {press: tea.KeyPressMsg{Code: tea.KeyHome}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			homeHandled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyHome})
			endHandled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyEnd})
			return homeHandled && endHandled
		}},
		"⏎": {press: tea.KeyPressMsg{Code: tea.KeyEnter}, honour: func(t *testing.T) bool {
			m := previewGuardModel(t)
			handled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			return handled
		}},
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

func sessionsGuardModel(t *testing.T) Model {
	t.Helper()
	return NewModelWithSessions([]Session{
		{Name: "alpha"},
		{Name: "bravo"},
		{Name: "charlie"},
	})
}

func previewGuardModel(t *testing.T) previewModel {
	t.Helper()
	return newPreviewHelpModel(t, testDarkTheme(t), false)
}

// Down moves the scrollback off the top and up brings it back: the pair is
// honoured only if both halves reach the viewport.
func previewKeysScrollApart(t *testing.T, down, up tea.KeyPressMsg) bool {
	t.Helper()
	m, _ := newPreviewModelWithLines(t, 50)
	m.viewport.GotoTop()
	m, _ = m.Update(down)
	moved := m.viewport.YOffset()
	m, _ = m.Update(up)
	return moved > 0 && m.viewport.YOffset() < moved
}

func themeGuardModel(t *testing.T, m Model) Model {
	t.Helper()
	if m.colourless {
		t.Fatal("the guard seed must not be colourless — that would block t and the probe would assert a refusal")
	}
	m.themeState.source = newEntryThemeSource(t, false)
	return m
}

func TestKeymapDispatchGuard_ThemeKeyProbe(t *testing.T) {
	press := tea.KeyPressMsg{Code: 't', Text: "t"}

	t.Run("sessions", func(t *testing.T) {
		unwired := pressSession(t, sessionsGuardModel(t), press)
		if unwired.themePanel.open {
			t.Fatal("precondition: an unwired seam must leave t a silent no-op")
		}
		if wired := pressSession(t, themeGuardModel(t, sessionsGuardModel(t)), press); !wired.themePanel.open {
			t.Error("t did not open the panel against a faked theme source — the guard's probe would be vacuous")
		}
	})

	t.Run("projects", func(t *testing.T) {
		unwired, _ := pressProject(t, projectsDispatchModel(t), press)
		if unwired.themePanel.open {
			t.Fatal("precondition: an unwired seam must leave t a silent no-op")
		}
		wired, _ := pressProject(t, themeGuardModel(t, projectsDispatchModel(t)), press)
		if !wired.themePanel.open {
			t.Error("t did not open the panel against a faked theme source — the guard's probe would be vacuous")
		}
	})
}

type sessionsGuardCreator struct{}

func (sessionsGuardCreator) CreateFromDir(string, []string) (string, error) { return "new", nil }

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}
