package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeLister is a minimal SessionLister test double so New can be exercised
// without a real tmux client. ListSessions is never invoked by construction.
type fakeLister struct{}

func (fakeLister) ListSessions() ([]tmux.Session, error) { return nil, nil }

func TestWithInitialMode(t *testing.T) {
	t.Run("sets the mode field on construction", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMode(prefs.ModeByTag))
		if m.sessionListMode != prefs.ModeByTag {
			t.Errorf("sessionListMode = %v, want ModeByTag", m.sessionListMode)
		}
	})

	t.Run("paints the mode-aware title on the first frame", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMode(prefs.ModeByTag))
		want := sessionListTitleForMode(prefs.ModeByTag, false, "")
		if got := m.sessionList.Title; got != want {
			t.Errorf("sessionList.Title = %q, want %q", got, want)
		}
	})

	t.Run("defaults to Flat when the option is omitted", func(t *testing.T) {
		m := New(fakeLister{})
		if m.sessionListMode != prefs.ModeFlat {
			t.Errorf("sessionListMode = %v, want ModeFlat", m.sessionListMode)
		}
	})

	t.Run("groups the first SessionsMsg ingestion by the injected mode", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		// Construct in By Tag, then seed projects and feed the first SessionsMsg.
		m := New(fakeLister{}, WithInitialMode(prefs.ModeByTag))
		m.setProjects(projects)
		m.applySessionListSize(80, 24)

		updated, _ := m.Update(SessionsMsg{Sessions: sessions})
		mm := updated.(Model)
		rows := sessionRows(mm.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		if got := rows[0].GroupKey; got != "work" {
			t.Errorf("GroupKey = %q, want %q (first ingestion did not group By Tag)", got, "work")
		}
	})
}
