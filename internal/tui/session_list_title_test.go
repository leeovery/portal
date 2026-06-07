package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
)

func TestSessionListTitleForMode(t *testing.T) {
	cases := []struct {
		name           string
		mode           prefs.SessionListMode
		insideTmux     bool
		currentSession string
		want           string
	}{
		{
			name: "Flat outside tmux",
			mode: prefs.ModeFlat,
			want: "Sessions",
		},
		{
			name: "By Project outside tmux",
			mode: prefs.ModeByProject,
			want: "Sessions — by project",
		},
		{
			name: "By Tag outside tmux",
			mode: prefs.ModeByTag,
			want: "Sessions — by tag",
		},
		{
			name:           "Flat inside tmux preserves current decoration",
			mode:           prefs.ModeFlat,
			insideTmux:     true,
			currentSession: "foo",
			want:           "Sessions (current: foo)",
		},
		{
			name:           "By Tag inside tmux composes mode suffix and current decoration",
			mode:           prefs.ModeByTag,
			insideTmux:     true,
			currentSession: "foo",
			want:           "Sessions — by tag (current: foo)",
		},
		{
			name:           "inside tmux with empty current session drops decoration",
			mode:           prefs.ModeByProject,
			insideTmux:     true,
			currentSession: "",
			want:           "Sessions — by project",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sessionListTitleForMode(c.mode, c.insideTmux, c.currentSession)
			if got != c.want {
				t.Errorf("sessionListTitleForMode(%v, %v, %q) = %q, want %q",
					c.mode, c.insideTmux, c.currentSession, got, c.want)
			}
		})
	}
}

func TestSessionListTitleModeAware(t *testing.T) {
	t.Run("shows Sessions for Flat mode", func(t *testing.T) {
		m := newRebuildTestModel(prefs.ModeFlat, nil, nil)
		m.rebuildSessionList()
		if got := m.SessionListTitle(); got != "Sessions" {
			t.Errorf("SessionListTitle() = %q, want %q", got, "Sessions")
		}
	})

	t.Run("shows by project title for By Project mode", func(t *testing.T) {
		m := newRebuildTestModel(prefs.ModeByProject, nil, nil)
		m.rebuildSessionList()
		if got := m.SessionListTitle(); got != "Sessions — by project" {
			t.Errorf("SessionListTitle() = %q, want %q", got, "Sessions — by project")
		}
	})

	t.Run("shows by tag title for By Tag mode", func(t *testing.T) {
		m := newRebuildTestModel(prefs.ModeByTag, nil, nil)
		m.rebuildSessionList()
		if got := m.SessionListTitle(); got != "Sessions — by tag" {
			t.Errorf("SessionListTitle() = %q, want %q", got, "Sessions — by tag")
		}
	})

	t.Run("updates the title on a mode change", func(t *testing.T) {
		persister := &fakeModePersister{}
		m := newSwitchViewTestModel(prefs.ModeFlat, persister, nil, nil)
		if got := m.SessionListTitle(); got != "Sessions" {
			t.Fatalf("pre-toggle SessionListTitle() = %q, want %q", got, "Sessions")
		}

		updated, _ := m.Update(keyS)
		if got := updated.(Model).SessionListTitle(); got != "Sessions — by project" {
			t.Errorf("post-toggle SessionListTitle() = %q, want %q", got, "Sessions — by project")
		}
	})

	t.Run("updates the title on a SessionsMsg refresh", func(t *testing.T) {
		m := newRebuildTestModel(prefs.ModeByTag, nil, nil)
		m.rebuildSessionList()

		updated, _ := m.Update(SessionsMsg{Sessions: []tmux.Session{{Name: "alpha"}}})
		if got := updated.(Model).SessionListTitle(); got != "Sessions — by tag" {
			t.Errorf("post-refresh SessionListTitle() = %q, want %q", got, "Sessions — by tag")
		}
	})

	t.Run("preserves the current-session decoration alongside the mode suffix inside tmux", func(t *testing.T) {
		m := newRebuildTestModel(prefs.ModeByTag, nil, nil)
		m.insideTmux = true
		m.currentSession = "foo"
		m.rebuildSessionList()
		if got := m.SessionListTitle(); got != "Sessions — by tag (current: foo)" {
			t.Errorf("SessionListTitle() = %q, want %q", got, "Sessions — by tag (current: foo)")
		}

		// A refresh inside tmux must keep both pieces.
		updated, _ := m.Update(SessionsMsg{Sessions: []tmux.Session{{Name: "alpha"}}})
		if got := updated.(Model).SessionListTitle(); got != "Sessions — by tag (current: foo)" {
			t.Errorf("post-refresh SessionListTitle() = %q, want %q", got, "Sessions — by tag (current: foo)")
		}
	})
}
