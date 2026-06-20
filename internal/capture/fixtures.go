package capture

import (
	"fmt"
	"sort"
	"strings"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// Fixture is a named, fully in-memory seam set for the capture harness. It
// bundles the canned read seams (lister, project store) so a test can assert
// the data, and exposes Deps() to assemble the production tui.Model via the
// shared tui.Build constructor.
type Fixture struct {
	name string

	// Lister is exported so tests can assert the deterministic session set.
	Lister *fakeLister

	projectStore *fakeProjectStore
	initialMode  prefs.SessionListMode
}

// Deps maps the fixture onto the shared tui.Deps seam set. Every tmux seam is a
// fake: the read seams return canned data; the mutating seams are no-ops. The
// resulting model is the exact production model (built via tui.Build) — no
// bespoke render path that could drift from reality.
func (f *Fixture) Deps() tui.Deps {
	return tui.Deps{
		Lister:       f.Lister,
		Killer:       fakeKiller{},
		Renamer:      fakeRenamer{},
		Creator:      fakeCreator{},
		ProjectStore: f.projectStore,
		Enumerator:   fakeEnumerator{},
		Reader:       fakeScrollbackReader{},
		// DirReader/DirRunner are deliberately left nil: the fixture sessions are
		// pre-stamped (Session.Dir set), so the lazy pane-read fallback never
		// fires — and the harness has no tmux server to read panes from anyway.
		// ModePersister is nil so an `s`-toggle during a capture writes nowhere.
		InitialMode: f.initialMode,
		CWD:         "/home/user",
	}
}

// Name returns the fixture's registered name.
func (f *Fixture) Name() string { return f.name }

// FixtureByName resolves a fixture by its registered name. An unknown name
// returns an error listing the available fixtures so a bad --fixture flag fails
// loudly with a useful message.
func FixtureByName(name string) (*Fixture, error) {
	switch name {
	case "sessions-flat":
		return sessionsFlatFixture(), nil
	case "sessions-by-tag":
		return sessionsByTagFixture(), nil
	default:
		return nil, fmt.Errorf("unknown fixture %q (available: %s)", name, strings.Join(FixtureNames(), ", "))
	}
}

// FixtureNames returns the sorted list of registered fixture names — the single
// source of truth for "what can --fixture take", used by FixtureByName's error
// and the capture tool's help text. It includes the contrast-validation swatch
// (a standalone tea.Model resolved by the capture tool, NOT a tui.Model-backed
// *Fixture) so the swatch is discoverable from the same listing.
func FixtureNames() []string {
	names := []string{"sessions-flat", "sessions-by-tag", ContrastValidationFixture}
	sort.Strings(names)
	return names
}

// sessionsFlatFixture builds the deterministic "sessions-flat" fixture: the
// fixed session set the Paper mock used (spec § 15 / tick task), in order, with
// window counts and attached flags. Determinism is load-bearing — the data is
// injected in-memory so no real config or tmux server is read.
//
// Every session carries a stamped Dir so the grouped-render lazy pane-read
// fallback never fires (the harness has no tmux server). The Flat view (the only
// view sessions-flat captures) ignores Dir entirely, but stamping it keeps the
// fixture honest for any future grouped-mode fixture that reuses this set.
func sessionsFlatFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true, Dir: "/home/user/code/agentic-workflows"},
		{Name: "agentic-workflows-codify", Windows: 2, Attached: false, Dir: "/home/user/code/agentic-workflows"},
		{Name: "fab-flowx-explore", Windows: 1, Attached: false, Dir: "/home/user/code/fab"},
		{Name: "evvi webhooks and watchers", Windows: 4, Attached: false, Dir: "/home/user/code/evvi"},
		{Name: "aviva-proxy-qNyfEO", Windows: 1, Attached: false, Dir: "/home/user/code/aviva"},
		{Name: "designlab-web-r8suyU", Windows: 2, Attached: false, Dir: "/home/user/code/designlab"},
		{Name: "evvi-sync-engine", Windows: 1, Attached: false, Dir: "/home/user/code/evvi"},
		{Name: "fab-aws-migration", Windows: 5, Attached: false, Dir: "/home/user/code/fab"},
		{Name: "flow-v1-api-XkkhTN", Windows: 1, Attached: false, Dir: "/home/user/code/flow"},
		{Name: "flowx-7UKPZH", Windows: 2, Attached: false, Dir: "/home/user/code/flowx"},
		{Name: "fabric-lk26UG", Windows: 1, Attached: false, Dir: "/home/user/code/fabric"},
		{Name: "folio-Jiz4el", Windows: 1, Attached: false, Dir: "/home/user/code/folio"},
	}

	return &Fixture{
		name:         "sessions-flat",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
	}
}

// sessionsByTagFixture builds the deterministic "sessions-by-tag" fixture: the
// same session set as sessions-flat, but opened in By-Tag mode (initialMode) with
// tagged projects whose paths match the session dirs. This drives the §5.3 By-Tag
// grouped capture so the `— by tag` mode suffix (and the restyled group headings)
// are visible in the screenshot.
//
// Tags are directory-anchored (§5.5): they live on the project record and a
// session inherits its directory's tags at grouped-render time. The project paths
// match the session Dir values exactly; both reduce to the same canonical key
// (CanonicalDirKey falls back to Clean(abs) for these non-existent paths), so the
// match is deterministic without a real filesystem. The `evvi` project is left
// UNTAGGED so its sessions collect under the pinned `Untagged` catch-all — showing
// both a tagged heading and the catch-all in one frame.
func sessionsByTagFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true, Dir: "/home/user/code/agentic-workflows"},
		{Name: "agentic-workflows-codify", Windows: 2, Attached: false, Dir: "/home/user/code/agentic-workflows"},
		{Name: "fab-flowx-explore", Windows: 1, Attached: false, Dir: "/home/user/code/fab"},
		{Name: "evvi webhooks and watchers", Windows: 4, Attached: false, Dir: "/home/user/code/evvi"},
		{Name: "aviva-proxy-qNyfEO", Windows: 1, Attached: false, Dir: "/home/user/code/aviva"},
		{Name: "designlab-web-r8suyU", Windows: 2, Attached: false, Dir: "/home/user/code/designlab"},
		{Name: "evvi-sync-engine", Windows: 1, Attached: false, Dir: "/home/user/code/evvi"},
		{Name: "fab-aws-migration", Windows: 5, Attached: false, Dir: "/home/user/code/fab"},
	}

	projects := []project.Project{
		{Path: "/home/user/code/agentic-workflows", Name: "agentic-workflows", Tags: []string{"work"}},
		{Path: "/home/user/code/fab", Name: "fab", Tags: []string{"work", "client"}},
		{Path: "/home/user/code/aviva", Name: "aviva", Tags: []string{"client"}},
		{Path: "/home/user/code/designlab", Name: "designlab", Tags: []string{"personal"}},
		// evvi is deliberately untagged → its sessions land in the Untagged catch-all.
		{Path: "/home/user/code/evvi", Name: "evvi"},
	}

	return &Fixture{
		name:         "sessions-by-tag",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByTag,
	}
}
