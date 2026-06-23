// Package capture provides the in-memory fakes and deterministic fixtures used
// ONLY by the offline visual-capture harness (cmd/capturetool). It implements
// every tmux seam the TUI model depends on with canned, deterministic data so a
// capture never opens a tmux server, never spawns a daemon, and never touches
// the real ~/.config/portal.
//
// This package MUST NOT be imported by the shipped portal binary — an import
// guard test (cmd/capturetool/import_guard_test.go) enforces that the portal
// binary's transitive dependency set excludes it, keeping harness scaffolding
// out of production.
package capture

import (
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeLister is the canned SessionLister seam — it returns a fixed session set
// with no tmux server contact.
type fakeLister struct {
	sessions []tmux.Session
}

func (f *fakeLister) ListSessions() ([]tmux.Session, error) {
	// Return a copy so a consumer mutating the slice cannot perturb the fixture's
	// determinism across rebuilds within the same process.
	out := make([]tmux.Session, len(f.sessions))
	copy(out, f.sessions)
	return out, nil
}

// fakeKiller is a no-op SessionKiller — the harness must never kill a session.
type fakeKiller struct{}

func (fakeKiller) KillSession(string) error { return nil }

// fakeRenamer is a no-op SessionRenamer.
type fakeRenamer struct{}

func (fakeRenamer) RenameSession(string, string) error { return nil }

// fakeCreator is a no-op SessionCreator — it never creates a tmux session.
type fakeCreator struct{}

func (fakeCreator) CreateFromDir(string, []string) (string, error) { return "", nil }

// fakeProjectStore returns a fixed project set with no JSON file contact. It
// satisfies tui.ProjectStore. CleanStale is a no-op (returns the same set) and
// Remove is a no-op (the harness never mutates projects.json).
type fakeProjectStore struct {
	projects []project.Project
}

func (f *fakeProjectStore) List() ([]project.Project, error) {
	out := make([]project.Project, len(f.projects))
	copy(out, f.projects)
	return out, nil
}

func (f *fakeProjectStore) CleanStale() ([]project.Project, error) { return nil, nil }

func (f *fakeProjectStore) Remove(string, string) error { return nil }

// fakeProjectEditor satisfies tui.ProjectEditor for the edit-project modal capture.
// Every mutation is an in-memory no-op (the harness never touches projects.json);
// the capture only needs the modal to OPEN and render its seeded state, so the
// edit-key nil-guard (model.go handleEditProjectKey) is satisfied without any real
// persistence side effect.
type fakeProjectEditor struct{}

func (fakeProjectEditor) Rename(string, string, string) error { return nil }

func (fakeProjectEditor) AddTag(string, string) error { return nil }

func (fakeProjectEditor) RemoveTag(string, string) error { return nil }

// fakeAliasEditor satisfies tui.AliasEditor for the edit-project modal capture. It
// holds a canned alias→path map so Load() returns the seeded aliases that render as
// chips for the matching project; the mutating SetAndSave/DeleteAndSave are
// in-memory no-ops (the harness never touches the aliases file).
type fakeAliasEditor struct {
	aliases map[string]string
}

func (f fakeAliasEditor) Load() (map[string]string, error) {
	out := make(map[string]string, len(f.aliases))
	for k, v := range f.aliases {
		out[k] = v
	}
	return out, nil
}

func (fakeAliasEditor) SetAndSave(string, string, string) error { return nil }

func (fakeAliasEditor) DeleteAndSave(string, string) (bool, error) { return true, nil }

// fakeEnumerator returns canned window/pane structure for the preview page so a
// capture can drive into Preview without a tmux server. `groups`, when set,
// overrides the default multi-window shape so a fixture can pin a specific
// Window/Pane counter shape (e.g. the single 1/1 preview-screen capture).
type fakeEnumerator struct {
	groups []tmux.WindowGroup
}

func (e fakeEnumerator) ListWindowsAndPanesInSession(string) ([]tmux.WindowGroup, error) {
	if e.groups != nil {
		return e.groups, nil
	}
	return []tmux.WindowGroup{
		{WindowIndex: 1, WindowName: "editor", PaneIndices: []int{1, 2}},
		{WindowIndex: 2, WindowName: "server", PaneIndices: []int{1}},
	}, nil
}

// fakeScrollbackReader returns a fixed scrollback snippet for the preview page.
// The three-shape Tail contract is honoured: this always returns canned bytes.
// `content`, when non-empty, overrides the default snippet so a fixture can seed
// realistic scrollback (e.g. the preview-screen capture). The seeded content is
// GENERIC example terminal output — Portal's preview is tool-agnostic, so no
// fixture references any specific tool/model.
type fakeScrollbackReader struct {
	content string
}

func (r fakeScrollbackReader) Tail(string) ([]byte, error) {
	if r.content != "" {
		return []byte(r.content), nil
	}
	return []byte("$ portal open\n(canned preview scrollback)\n"), nil
}
