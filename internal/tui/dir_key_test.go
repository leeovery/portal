package tui

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

// Deps.DirKey reduces both sides of a session-to-project match: the paths the
// projects are stored at and the directories the sessions record. Paths no
// disk resolution could join group together under a key that joins them, so
// nothing reached CanonicalDirKey.
func TestBuild_MatchesSessionsToProjectsByDirKey(t *testing.T) {
	var reduced []string
	byBase := func(dir string) string {
		reduced = append(reduced, dir)
		return filepath.Base(dir)
	}
	m := Build(Deps{Lister: fakeLister{}, InitialMode: prefs.ModeByProject, DirKey: byBase})
	m.setProjects([]project.Project{{Path: "/nowhere/stored/kiln", Name: "kiln"}})
	m.applySessionListSize(80, 24)

	updated, _ := m.Update(SessionsMsg{Sessions: []Session{{Name: "kiln-a", Dir: "/nowhere/recorded/kiln"}}})
	rows := sessionRows(updated.(Model).sessionList.Items())

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if got := rows[0].GroupHeading; got != "kiln" {
		t.Errorf("GroupHeading = %q, want %q matched through DirKey", got, "kiln")
	}
	for _, dir := range []string{"/nowhere/stored/kiln", "/nowhere/recorded/kiln"} {
		if !slices.Contains(reduced, dir) {
			t.Errorf("DirKey never reduced %q; reduced %q", dir, reduced)
		}
	}
}

// Sessions grouped before any projects load are matched through DirKey too.
func TestBuild_MatchesThroughDirKeyBeforeProjectsLoad(t *testing.T) {
	var reduced []string
	m := Build(Deps{Lister: fakeLister{}, InitialMode: prefs.ModeByProject, DirKey: func(dir string) string {
		reduced = append(reduced, dir)
		return dir
	}})
	m.applySessionListSize(80, 24)

	m.Update(SessionsMsg{Sessions: []Session{{Name: "kiln-a", Dir: "/nowhere/recorded/kiln"}}})

	if !slices.Contains(reduced, "/nowhere/recorded/kiln") {
		t.Errorf("DirKey reduced %q, want the session's dir: it was matched by CanonicalDirKey", reduced)
	}
}
