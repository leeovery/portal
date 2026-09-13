package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

func TestAnyTagsExist(t *testing.T) {
	cases := []struct {
		name     string
		projects []project.Project
		want     bool
	}{
		{"nil projects", nil, false},
		{"no projects carry tags", []project.Project{{Path: "/a", Name: "A"}, {Path: "/b", Name: "B"}}, false},
		{"empty tag slices", []project.Project{{Path: "/a", Name: "A", Tags: []string{}}}, false},
		{"one project carries a tag", []project.Project{{Path: "/a", Name: "A"}, {Path: "/b", Name: "B", Tags: []string{"work"}}}, true},
	}
	for _, c := range cases {
		if got := anyTagsExist(c.projects); got != c.want {
			t.Errorf("%s: anyTagsExist() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestByTagZeroTagsSignpost(t *testing.T) {
	t.Run("shows the No tags yet signpost in By Tag mode with zero tags anywhere", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.rebuildSessionList()

		if !m.byTagSignpost {
			t.Fatalf("byTagSignpost = false, want true (zero tags anywhere in By Tag mode)")
		}
		if !strings.Contains(m.View().Content, "No tags yet") {
			t.Errorf("rendered view does not contain %q:\n%s", "No tags yet", m.View().Content)
		}
	})

	t.Run("renders the plain flat session list under the signpost (no Untagged heading)", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}, {Name: "portal-def", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.rebuildSessionList()

		got := m.sessionList.Items()
		want := ToListItems(sessions)
		if len(got) != len(want) {
			t.Fatalf("len(items) = %d, want %d (plain flat slice)", len(got), len(want))
		}
		for i := range want {
			gi := asSessionItem(t, got[i])
			if gi != asSessionItem(t, want[i]) {
				t.Errorf("item %d = %+v, want flat %+v", i, gi, want[i])
			}
			if gi.GroupKey != "" || gi.GroupHeading != "" || gi.CatchAll {
				t.Errorf("item %d is grouped (key=%q heading=%q catchAll=%v), want flat",
					i, gi.GroupKey, gi.GroupHeading, gi.CatchAll)
			}
		}
		if strings.Contains(m.View().Content, untaggedHeading) {
			t.Errorf("rendered view contains %q heading, want plain flat list:\n%s", untaggedHeading, m.View().Content)
		}
	})

	t.Run("advances By Tag to Flat with one s press from the signposted state", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}
		persister := &fakeModePersister{}

		m := newSwitchViewTestModel(t, prefs.ModeByTag, persister, sessions, projects)
		if !m.byTagSignpost {
			t.Fatalf("setup invariant: byTagSignpost = false, want true (signposted By Tag state)")
		}

		updated, _ := m.Update(keyS)
		mm := updated.(Model)
		if mm.sessionListMode != prefs.ModeFlat {
			t.Errorf("sessionListMode = %v, want ModeFlat (one s advances signposted By Tag to Flat)", mm.sessionListMode)
		}
		if mm.byTagSignpost {
			t.Errorf("byTagSignpost = true after advancing to Flat, want false (cleared on rebuild)")
		}
		if strings.Contains(mm.View().Content, "No tags yet") {
			t.Errorf("signpost still rendered in Flat mode:\n%s", mm.View().Content)
		}
	})

	t.Run("shows the signpost when reopening in persisted By Tag with zero tags", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.rebuildSessionList()

		if !m.byTagSignpost {
			t.Fatalf("byTagSignpost = false on reopen-persisted By Tag with zero tags, want true")
		}
		if !strings.Contains(m.View().Content, "No tags yet") {
			t.Errorf("rendered view does not contain %q:\n%s", "No tags yet", m.View().Content)
		}
		got := m.sessionList.Items()
		want := ToListItems(sessions)
		if len(got) != len(want) {
			t.Fatalf("len(items) = %d, want %d (plain flat slice)", len(got), len(want))
		}
		for i := range want {
			if asSessionItem(t, got[i]) != asSessionItem(t, want[i]) {
				t.Errorf("item %d not flat: %+v", i, asSessionItem(t, got[i]))
			}
		}
	})

	t.Run("does not show the signpost when at least one tag exists", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.rebuildSessionList()

		if m.byTagSignpost {
			t.Errorf("byTagSignpost = true with a tag present, want false (normal By Tag grouping)")
		}
		if strings.Contains(m.View().Content, "No tags yet") {
			t.Errorf("signpost rendered when a tag exists:\n%s", m.View().Content)
		}
		want := buildByTag(sessions, project.NewIndex(projects), nil)
		if len(m.sessionList.Items()) != len(want) {
			t.Errorf("len(items) = %d, want %d (normal By Tag build)", len(m.sessionList.Items()), len(want))
		}
	})

	t.Run("does not show the signpost when tags exist but all live sessions are tagged", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.rebuildSessionList()

		if m.byTagSignpost {
			t.Errorf("byTagSignpost = true with tags present (all sessions tagged), want false")
		}
		if strings.Contains(m.View().Content, "No tags yet") {
			t.Errorf("signpost rendered when tags exist (all sessions tagged):\n%s", m.View().Content)
		}
		if strings.Contains(m.View().Content, untaggedHeading) {
			t.Errorf("Untagged heading rendered when all sessions are tagged:\n%s", m.View().Content)
		}
		for _, si := range sessionRows(m.sessionList.Items()) {
			if si.CatchAll {
				t.Errorf("found a CatchAll (Untagged) item; all sessions should be tagged")
			}
		}
		for _, h := range headerRows(m.sessionList.Items()) {
			if h.Heading == untaggedHeading {
				t.Errorf("found an Untagged header; all sessions should be tagged")
			}
		}
	})
}
