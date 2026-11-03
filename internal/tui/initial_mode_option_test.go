package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

type fakeLister struct{}

func (fakeLister) ListSessions() ([]Session, error) { return nil, nil }

func TestWithInitialMode(t *testing.T) {
	t.Run("sets the mode field on construction", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMode(prefs.ModeByTag))
		if m.sessionListMode != prefs.ModeByTag {
			t.Errorf("sessionListMode = %v, want ModeByTag", m.sessionListMode)
		}
	})

	t.Run("paints the mode-aware title on the first frame", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialMode(prefs.ModeByTag))
		want := sessionListTitleForMode(prefs.ModeByTag, "")
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
		sessions := []Session{{Name: "portal-abc", Answered: true, Dir: dir}}

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
