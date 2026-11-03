package capture

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// Placeholder cause — only the UserMessage renders, so it is never classified.
var errFixtureFatal = errors.New("permission denied")

type Fixture struct {
	name string
	// The render size the fixture requires, per dimension, where its frame IS a
	// geometry rather than content at whatever size the caller has. Zero takes
	// the caller's.
	width, height int
	// The terminal its golden frame is kept at, per dimension, where the screen
	// is designed for one other than the harness's. Zero takes harnessSize's.
	frame Size

	Lister *fakeLister
	// Session names, never indicator text: the delegate draws the indicator.
	pendingSessions []string

	projectStore  *fakeProjectStore
	projectEditor tui.ProjectEditor
	aliasEditor   tui.AliasEditor
	initialMode   prefs.SessionListMode
	// search non-nil opens the picker as a session search did, with the term
	// committed as the list's filter.
	search             *tui.SearchForm
	initialMultiSelect []string
	initialCursor      string
	// Independent of the `--theme` palette: the union is what the panel lists —
	// assembled from the enumeration and these keys — and the keys are what it
	// marks.
	themeKeys        theme.RawKeys
	themeUnion       theme.Union
	themeEnumeration theme.Enumeration
	themeSlots       []theme.SlotResolution
	// Placement only — applies no theme. Without it a cursor-off-the-marked-row
	// frame is unreachable in a one-shot render.
	initialThemeCursor string
	// State, never text: the message slot composes its own copy, so no fixture
	// can put a paraphrase on a captured frame.
	initialThemeConfirm      bool
	initialThemeCommitFailed bool
	initialDetection         *spawn.Identity
	initialGoneFlagged       []string
	// The in-burst Opening band as (done, total).
	initialBurstOpening [2]int
	initialFlash        string
	command             []string
	history             string
	historyFile         string
	serverStarted       bool
	loadingEvents       []tui.BootstrapProgressMsg
	fatalEvent          tui.BootstrapFatalMsg
	// Tapes are scaffolding and do not live in the repository, so the post-load
	// key script is declared here.
	captureKeys []tea.KeyPressMsg
	// landsLastKeyRead runs the last capture key's command and delivers what it
	// returns, as a running program would: the preview's body arrives that way.
	// It delivers only that command's own message, not any it leads to.
	landsLastKeyRead bool
	noColor          bool
}

// Deps hands the palette to two seams that must agree — the constant nomination
// and the faked ThemeSource — or the panel's open repaints the frame off
// `--theme`.
func (f *Fixture) Deps(th theme.Theme) tui.Deps {
	return tui.Deps{
		Theme:         theme.ConstantNomination(th),
		ThemeKeys:     f.themeKeys,
		ThemeSource:   f.themeSource(th),
		Lister:        f.Lister,
		PendingReader: f.pendingReader(),
		Killer:        fakeKiller{},
		Labeller:      fakeLabeller{},
		Creator:       fakeCreator{},
		ProjectStore:  f.projectStore,
		ProjectEditor: f.projectEditor,
		AliasEditor:   f.aliasEditor,
		History:       fakeHistory{content: f.history, file: f.historyFile},
		// DirRunner stays nil, so the git-root fallback resolves nothing and a
		// fixture session carrying no recorded directory groups into the
		// catch-all; ModePersister stays nil so an `s`-toggle writes nowhere.
		InitialMode:   f.initialMode,
		Search:        f.search,
		Command:       f.command,
		CWD:           fixtureHome,
		Home:          fixtureHome,
		ServerStarted: f.serverStarted,
		NoColor:       f.noColor,
		Capture: tui.CaptureSeeds{
			ThemeCursor:  f.initialThemeCursor,
			ThemeConfirm: f.initialThemeConfirm,
			// The panel's message slot is a single-slot arbiter: a fixture declares
			// at most one of ThemeConfirm / ThemeCommitFailed.
			ThemeCommitFailed: f.initialThemeCommitFailed,
			MultiSelect:       f.initialMultiSelect,
			Cursor:            f.initialCursor,
			Detection:         f.initialDetection,
			GoneFlagged:       f.initialGoneFlagged,
			BurstOpening:      f.initialBurstOpening,
			Flash:             f.initialFlash,
		},
		ProgressReceiver: loadingReceiverOrNil(f.loadingEvents, f.fatalEvent),
	}
}

// Nil when the fixture declares no panel — a nil source makes `t` a silent
// no-op rather than opening a zero-row slide-over.
func (f *Fixture) themeSource(th theme.Theme) tui.ThemeSource {
	if len(f.themeUnion.Rows) == 0 {
		return nil
	}
	return newFakeThemeSource(th, f.themeEnumeration, f.themeUnion, f.themeSlots)
}

// Nil rather than an empty reader: Build wires any non-nil one.
func (f *Fixture) pendingReader() tui.PendingResumeReader {
	if len(f.pendingSessions) == 0 {
		return nil
	}
	return fakePendingReader{sessions: f.pendingSessions}
}

func loadingReceiverOrNil(events []tui.BootstrapProgressMsg, fatal tui.BootstrapFatalMsg) tea.Cmd {
	if fatal.FailedStep > 0 {
		return loadingFatalReceiver(events, fatal)
	}
	if len(events) == 0 {
		return nil
	}
	return loadingProgressReceiver(events)
}

func (f *Fixture) Name() string { return f.name }

// fixtureBuilders is the registry: both lookups derive from it, and each builder
// names its own fixture, so a builder's name cannot be reachable by one lookup
// and absent from the other.
func fixtureBuilders() []func() *Fixture {
	return []func() *Fixture{
		sessionsFlatFixture,
		sessionsEmptyFixture,
		sessionsByProjectFixture,
		sessionsByProjectLabelledFixture,
		sessionsByTagFixture,
		sessionsPagedFixture,
		sessionsInlineFlashFixture,
		sessionsRenameRefusedLabelFixture,
		sessionsMultiSelectActiveFixture,
		sessionsUnsupportedTerminalFixture,
		sessionsUnsupportedNullFixture,
		sessionsMultiSelectPreflightAbortFixture,
		sessionsBurstOpeningFixture,
		sessionsNoTagsSignpostFixture,
		sessionsSearchResultsFixture,
		sessionsSearchHomeFixture,
		sessionsPendingResumeFixture,
		sessionsPendingResumeColourlessFixture,
		sessionsNotAnsweringFixture,
		projectsFixture,
		projectsCommandPendingFixture,
		themePanelAdaptivePairFixture,
		themePanelConstantPreviewingFixture,
		themePanelInvalidRowFixture,
		themePanelDirUnreadableFixture,
		themePanelNarrowFixture,
		themePanelPaginatedFixture,
		themePanelProjectsFixture,
		themePanelConfirmFixture,
		themePanelCommitFailedFixture,
		themePanelMinHeightMessageFixture,
		previewScreenFixture,
		previewLongScrollbackFixture,
		previewZmxHistoryFixture,
		loadingScreenFixture,
		loadingErrorFixture,
	}
}

func FixtureByName(name string) (*Fixture, error) {
	for _, build := range fixtureBuilders() {
		if fx := build(); fx.Name() == name {
			return fx, nil
		}
	}
	return nil, unknownFixture(name)
}

func unknownFixture(name string) error {
	return fmt.Errorf("unknown fixture %q (available: %s)", name, strings.Join(FixtureNames(), ", "))
}

// FixtureNames includes the standalone names too — the contrast-validation
// swatch and the resume surfaces — which are tea.Models of their own rather
// than a *Fixture. StandaloneNames() is the set FixtureByName does not resolve.
func FixtureNames() []string {
	builders := fixtureBuilders()
	standalone := StandaloneNames()
	names := make([]string, 0, len(builders)+len(standalone))
	for _, build := range builders {
		names = append(names, build().Name())
	}
	names = append(names, standalone...)
	slices.Sort(names)
	return names
}

// Every session carries a recorded Dir, so grouping never needs the git-root
// fallback the harness leaves unwired.
func sessionsFlatFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "orchard-pipeline-code-based", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "orchard-pipeline-codify", Attached: false, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "kiln-relayx-explore", Attached: false, Dir: "/home/user/code/kiln"},
		{Name: "lumen webhooks and watchers", Attached: false, Dir: "/home/user/code/lumen"},
		{Name: "tessera-proxy-Vb3kQw", Attached: false, Dir: "/home/user/code/tessera"},
		{Name: "atelier-web-m4TzpD", Attached: false, Dir: "/home/user/code/atelier"},
		{Name: "lumen-sync-engine", Attached: false, Dir: "/home/user/code/lumen"},
		{Name: "kiln-aws-migration", Attached: false, Dir: "/home/user/code/kiln"},
		{Name: "relay-v1-api-Hq7wNe", Attached: false, Dir: "/home/user/code/relay"},
		{Name: "relayx-3RJDWA", Attached: false, Dir: "/home/user/code/relayx"},
		{Name: "loom-p84cXa", Attached: false, Dir: "/home/user/code/loom"},
		{Name: "quill-Ue9fLr", Attached: false, Dir: "/home/user/code/quill"},
	}

	return &Fixture{
		name:         "sessions-flat",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
	}
}

func sessionsEmptyFixture() *Fixture {
	return &Fixture{
		name:         "sessions-empty",
		Lister:       &fakeLister{sessions: nil},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
		// With zero sessions the default-page rule routes to Projects; `x` lands
		// the model on the empty Sessions screen this fixture captures.
		captureKeys: []tea.KeyPressMsg{keyRune('x')},
	}
}

// The paths do not exist; CanonicalDirKey falls back to Clean(abs) on both
// sides, so the grouping is deterministic without a real filesystem.
func sessionsByProjectFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "orchard-pipeline-code-based", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "orchard-pipeline-codify", Attached: false, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "kiln-relayx-explore", Attached: false, Dir: "/home/user/code/kiln"},
		{Name: "lumen webhooks and watchers", Attached: false, Dir: "/home/user/code/lumen"},
		{Name: "tessera-proxy-Vb3kQw", Attached: false, Dir: "/home/user/code/tessera"},
		{Name: "atelier-web-m4TzpD", Attached: false, Dir: "/home/user/code/atelier"},
		// Matches no project → lands in the Unknown catch-all.
		{Name: "orphan-explore", Attached: false, Dir: "/home/user/code/untracked-scratch"},
	}

	projects := []project.Project{
		{Path: "/home/user/code/orchard-pipeline", Name: "orchard-pipeline"},
		{Path: "/home/user/code/kiln", Name: "kiln"},
		{Path: "/home/user/code/lumen", Name: "lumen"},
		{Path: "/home/user/code/tessera", Name: "tessera"},
		{Path: "/home/user/code/atelier", Name: "atelier"},
	}

	return &Fixture{
		name:         "sessions-by-project",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByProject,
	}
}

// Labelled rows in a grouped mode: each group, the catch-all included, orders
// by the text its rows show, so a label sits where it reads and two rows
// showing the same text fall back to their names.
func sessionsByProjectLabelledFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "orchard-pipeline-Vb3kQw", Label: "release checklist", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "orchard-pipeline-codify", Dir: "/home/user/code/orchard-pipeline"},
		{Name: "orchard-pipeline-Hq7wNe", Label: "api · user search", Dir: "/home/user/code/orchard-pipeline"},
		{Name: "kiln-m4TzpD", Label: "spike", Dir: "/home/user/code/kiln"},
		{Name: "kiln-3RJDWA", Label: "spike", Dir: "/home/user/code/kiln"},
		{Name: "kiln-aws-migration", Dir: "/home/user/code/kiln"},
		{Name: "scratch-p84cXa", Label: "notes on the outage", Dir: "/home/user/code/untracked-scratch"},
		{Name: "scratch-Ue9fLr", Dir: "/home/user/code/untracked-scratch"},
	}

	projects := []project.Project{
		{Path: "/home/user/code/orchard-pipeline", Name: "orchard-pipeline"},
		{Path: "/home/user/code/kiln", Name: "kiln"},
	}

	return &Fixture{
		name:         "sessions-by-project-labelled",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByProject,
	}
}

// The paths do not exist; CanonicalDirKey falls back to Clean(abs) on both
// sides, so the match is deterministic without a real filesystem.
func sessionsByTagFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "orchard-pipeline-code-based", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "orchard-pipeline-codify", Attached: false, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "kiln-relayx-explore", Attached: false, Dir: "/home/user/code/kiln"},
		{Name: "lumen webhooks and watchers", Attached: false, Dir: "/home/user/code/lumen"},
		{Name: "tessera-proxy-Vb3kQw", Attached: false, Dir: "/home/user/code/tessera"},
		{Name: "atelier-web-m4TzpD", Attached: false, Dir: "/home/user/code/atelier"},
		{Name: "lumen-sync-engine", Attached: false, Dir: "/home/user/code/lumen"},
		{Name: "kiln-aws-migration", Attached: false, Dir: "/home/user/code/kiln"},
	}

	projects := []project.Project{
		{Path: "/home/user/code/orchard-pipeline", Name: "orchard-pipeline", Tags: []string{"work"}},
		{Path: "/home/user/code/kiln", Name: "kiln", Tags: []string{"work", "client"}},
		{Path: "/home/user/code/tessera", Name: "tessera", Tags: []string{"client"}},
		{Path: "/home/user/code/atelier", Name: "atelier", Tags: []string{"personal"}},
		// Deliberately untagged → its sessions land in the Untagged catch-all,
		// so a tagged heading and the catch-all share one frame.
		{Path: "/home/user/code/lumen", Name: "lumen"},
	}

	return &Fixture{
		name:         "sessions-by-tag",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByTag,
	}
}

// Enough sessions to span several pages at the capture size, so the
// height-driven paginator renders its multi-dot row.
func sessionsPagedFixture() *Fixture {
	const count = 100
	dirs := []string{
		"/home/user/code/orchard-pipeline",
		"/home/user/code/kiln",
		"/home/user/code/lumen",
		"/home/user/code/tessera",
		"/home/user/code/atelier",
		"/home/user/code/relay",
	}
	sessions := make([]tui.Session, 0, count)
	for i := range count {
		sessions = append(sessions, tui.Session{
			Name:     fmt.Sprintf("session-%02d", i),
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

func sessionsInlineFlashFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "kiln-relayx-explore", Attached: true, Dir: "/home/user/code/kiln"},
		{Name: "orchard-pipeline-codify", Attached: false, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "relayx-3RJDWA", Attached: false, Dir: "/home/user/code/relayx"},
		{Name: "tessera-proxy-Vb3kQw", Attached: false, Dir: "/home/user/code/tessera"},
	}

	return &Fixture{
		name:         "sessions-inline-flash",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
		initialFlash: "quill-Ue9fLr closed externally — list updated",
	}
}

// The refusal is typed, never seeded: the keys open the real rename modal and
// add a zero-width space, a format character the text input keeps, so the band
// carries the index's own refusal rather than a copy this file could drift from.
func sessionsRenameRefusedLabelFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-rename-refused-label"
	fx.captureKeys = []tea.KeyPressMsg{keyRune('r'), keyRune('\u200b'), keyEnter()}
	fx.landsLastKeyRead = true
	return fx
}

func sessionsMultiSelectActiveFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-multi-select-active"
	fx.initialMultiSelect = []string{
		"orchard-pipeline-codify",
		"kiln-relayx-explore",
		"atelier-web-m4TzpD",
	}
	fx.initialCursor = "kiln-relayx-explore"
	return fx
}

// Apple Terminal is a recognised-but-undriven identity that resolves
// unsupported, so the proactive banner replaces the section header.
func sessionsUnsupportedTerminalFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-unsupported-terminal"
	fx.initialDetection = &spawn.Identity{Name: "Apple Terminal", BundleID: "com.apple.Terminal"}
	return fx
}

// The banner is named-only, so the null identity renders the standard header —
// visually identical to sessions-flat by design.
func sessionsUnsupportedNullFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-unsupported-null"
	fx.initialDetection = &spawn.Identity{}
	return fx
}

func sessionsMultiSelectPreflightAbortFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-multi-select-preflight-abort"
	fx.initialMultiSelect = []string{
		"orchard-pipeline-codify",
		"kiln-relayx-explore",
		"atelier-web-m4TzpD",
	}
	fx.initialCursor = "kiln-relayx-explore"
	fx.initialGoneFlagged = []string{"kiln-relayx-explore"}
	return fx
}

func sessionsBurstOpeningFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-burst-opening"
	fx.initialMultiSelect = []string{
		"orchard-pipeline-codify",
		"kiln-relayx-explore",
		"atelier-web-m4TzpD",
	}
	fx.initialBurstOpening = [2]int{2, 3}
	return fx
}

// The project store is populated deliberately: the zero-tags condition, not a
// missing store, must drive the signpost.
func sessionsNoTagsSignpostFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "kiln-relayx-explore", Attached: true, Dir: "/home/user/code/kiln"},
		{Name: "orchard-pipeline-codify", Attached: false, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "relayx-3RJDWA", Attached: false, Dir: "/home/user/code/relayx"},
		{Name: "tessera-proxy-Vb3kQw", Attached: false, Dir: "/home/user/code/tessera"},
	}

	projects := []project.Project{
		{Path: "/home/user/code/kiln", Name: "kiln"},
		{Path: "/home/user/code/orchard-pipeline", Name: "orchard-pipeline"},
		{Path: "/home/user/code/relayx", Name: "relayx"},
		{Path: "/home/user/code/tessera", Name: "tessera"},
	}

	return &Fixture{
		name:         "sessions-no-tags-signpost",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByTag,
	}
}

// fixtureHome is the home directory every fixture's model abbreviates against,
// handed to it through Deps, so no frame reads the process's $HOME.
const fixtureHome = "/home/user"

// One frame carrying every branch of the search-opened directory column: a home
// path shown abbreviated, a row matched on its directory alone, a row whose
// recorded directory is empty, a path deep enough to be left-truncated at the
// widths these captures are taken at, a path outside home that stays absolute,
// and a session matching neither field so the narrowing is visible by its
// absence.
//
// legacy-port-shim carries no Dir, against this file's recorded-Dir convention:
// the git-root fallback is unwired, so a grouped rebuild resolves nothing and
// routes it to the catch-all.
func sessionsSearchResultsFixture() *Fixture {
	home := fixtureHome
	portalDir := filepath.Join(home, "code/portal")
	gatewayDir := filepath.Join(home, "code/portal-gateway")
	lumenDir := filepath.Join(home, "code/lumen")

	sessions := []tui.Session{
		{Name: "portal-a1b2", Attached: true, Dir: portalDir},
		{Name: "api-work", Attached: false, Dir: gatewayDir},
		{Name: "legacy-port-shim", Attached: false},
		{Name: "portal-design-exports-review", Attached: false, Dir: filepath.Join(portalDir, "internal/capture/testdata/reference/design-exports/frames/archive/2026-q3")},
		{Name: "portal-notes", Attached: false, Dir: "/opt/portal-tools"},
		{Name: "lumen-sync-engine", Attached: false, Dir: lumenDir},
	}

	projects := []project.Project{
		{Path: portalDir, Name: "portal", Tags: []string{"work"}},
		{Path: gatewayDir, Name: "portal-gateway", Tags: []string{"work", "client"}},
		{Path: "/opt/portal-tools", Name: "portal-tools", Tags: []string{"tools"}},
		{Path: lumenDir, Name: "lumen"},
	}

	return &Fixture{
		name:         "sessions-search-results",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeFlat,
		search:       &tui.SearchForm{Term: "port"},
	}
}

// The same sessions searched by a term only the home abbreviation can match:
// every row present is there because its directory folded to ~/code under the
// fixture's pinned home, which the process's $HOME never is in a test.
func sessionsSearchHomeFixture() *Fixture {
	fx := sessionsSearchResultsFixture()
	fx.name = "sessions-search-home"
	fx.search = &tui.SearchForm{Term: "~/code"}
	return fx
}

// One row per attached × pending combination, so the indicators' packing is
// seen in one frame rather than inferred from rows showing one each.
func sessionsPendingResumeFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "orchard-pipeline-codify", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "lumen-sync-engine", Attached: false, Dir: "/home/user/code/lumen"},
		{Name: "loom-p84cXa", Attached: false, Dir: "/home/user/code/loom"},
		{Name: "quill-Ue9fLr", Attached: true, Dir: "/home/user/code/quill"},
	}

	return &Fixture{
		name:            "sessions-pending-resume",
		Lister:          &fakeLister{sessions: sessions},
		pendingSessions: []string{"lumen-sync-engine", "quill-Ue9fLr"},
		projectStore:    &fakeProjectStore{projects: nil},
		initialMode:     prefs.ModeFlat,
	}
}

// Sessions that did not answer, each known by its name alone, beside ones that
// did — one under the cursor, so both the selected and the canvas dimming show.
func sessionsNotAnsweringFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "kiln-relayx-explore", NotAnswering: true},
		{Name: "orchard-pipeline-codify", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "lumen-sync-engine", Dir: "/home/user/code/lumen"},
		{Name: "quill-Ue9fLr", NotAnswering: true},
		{Name: "loom-p84cXa", Dir: "/home/user/code/loom"},
	}

	return &Fixture{
		name:         "sessions-not-answering",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
	}
}

func sessionsPendingResumeColourlessFixture() *Fixture {
	fx := sessionsPendingResumeFixture()
	fx.name = "sessions-pending-resume-colourless"
	fx.noColor = true
	return fx
}

// Built-in and drop-in rows are deliberately indistinguishable, so one valid
// drop-in must stand beside the built-ins; it sorts first.
const themePanelDropInSlug = "catppuccin-latte"

// A fixture declares its inputs and production assembles the list, so
// membership, dedup and display order have one implementation. Reassemble, never
// Open: the harness reads no directory and emits no `theme: enumerated`.
func themePanelUnionFrom(e theme.Enumeration, keys theme.RawKeys) theme.Union {
	return theme.Assembler{Loader: theme.NewSilentLoader()}.Reassemble(e, keys)
}

func themePanelEnumeration() theme.Enumeration {
	return themePanelDirEnumeration(themePanelDirEntry(themePanelDropInSlug+theme.FileExtension, themePanelDropInSlug))
}

// Capture with `--theme nord` — the row under the cursor at open (capturetool
// runs no gate, so the dark slot's theme paints). An incoherent pairing
// renders the wrong colours with nothing on the frame to say so.
func themePanelAdaptivePairFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "theme-panel-adaptive-pair"
	fx.themeKeys = theme.RawKeys{Light: theme.DefaultLightSlug, Dark: "nord"}
	fx.themeEnumeration = themePanelEnumeration()
	fx.themeUnion = themePanelUnionFrom(fx.themeEnumeration, fx.themeKeys)
	fx.themeSlots = []theme.SlotResolution{
		{Slot: theme.SlotLight, Requested: theme.DefaultLightSlug, Resolved: theme.DefaultLightSlug},
		{Slot: theme.SlotDark, Requested: "nord", Resolved: "nord"},
	}
	// The open already anchors this row; declared so the coherent `--theme`
	// partner is stated in one place.
	fx.initialThemeCursor = "nord"
	fx.captureKeys = []tea.KeyPressMsg{keyRune('t')}
	return fx
}

// Capture with `--theme tokyo-night-day` — the previewed row under the seeded
// cursor, deliberately not the marked constant: the cursor's row is what is
// painted behind the panel.
func themePanelConstantPreviewingFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "theme-panel-constant-previewing"
	fx.themeKeys = theme.RawKeys{Theme: "nord"}
	fx.themeEnumeration = themePanelEnumeration()
	fx.themeUnion = themePanelUnionFrom(fx.themeEnumeration, fx.themeKeys)
	// Exactly one record with SlotConstant — a second record would render the
	// pair's badges instead of the bare marker.
	fx.themeSlots = []theme.SlotResolution{
		{Slot: theme.SlotConstant, Requested: "nord", Resolved: "nord"},
	}
	fx.initialThemeCursor = theme.DefaultLightSlug
	fx.captureKeys = []tea.KeyPressMsg{keyRune('t')}
	return fx
}

// Capture at the panel's minimum width (the message slot may wrap there) with
// `--theme tokyo-night-day`, the row under the seeded cursor. The fixture
// declares no render size, only its frame size, so live that width comes from
// the capturing terminal: at a wide one it renders the confirm on a single row.
func themePanelConfirmFixture() *Fixture {
	fx := themePanelConstantPreviewingFixture()
	fx.name = "theme-panel-confirm"
	fx.initialThemeConfirm = true
	fx.frame.Width = tui.ThemePanelMinWidthTerminal()
	return fx
}

// Capture with `--theme nord`, the row the open anchors the cursor to.
func themePanelCommitFailedFixture() *Fixture {
	fx := themePanelAdaptivePairFixture()
	fx.name = "theme-panel-commit-failed"
	fx.initialThemeCommitFailed = true
	return fx
}

// Declares the terminal that lands exactly on the panel's height floor, at the
// minimum width: what it exercises is the floor's arithmetic — one list row and
// one message row beneath the header, with the standing footer intact. Capture
// with `--theme nord`.
func themePanelMinHeightMessageFixture() *Fixture {
	fx := themePanelCommitFailedFixture()
	fx.name = "theme-panel-min-height-message"
	fx.width = tui.ThemePanelMinWidthTerminal()
	fx.height = tui.ThemePanelFloorTerminalHeight()
	return fx
}

// Never resolved, opened or stat'ed; shared so no fixture invents a second one.
const themesDirPath = "/home/user/.config/portal/themes"

// No palette: the fake repaints every valid row onto the `--theme` value, so
// one declared here is overwritten before it can reach a frame.
func themePanelDirEntry(filename, slug string) theme.Entry {
	return theme.Entry{Path: themesDirPath + "/" + filename, Filename: filename, Slug: slug}
}

// A candidate's verdict belongs to the entry the assembler reads, which is why
// the rejection is declared here rather than on the row it produces. An empty
// slug is the `bad name` shape, where the filename yields none.
func themePanelRejectedEntry(filename, slug string, rejection *theme.Rejection) theme.Entry {
	entry := themePanelDirEntry(filename, slug)
	entry.Rejection = rejection
	return entry
}

func themePanelDirEnumeration(entries ...theme.Entry) theme.Enumeration {
	return theme.Enumeration{Entries: entries, DirPath: themesDirPath}
}

// Capture with `--theme tokyo-night` — the fallback the broken dark slot resolves
// to, where the open lands the cursor. That cursor sits on a valid row
// deliberately: the arrow skip keeps it off invalid ones, so a frame parked on a
// rejection is a state production cannot reach.
func themePanelInvalidRowFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "theme-panel-invalid-row"
	// The dark slot names a broken drop-in; the light slot a loadable built-in,
	// so one badge sits on an invalid row and one on a valid one.
	fx.themeKeys = theme.RawKeys{Light: theme.DefaultLightSlug, Dark: themePanelBrokenSlug}
	fx.themeEnumeration = themePanelDirEnumeration(
		// Invalid with a reason that fits beside its label.
		themePanelRejectedEntry("aurora-glow"+theme.FileExtension, "aurora-glow",
			&theme.Rejection{Reason: theme.ReasonBadSyntax, Line: 12, Detail: "quoted value"}),
		// `bad name`: no slug, so labelled and sorted by filename — long enough to
		// fill the row, which drops its reason.
		themePanelRejectedEntry("My Gorgeous Midnight Palette"+theme.FileExtension, "",
			&theme.Rejection{Reason: theme.ReasonBadName, BadNameCause: theme.BadNameSlug}),
		// Invalid and persisted: the badge takes the right edge, so the reason
		// never renders.
		themePanelRejectedEntry(themePanelBrokenSlug+theme.FileExtension, themePanelBrokenSlug,
			&theme.Rejection{Reason: theme.ReasonBadColour, Line: 7, Detail: "text.primary = #12345"}),
	)
	fx.themeUnion = themePanelUnionFrom(fx.themeEnumeration, fx.themeKeys)
	fx.themeSlots = []theme.SlotResolution{
		{Slot: theme.SlotLight, Requested: theme.DefaultLightSlug, Resolved: theme.DefaultLightSlug},
		// The nomination did not load: Resolved names the mode-matched default
		// while Requested — what the badge table keys on — names what the user set.
		{Slot: theme.SlotDark, Requested: themePanelBrokenSlug, Resolved: theme.DefaultDarkSlug, FellBack: true, Reason: theme.ReasonBadColour},
	}
	fx.initialThemeCursor = theme.DefaultDarkSlug
	fx.captureKeys = []tea.KeyPressMsg{keyRune('t')}
	return fx
}

const themePanelBrokenSlug = "nord-frost"

// The dir-unreadable warning is viewport chrome, not a list row — only
// observable after paging past where a list row would have scrolled away,
// hence the Ctrl+↓ and a frame height that forces a second page. Capture
// with `--theme tokyo-night`, where the paging key lands the cursor.
func themePanelDirUnreadableFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "theme-panel-dir-unreadable"
	// Both slots name drop-ins the unreadable directory cannot answer for, so
	// each contributes a persisted row and keeps its badge.
	fx.themeKeys = theme.RawKeys{Light: themePanelUnreachableLightSlug, Dark: themePanelBrokenSlug}
	fx.themeEnumeration = theme.Enumeration{DirUnusable: true, DirPath: themesDirPath}
	fx.themeUnion = themePanelUnionFrom(fx.themeEnumeration, fx.themeKeys)
	fx.themeSlots = []theme.SlotResolution{
		{Slot: theme.SlotLight, Requested: themePanelUnreachableLightSlug, Resolved: theme.DefaultLightSlug, FellBack: true, Reason: theme.ReasonUnreadable},
		{Slot: theme.SlotDark, Requested: themePanelBrokenSlug, Resolved: theme.DefaultDarkSlug, FellBack: true, Reason: theme.ReasonUnreadable},
	}
	// A page-1 row, deliberately: the open would otherwise anchor the cursor on
	// the dark fallback, already on page 2, and the paging key would move nothing.
	fx.initialThemeCursor = "nord"
	fx.captureKeys = []tea.KeyPressMsg{keyRune('t'), keyPageDown()}
	fx.frame.Height = themePanelDirUnreadableFrameHeight
	return fx
}

// Short enough that the panel's list spans two pages.
const themePanelDirUnreadableFrameHeight = 18

// Sorts between the nord* rows and the tokyo-night* ones, putting one badged
// persisted row on each page of the captured frame.
const themePanelUnreachableLightSlug = "solarized-dusk"

// Identical data to the adaptive pair; it declares the terminal that steps the
// panel down to its minimum width, which is the whole of what it exercises —
// every row still on one line, and the badges reserved before the narrowed
// width can crowd them out. Capture with `--theme nord`.
func themePanelNarrowFixture() *Fixture {
	fx := themePanelAdaptivePairFixture()
	fx.name = "theme-panel-narrow"
	fx.width = tui.ThemePanelMinWidthTerminal()
	return fx
}

// The union must overflow the panel body or the pagination dots render on no
// frame. Capture with `--theme nord`.
func themePanelPaginatedFixture() *Fixture {
	fx := themePanelAdaptivePairFixture()
	fx.name = "theme-panel-paginated"
	fx.themeEnumeration = themePanelDirEnumeration(themePanelPaginatedEntries()...)
	fx.themeUnion = themePanelUnionFrom(fx.themeEnumeration, fx.themeKeys)
	return fx
}

// Chosen for margin rather than the exact overflow threshold: the row count at
// which the list stops paginating moves with the capture size, and a union that
// fitted on one page would leave the dots uncovered.
const themePanelSyntheticDropIns = 30

// Named to sort after every built-in slug, keeping the badged rows on page 1.
func themePanelSyntheticSlug(i int) string {
	return fmt.Sprintf("vivid-%02d", i+1)
}

func themePanelPaginatedEntries() []theme.Entry {
	entries := slices.Clone(themePanelEnumeration().Entries)
	for i := range themePanelSyntheticDropIns {
		slug := themePanelSyntheticSlug(i)
		entries = append(entries, themePanelDirEntry(slug+theme.FileExtension, slug))
	}
	return entries
}

// The overlay cuts the Projects page mid-label deliberately: the page beneath
// is composed at the unreduced content width and not re-laid-out. Capture with
// `--theme nord`, the row the open anchors the cursor to.
func themePanelProjectsFixture() *Fixture {
	fx := projectsFixture()
	fx.name = "theme-panel-projects"
	pair := themePanelAdaptivePairFixture()
	fx.themeKeys = pair.themeKeys
	fx.themeEnumeration = pair.themeEnumeration
	fx.themeUnion = themePanelUnionFrom(fx.themeEnumeration, fx.themeKeys)
	fx.themeSlots = pair.themeSlots
	fx.initialThemeCursor = pair.initialThemeCursor
	fx.captureKeys = []tea.KeyPressMsg{keyRune('x'), keyRune('t')}
	return fx
}

// Opens on Sessions (the production no-arg default); `x` reaches the Projects
// page the capture is of.
func projectsFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "orchard-pipeline-code-based", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "portal", Attached: false, Dir: "/home/user/code/portal"},
		{Name: "relayx-3RJDWA", Attached: false, Dir: "/home/user/code/loom/relayx"},
	}

	// relay-v1-api's tags render as the edit-project modal's seeded chips.
	relayPath := "/home/user/code/loom/relayv1/relay-v1-api"
	projects := []project.Project{
		{Name: "relay-v1-api", Path: relayPath, Tags: []string{"Loom", "api"}},
		{Name: "portal", Path: "/home/user/code/portal"},
		{Name: "sorrel", Path: "/home/user/code/sorrel"},
		{Name: "bramble", Path: "/home/user/code/bramble"},
		{Name: "orchard-pipeline", Path: "/home/user/code/orchard-pipeline"},
		{Name: "user", Path: "/home/user"},
		{Name: "relayx", Path: "/home/user/code/loom/relayx"},
		{Name: "atelier-web", Path: "/home/user/code/atelier/atelier-web"},
		{Name: "lumen", Path: "/home/user/code/lumen"},
		{Name: "tessera-proxy", Path: "/home/user/code/tessera"},
		{Name: "kiln-aws-migration", Path: "/home/user/code/kiln"},
		{Name: "quill", Path: "/home/user/code/quill"},
		{Name: "loom", Path: "/home/user/code/loom"},
		{Name: "lumen-sync-engine", Path: "/home/user/code/lumen-sync"},
	}

	return &Fixture{
		name:         "projects",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		// Both editors wired so `e` opens the edit modal (nil-guarded otherwise);
		// aliases are keyed to the same path the project record carries so the
		// modal's per-project lookup matches.
		projectEditor: fakeProjectEditor{},
		aliasEditor: fakeAliasEditor{aliases: map[string]string{
			"rapi": relayPath,
			"v1":   relayPath,
		}},
		initialMode: prefs.ModeFlat,
		captureKeys: []tea.KeyPressMsg{keyRune('x')},
	}
}

// The seeded command stays generic: no fixture may reference a specific tool.
func projectsCommandPendingFixture() *Fixture {
	fx := projectsFixture()
	fx.name = "projects-command-pending"
	fx.command = []string{"npm", "run", "dev"}
	return fx
}

// The history stays generic terminal output: no fixture may reference a
// specific tool.
func previewScreenFixture() *Fixture {
	sessions := []tui.Session{
		{Name: "tessera-proxy-Vb3kQw", Attached: false, Dir: "/home/user/code/tessera"},
		{Name: "orchard-pipeline-code-based", Attached: true, Dir: "/home/user/code/orchard-pipeline"},
		{Name: "kiln-relayx-explore", Attached: false, Dir: "/home/user/code/kiln"},
		{Name: "lumen-sync-engine", Attached: false, Dir: "/home/user/code/lumen"},
	}

	history := "$ kubectl rollout status deploy/tessera-proxy\n" +
		"deployment \"tessera-proxy\" successfully rolled out\n" +
		"$ make build\n" +
		"go build -o bin/tessera-proxy ./cmd/tessera-proxy\n" +
		"build complete in 4.2s\n" +
		"$ ./bin/tessera-proxy --check\n" +
		"config ok · 3 routes · listening on :8080\n"

	return &Fixture{
		name:         "preview-screen",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: nil},
		initialMode:  prefs.ModeFlat,
		history:      history,
		// The history is tessera's, so the cursor sits on its row.
		initialCursor: "tessera-proxy-Vb3kQw",
		captureKeys:   []tea.KeyPressMsg{{Code: tea.KeySpace}},
		// Without the read landing, the frame is the preview's loading line.
		landsLastKeyRead: true,
	}
}

// Overflows any terminal, so the preview's scrolling — keys and swipe alike —
// has somewhere to go in the live view. Numbered lines make each step visible.
func previewLongScrollbackFixture() *Fixture {
	fx := previewScreenFixture()
	fx.name = "preview-long-scrollback"
	var b strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "%3d  build step %d complete\n", i, i)
	}
	fx.history = b.String()
	return fx
}

// Drawn from zmx's own serialisation of a real session, so the frame shows the
// preview's rendering of what zmx sends, not of what a fixture author typed.
func previewZmxHistoryFixture() *Fixture {
	fx := previewScreenFixture()
	fx.name = "preview-zmx-history"
	fx.historyFile = zmxHistoryFile()
	return fx
}

func loadingScreenFixture() *Fixture {
	events := []tui.BootstrapProgressMsg{
		{Index: 1},
		{Index: 2},
		{Index: 3},
		{Index: 4},
		{Index: 5},
		// Sets the active "Restoring sessions" 8/12 counter without completing
		// restore, so the page sits mid-restore.
		{Index: 6, RestoreN: 8, RestoreM: 12},
	}
	return &Fixture{
		name:          "loading-screen",
		Lister:        &fakeLister{sessions: nil},
		projectStore:  &fakeProjectStore{projects: nil},
		initialMode:   prefs.ModeFlat,
		serverStarted: true,
		loadingEvents: events,
	}
}

func loadingErrorFixture() *Fixture {
	events := []tui.BootstrapProgressMsg{
		{Index: 1},
		{Index: 2},
	}
	return &Fixture{
		name:          "loading-error",
		Lister:        &fakeLister{sessions: nil},
		projectStore:  &fakeProjectStore{projects: nil},
		initialMode:   prefs.ModeFlat,
		serverStarted: true,
		loadingEvents: events,
		fatalEvent: tui.BootstrapFatalMsg{
			FailedStep: 3,
			Message:    "Portal failed to set @portal-restoring marker: permission denied",
			Err:        errFixtureFatal,
		},
	}
}
