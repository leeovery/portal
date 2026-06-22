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

	projectStore  *fakeProjectStore
	projectEditor tui.ProjectEditor
	aliasEditor   tui.AliasEditor
	initialMode   prefs.SessionListMode
	// initialFlash seeds the §11.2 inline WARNING flash band on the first frame
	// (empty for every fixture that does not capture the flash). It is the only way
	// to render the otherwise-transient flash in the inert capture harness.
	initialFlash string
	// command seeds the §11.4 command-pending mode (empty for every fixture that is
	// not the command-pending capture). When non-empty, tui.Build applies WithCommand
	// so the model opens on the Projects page with the command-pending banner shown.
	command []string
}

// Deps maps the fixture onto the shared tui.Deps seam set. Every tmux seam is a
// fake: the read seams return canned data; the mutating seams are no-ops. The
// resulting model is the exact production model (built via tui.Build) — no
// bespoke render path that could drift from reality.
func (f *Fixture) Deps() tui.Deps {
	return tui.Deps{
		Lister:        f.Lister,
		Killer:        fakeKiller{},
		Renamer:       fakeRenamer{},
		Creator:       fakeCreator{},
		ProjectStore:  f.projectStore,
		ProjectEditor: f.projectEditor,
		AliasEditor:   f.aliasEditor,
		Enumerator:    fakeEnumerator{},
		Reader:        fakeScrollbackReader{},
		// DirReader/DirRunner are deliberately left nil: the fixture sessions are
		// pre-stamped (Session.Dir set), so the lazy pane-read fallback never
		// fires — and the harness has no tmux server to read panes from anyway.
		// ModePersister is nil so an `s`-toggle during a capture writes nowhere.
		InitialMode:  f.initialMode,
		InitialFlash: f.initialFlash,
		Command:      f.command,
		CWD:          "/home/user",
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
	case "sessions-by-project":
		return sessionsByProjectFixture(), nil
	case "sessions-by-tag":
		return sessionsByTagFixture(), nil
	case "sessions-paged":
		return sessionsPagedFixture(), nil
	case "sessions-inline-flash":
		return sessionsInlineFlashFixture(), nil
	case "sessions-no-tags-signpost":
		return sessionsNoTagsSignpostFixture(), nil
	case "projects":
		return projectsFixture(), nil
	case "projects-command-pending":
		return projectsCommandPendingFixture(), nil
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
	names := []string{"sessions-flat", "sessions-by-project", "sessions-by-tag", "sessions-paged", "sessions-inline-flash", "sessions-no-tags-signpost", "projects", "projects-command-pending", ContrastValidationFixture}
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

// sessionsByProjectFixture builds the deterministic "sessions-by-project"
// fixture: opened in By-Project mode (initialMode) with a set of projects whose
// paths match most session dirs, plus one session whose directory matches NO
// project so it lands in the pinned Unknown catch-all. This drives the §5.2
// By-Project grouped capture so the `— by project` mode suffix and the restyled
// group headings + nested rows (heading text.detail, `··· N` count text.dim,
// rows indented one level under their heading) are visible in the screenshot.
//
// By-Project is Pattern A — ONE row per session under its project, the key being
// the session's directory reduced to a canonical path. agentic-workflows carries
// two sessions (so a group with N>1 renders), and `orphan-explore` is stamped to
// a directory with no matching project so the Unknown catch-all heading appears in
// the same frame. The project paths match the session Dir values exactly; both
// reduce to the same canonical key (CanonicalDirKey falls back to Clean(abs) for
// these non-existent paths), so the grouping is deterministic without a real
// filesystem.
func sessionsByProjectFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true, Dir: "/home/user/code/agentic-workflows"},
		{Name: "agentic-workflows-codify", Windows: 2, Attached: false, Dir: "/home/user/code/agentic-workflows"},
		{Name: "fab-flowx-explore", Windows: 1, Attached: false, Dir: "/home/user/code/fab"},
		{Name: "evvi webhooks and watchers", Windows: 4, Attached: false, Dir: "/home/user/code/evvi"},
		{Name: "aviva-proxy-qNyfEO", Windows: 1, Attached: false, Dir: "/home/user/code/aviva"},
		{Name: "designlab-web-r8suyU", Windows: 2, Attached: false, Dir: "/home/user/code/designlab"},
		// orphan-explore's directory matches NO project → the Unknown catch-all.
		{Name: "orphan-explore", Windows: 1, Attached: false, Dir: "/home/user/code/untracked-scratch"},
	}

	projects := []project.Project{
		{Path: "/home/user/code/agentic-workflows", Name: "agentic-workflows"},
		{Path: "/home/user/code/fab", Name: "fab"},
		{Path: "/home/user/code/evvi", Name: "evvi"},
		{Path: "/home/user/code/aviva", Name: "aviva"},
		{Path: "/home/user/code/designlab", Name: "designlab"},
		// No project for /home/user/code/untracked-scratch → orphan-explore is Unknown.
	}

	return &Fixture{
		name:         "sessions-by-project",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByProject,
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

// sessionsPagedFixture builds the deterministic "sessions-paged" fixture: a fixed
// set of 100 sessions — enough to span SEVERAL pages at the 1280×800 capture size
// (the measured per-page capacity there is ≤ 31, so ~4 pages), so the §3.5
// height-driven paginator renders a multi-dot centred row (one active accent.violet
// dot + several inactive text.faint dots) the restyle task captures. It opens in
// Flat mode (the dots are a Flat-list concern; By-Tag has its own fixture).
//
// The data is FIXED (count, names, window counts, attached flags) — determinism is
// the capture gate. The session set is generated by a pure index function so the
// 100 rows are stable across machines and runs; only the first row is attached so
// the capture shows the `● attached` marker on a single deterministic row. Every
// session carries a stamped Dir so the grouped-render lazy pane-read fallback never
// fires (the harness has no tmux server); Flat ignores Dir but stamping keeps the
// fixture honest.
func sessionsPagedFixture() *Fixture {
	const count = 100
	// A small ring of project dirs so the stamped Dirs are realistic and stable
	// without introducing per-row variance that could perturb the capture.
	dirs := []string{
		"/home/user/code/agentic-workflows",
		"/home/user/code/fab",
		"/home/user/code/evvi",
		"/home/user/code/aviva",
		"/home/user/code/designlab",
		"/home/user/code/flow",
	}
	sessions := make([]tmux.Session, 0, count)
	for i := 0; i < count; i++ {
		sessions = append(sessions, tmux.Session{
			Name: fmt.Sprintf("session-%02d", i),
			// A deterministic 1–4 window-count cycle so the rows are not all identical.
			Windows:  (i % 4) + 1,
			Attached: i == 0,
			Dir:      dirs[i%len(dirs)],
		})
	}

	return &Fixture{
		name:         "sessions-paged",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
	}
}

// sessionsInlineFlashFixture builds the deterministic "sessions-inline-flash"
// fixture: a small Flat-mode session set with the §11.2 inline WARNING flash band
// seeded on the first frame, mirroring testdata/vhs/reference/sessions-inline-flash-mv.png.
// The flash band (orange ▌ left-bar + ⚠ + "folio-Jiz4el closed externally — list
// updated" on the bg.warning tint, text.on-warning message) sits directly under
// the title separator, above the `Sessions 4` section header.
//
// The flash is otherwise transient (production sets it only on the preview-bail
// path), so it is seeded via Deps.InitialFlash — the only way to render it in the
// inert harness. The session set matches the reference exactly: fab-flowx-explore
// (attached, 3 windows), agentic-workflows-codify (1), flowx-7UKPZH (2),
// aviva-proxy-qNyfEO (1). Every session carries a stamped Dir for honesty even
// though Flat ignores it (no project store, no grouping).
func sessionsInlineFlashFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 3, Attached: true, Dir: "/home/user/code/fab"},
		{Name: "agentic-workflows-codify", Windows: 1, Attached: false, Dir: "/home/user/code/agentic-workflows"},
		{Name: "flowx-7UKPZH", Windows: 2, Attached: false, Dir: "/home/user/code/flowx"},
		{Name: "aviva-proxy-qNyfEO", Windows: 1, Attached: false, Dir: "/home/user/code/aviva"},
	}

	return &Fixture{
		name:         "sessions-inline-flash",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
		initialFlash: "folio-Jiz4el closed externally — list updated",
	}
}

// sessionsNoTagsSignpostFixture builds the deterministic
// "sessions-no-tags-signpost" fixture: opened in By-Tag mode (initialMode) with
// projects that carry NO tags. Because no project anywhere carries a tag,
// anyTagsExist is false, so the By-Tag view degrades to the flat session list
// under the §11.3 persistent violet "No tags yet" info signpost (degrade with
// message, not silent flatten — §5.3) rather than grouping.
//
// The project store IS populated (the session dirs map to real project records)
// so the gate is exercised honestly — it is the ZERO-tags condition, not a
// missing project store, that drives the signpost. The session set matches the
// reference frame exactly (testdata/vhs/reference/sessions-no-tags-signpost-mv.png):
// fab-flowx-explore (attached, 3 windows), agentic-workflows-codify (1),
// flowx-7UKPZH (2), aviva-proxy-qNyfEO (4). Every session carries a stamped Dir so
// the §5.4 zero-pane-reads invariant on the signpost/flat arm holds without a tmux
// server (and the harness has none anyway). Like the other fixtures it NEVER opens
// a tmux server or touches ~/.config/portal.
func sessionsNoTagsSignpostFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 3, Attached: true, Dir: "/home/user/code/fab"},
		{Name: "agentic-workflows-codify", Windows: 1, Attached: false, Dir: "/home/user/code/agentic-workflows"},
		{Name: "flowx-7UKPZH", Windows: 2, Attached: false, Dir: "/home/user/code/flowx"},
		{Name: "aviva-proxy-qNyfEO", Windows: 4, Attached: false, Dir: "/home/user/code/aviva"},
	}

	// Projects exist and match the session dirs, but NONE carry a tag → the
	// zero-tags-anywhere gate fires and the signpost shows over the flat list.
	projects := []project.Project{
		{Path: "/home/user/code/fab", Name: "fab"},
		{Path: "/home/user/code/agentic-workflows", Name: "agentic-workflows"},
		{Path: "/home/user/code/flowx", Name: "flowx"},
		{Path: "/home/user/code/aviva", Name: "aviva"},
	}

	return &Fixture{
		name:         "sessions-no-tags-signpost",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByTag,
	}
}

// projectsFixture builds the deterministic "projects" fixture: a rich project
// store (14 projects with real-looking absolute paths, mirroring the §6 Projects
// (MV) reference frame testdata/vhs/reference/projects-mv.png) so the §6 Projects
// page reskin — the §3.1 PORTAL header, the state.green `Projects 14` section
// header, the two-line MV rows (name text.primary heavy / path text.detail dim),
// the full-height accent.violet left-bar selection over the bg.selection tint, and
// the §6.3 condensed footer — is visible in the screenshot.
//
// It opens on the Sessions page (the production default for a no-arg launch); the
// tape (testdata/vhs/projects.tape) types `x` to switch to the Projects page. The
// first project (flow-v1-api) is the cursor row in the reference, so it carries the
// full-height violet bar + selection tint in the capture. The session set is the
// sessions-flat set so the pre-`x` Sessions frame is a realistic, deterministic
// list; the capture is of the Projects page reached by the `x` key.
//
// Like the other fixtures it NEVER opens a tmux server or touches ~/.config/portal:
// the project store is the in-memory fake.
func projectsFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true, Dir: "/Users/leeovery/Code/agentic-workflows"},
		{Name: "portal", Windows: 2, Attached: false, Dir: "/Users/leeovery/Code/portal"},
		{Name: "flowx-7UKPZH", Windows: 1, Attached: false, Dir: "/Users/leeovery/Code/fabric/flowx"},
	}

	// 14 projects with real-looking absolute paths (matches the reference count).
	// flow-v1-api carries the reference Tags [Fabric, api] so the edit-project modal
	// capture (opened on it via the edit-modal tapes) renders the seeded chips.
	flowPath := "/Users/leeovery/Code/fabric/flowv1/flow-v1-api"
	projects := []project.Project{
		{Name: "flow-v1-api", Path: flowPath, Tags: []string{"Fabric", "api"}},
		{Name: "portal", Path: "/Users/leeovery/Code/portal"},
		{Name: "mint", Path: "/Users/leeovery/Code/mint"},
		{Name: "agntc", Path: "/Users/leeovery/Code/agntc"},
		{Name: "agentic-workflows", Path: "/Users/leeovery/Code/agentic-workflows"},
		{Name: "leeovery", Path: "/Users/leeovery"},
		{Name: "flowx", Path: "/Users/leeovery/Code/fabric/flowx"},
		{Name: "designlab-web", Path: "/Users/leeovery/Code/designlab/designlab-web"},
		{Name: "evvi", Path: "/Users/leeovery/Code/evvi"},
		{Name: "aviva-proxy", Path: "/Users/leeovery/Code/aviva"},
		{Name: "fab-aws-migration", Path: "/Users/leeovery/Code/fab"},
		{Name: "folio", Path: "/Users/leeovery/Code/folio"},
		{Name: "fabric", Path: "/Users/leeovery/Code/fabric"},
		{Name: "evvi-sync-engine", Path: "/Users/leeovery/Code/evvi-sync"},
	}

	return &Fixture{
		name:         "projects",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		// Wire the edit-project modal editors so the `e` key opens the modal in the
		// harness (handleEditProjectKey nil-guards when either is nil). The aliases
		// map seeds flow-v1-api's reference alias chips [fapi, v1] (keyed to the same
		// path the project record carries, so the modal's per-project alias lookup
		// matches). All mutations are in-memory no-ops.
		projectEditor: fakeProjectEditor{},
		aliasEditor: fakeAliasEditor{aliases: map[string]string{
			"fapi": flowPath,
			"v1":   flowPath,
		}},
		initialMode: prefs.ModeFlat,
	}
}

// projectsCommandPendingFixture builds the deterministic "projects-command-pending"
// fixture: the SAME rich project store as the `projects` fixture, but built with a
// pending command so m.commandPending is true — the model opens directly on the
// command-pending Projects page (WithCommand sets PageProjects), rendering the §11.4
// banner (violet `▌` left-bar + `▸ Pick a project to run` + the command in an
// accent.orange chip) over the FULL Projects chrome (green `Projects 14` header +
// `/ to filter`), with the swapped `⏎ run here · n run in cwd · esc cancel` footer.
// Mirrors testdata/vhs/reference/projects-command-pending-mv.png.
//
// The seeded command is a GENERIC build command (`npm run dev`) — Portal's
// run-a-command mechanism is tool-agnostic, so no Portal artifact references any
// specific tool. The reference frame's chip shows a different example command; only
// the chip's command text differs (generic vs the frame's example) — the banner
// structure, colours, chrome, and footer match. Like the other fixtures it NEVER
// opens a tmux server or touches ~/.config/portal: the project store is the
// in-memory fake.
func projectsCommandPendingFixture() *Fixture {
	fx := projectsFixture()
	fx.name = "projects-command-pending"
	fx.command = []string{"npm", "run", "dev"}
	return fx
}
