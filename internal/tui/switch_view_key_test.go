package tui

import (
	"errors"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeModePersister is a test double for the ModePersister seam that records
// every Save call (value + count) and can be configured to fail.
type fakeModePersister struct {
	calls   int
	last    prefs.SessionListMode
	saveErr error
}

func (f *fakeModePersister) Save(mode prefs.SessionListMode) error {
	f.calls++
	f.last = mode
	return f.saveErr
}

// keyS is the browse-mode switch-view key.
var keyS = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}

// newSwitchViewTestModel builds a Model on the sessions page with a real
// session list, the supplied mode + persister, and the given sessions/projects.
func newSwitchViewTestModel(mode prefs.SessionListMode, persister ModePersister, sessions []tmux.Session, projects []project.Project) Model {
	m := Model{
		sessions:        sessions,
		projects:        projects,
		sessionList:     newSessionList(nil),
		projectList:     newProjectList(),
		activePage:      PageSessions,
		sessionListMode: mode,
		modePersister:   persister,
	}
	m.applySessionListSize(80, 24)
	m.rebuildSessionList()
	return m
}

func TestNextSessionListMode(t *testing.T) {
	cases := []struct {
		in   prefs.SessionListMode
		want prefs.SessionListMode
	}{
		{prefs.ModeFlat, prefs.ModeByProject},
		{prefs.ModeByProject, prefs.ModeByTag},
		{prefs.ModeByTag, prefs.ModeFlat},
		// Out-of-range value collapses defensively to Flat.
		{prefs.SessionListMode(99), prefs.ModeFlat},
	}
	for _, c := range cases {
		if got := nextSessionListMode(c.in); got != c.want {
			t.Errorf("nextSessionListMode(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSwitchViewKey(t *testing.T) {
	t.Run("cycles Flat to By Project to By Tag to Flat on successive s presses", func(t *testing.T) {
		persister := &fakeModePersister{}
		m := newSwitchViewTestModel(prefs.ModeFlat, persister, nil, nil)

		want := []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag, prefs.ModeFlat}
		var cur tea.Model = m
		for i, expected := range want {
			updated, _ := cur.Update(keyS)
			cur = updated
			if got := updated.(Model).sessionListMode; got != expected {
				t.Errorf("press %d: sessionListMode = %v, want %v", i+1, got, expected)
			}
		}
	})

	t.Run("cycles unconditionally with zero live sessions", func(t *testing.T) {
		persister := &fakeModePersister{}
		m := newSwitchViewTestModel(prefs.ModeFlat, persister, nil, nil)

		updated, _ := m.Update(keyS)
		if got := updated.(Model).sessionListMode; got != prefs.ModeByProject {
			t.Errorf("sessionListMode = %v, want ModeByProject", got)
		}
		if persister.calls != 1 {
			t.Errorf("persister.calls = %d, want 1", persister.calls)
		}
	})

	t.Run("cycles unconditionally with zero tags", func(t *testing.T) {
		dir := t.TempDir()
		// Project with no tags + one live session in it.
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}
		persister := &fakeModePersister{}

		// Start in By Project; pressing s advances into By Tag even though no
		// tags exist anywhere.
		m := newSwitchViewTestModel(prefs.ModeByProject, persister, sessions, projects)

		updated, _ := m.Update(keyS)
		if got := updated.(Model).sessionListMode; got != prefs.ModeByTag {
			t.Errorf("sessionListMode = %v, want ModeByTag (cycle must include By Tag with zero tags)", got)
		}
	})

	t.Run("persists the new mode exactly once per s press", func(t *testing.T) {
		persister := &fakeModePersister{}
		m := newSwitchViewTestModel(prefs.ModeFlat, persister, nil, nil)

		updated, _ := m.Update(keyS)
		if persister.calls != 1 {
			t.Fatalf("persister.calls = %d after one press, want 1", persister.calls)
		}
		if persister.last != prefs.ModeByProject {
			t.Errorf("persister.last = %v, want ModeByProject", persister.last)
		}

		updated2, _ := updated.(Model).Update(keyS)
		_ = updated2
		if persister.calls != 2 {
			t.Errorf("persister.calls = %d after two presses, want 2", persister.calls)
		}
		if persister.last != prefs.ModeByTag {
			t.Errorf("persister.last = %v, want ModeByTag", persister.last)
		}
	})

	t.Run("does not persist on a SessionsMsg refresh", func(t *testing.T) {
		persister := &fakeModePersister{}
		m := newSwitchViewTestModel(prefs.ModeByProject, persister, nil, nil)

		m.Update(SessionsMsg{Sessions: nil})
		if persister.calls != 0 {
			t.Errorf("persister.calls = %d after SessionsMsg, want 0 (persist only on s press)", persister.calls)
		}
	})

	t.Run("treats s as a literal filter character while the filter input is focused", func(t *testing.T) {
		persister := &fakeModePersister{}
		m := newSwitchViewTestModel(prefs.ModeFlat, persister, nil, nil)
		// Drive the list into the actively-filtering state so SettingFilter() is true.
		m.sessionList.SetFilterState(list.Filtering)

		updated, _ := m.Update(keyS)
		if got := updated.(Model).sessionListMode; got != prefs.ModeFlat {
			t.Errorf("sessionListMode = %v, want ModeFlat (s must not cycle while filtering)", got)
		}
		if persister.calls != 0 {
			t.Errorf("persister.calls = %d, want 0 (s is a literal filter char while filtering)", persister.calls)
		}
	})

	t.Run("advances the mode even when persistence fails", func(t *testing.T) {
		persister := &fakeModePersister{saveErr: errors.New("disk full")}
		m := newSwitchViewTestModel(prefs.ModeFlat, persister, nil, nil)

		updated, _ := m.Update(keyS)
		if got := updated.(Model).sessionListMode; got != prefs.ModeByProject {
			t.Errorf("sessionListMode = %v, want ModeByProject (persist failure must not abort toggle)", got)
		}
		if persister.calls != 1 {
			t.Errorf("persister.calls = %d, want 1 (Save still attempted on failure path)", persister.calls)
		}
	})

	t.Run("tolerates a nil persister", func(t *testing.T) {
		m := newSwitchViewTestModel(prefs.ModeFlat, nil, nil, nil)

		updated, _ := m.Update(keyS)
		if got := updated.(Model).sessionListMode; got != prefs.ModeByProject {
			t.Errorf("sessionListMode = %v, want ModeByProject (nil persister must not panic)", got)
		}
	})

	t.Run("re-renders the list into the new mode on press", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}
		persister := &fakeModePersister{}

		// Flat → By Project: after the press, items carry the project heading.
		m := newSwitchViewTestModel(prefs.ModeFlat, persister, sessions, projects)

		updated, _ := m.Update(keyS)
		mm := updated.(Model)
		items := mm.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		if got := asSessionItem(t, items[0]).GroupHeading; got != "Portal" {
			t.Errorf("GroupHeading = %q, want %q (list did not re-render into By Project)", got, "Portal")
		}
	})
}
