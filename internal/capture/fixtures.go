package capture

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
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

	Lister *fakeLister

	projectStore       *fakeProjectStore
	projectEditor      tui.ProjectEditor
	aliasEditor        tui.AliasEditor
	initialMode        prefs.SessionListMode
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
	scrollback          string
	enumeratorGroups    []tmux.WindowGroup
	serverStarted       bool
	loadingEvents       []tui.BootstrapProgressMsg
	fatalEvent          tui.BootstrapFatalMsg
	// Tapes are scaffolding and do not live in the repository, so the post-load
	// key script is declared here.
	captureKeys []tea.KeyPressMsg
	noColor     bool
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
		Killer:        fakeKiller{},
		Renamer:       fakeRenamer{},
		Creator:       fakeCreator{},
		ProjectStore:  f.projectStore,
		ProjectEditor: f.projectEditor,
		AliasEditor:   f.aliasEditor,
		Enumerator:    fakeEnumerator{groups: f.enumeratorGroups},
		Reader:        fakeScrollbackReader{content: f.scrollback},
		// DirReader/DirRunner stay nil (sessions are pre-stamped, so the lazy
		// pane-read fallback never fires); ModePersister stays nil so an
		// `s`-toggle writes nowhere.
		InitialMode:   f.initialMode,
		Command:       f.command,
		CWD:           "/home/user",
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
		sessionsByTagFixture,
		sessionsPagedFixture,
		sessionsInlineFlashFixture,
		sessionsRenameRefusedSeparatorFixture,
		sessionsRenameRefusedIDPrefixFixture,
		sessionsRenameRefusedFlagPrefixFixture,
		sessionsMultiSelectActiveFixture,
		sessionsUnsupportedTerminalFixture,
		sessionsUnsupportedNullFixture,
		sessionsMultiSelectPreflightAbortFixture,
		sessionsBurstOpeningFixture,
		sessionsNoTagsSignpostFixture,
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
	return nil, fmt.Errorf("unknown fixture %q (available: %s)", name, strings.Join(FixtureNames(), ", "))
}

// FixtureNames includes the contrast-validation swatch, which is a standalone
// tea.Model rather than a *Fixture.
func FixtureNames() []string {
	builders := fixtureBuilders()
	names := make([]string, 0, len(builders)+1)
	for _, build := range builders {
		names = append(names, build().Name())
	}
	names = append(names, ContrastValidationFixture)
	slices.Sort(names)
	return names
}

// Every session carries a stamped Dir so the grouped-render lazy pane-read
// fallback never fires (the harness has no tmux server).
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
	sessions := []tmux.Session{
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true, Dir: "/home/user/code/agentic-workflows"},
		{Name: "agentic-workflows-codify", Windows: 2, Attached: false, Dir: "/home/user/code/agentic-workflows"},
		{Name: "fab-flowx-explore", Windows: 1, Attached: false, Dir: "/home/user/code/fab"},
		{Name: "evvi webhooks and watchers", Windows: 4, Attached: false, Dir: "/home/user/code/evvi"},
		{Name: "aviva-proxy-qNyfEO", Windows: 1, Attached: false, Dir: "/home/user/code/aviva"},
		{Name: "designlab-web-r8suyU", Windows: 2, Attached: false, Dir: "/home/user/code/designlab"},
		// Matches no project → lands in the Unknown catch-all.
		{Name: "orphan-explore", Windows: 1, Attached: false, Dir: "/home/user/code/untracked-scratch"},
	}

	projects := []project.Project{
		{Path: "/home/user/code/agentic-workflows", Name: "agentic-workflows"},
		{Path: "/home/user/code/fab", Name: "fab"},
		{Path: "/home/user/code/evvi", Name: "evvi"},
		{Path: "/home/user/code/aviva", Name: "aviva"},
		{Path: "/home/user/code/designlab", Name: "designlab"},
	}

	return &Fixture{
		name:         "sessions-by-project",
		Lister:       &fakeLister{sessions: sessions},
		projectStore: &fakeProjectStore{projects: projects},
		initialMode:  prefs.ModeByProject,
	}
}

// The paths do not exist; CanonicalDirKey falls back to Clean(abs) on both
// sides, so the match is deterministic without a real filesystem.
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
		// Deliberately untagged → its sessions land in the Untagged catch-all,
		// so a tagged heading and the catch-all share one frame.
		{Path: "/home/user/code/evvi", Name: "evvi"},
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
		"/home/user/code/agentic-workflows",
		"/home/user/code/fab",
		"/home/user/code/evvi",
		"/home/user/code/aviva",
		"/home/user/code/designlab",
		"/home/user/code/flow",
	}
	sessions := make([]tmux.Session, 0, count)
	for i := range count {
		sessions = append(sessions, tmux.Session{
			Name:     fmt.Sprintf("session-%02d", i),
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

// The refusal is typed, never seeded: the keys open the real rename modal and
// commit an unaddressable name, so the band carries production's own wording
// rather than a copy of it this file could drift from.
func sessionsRenameRefusedSeparatorFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-rename-refused-separator"
	fx.captureKeys = []tea.KeyPressMsg{keyRune('r'), keyRune(':'), keyEnter()}
	return fx
}

// The modal opens with the cursor at the end of the current name and only a
// leading "$" is refused, so the line-start key is load-bearing.
func sessionsRenameRefusedIDPrefixFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-rename-refused-id-prefix"
	fx.captureKeys = []tea.KeyPressMsg{keyRune('r'), keyLineStart(), keyRune('$'), keyEnter()}
	return fx
}

// The third refusal, and the longest of the three: only a leading "-" is
// refused, so the line-start key is load-bearing here too.
func sessionsRenameRefusedFlagPrefixFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-rename-refused-flag-prefix"
	fx.captureKeys = []tea.KeyPressMsg{keyRune('r'), keyLineStart(), keyRune('-'), keyEnter()}
	return fx
}

func sessionsMultiSelectActiveFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-multi-select-active"
	fx.initialMultiSelect = []string{
		"agentic-workflows-codify",
		"fab-flowx-explore",
		"designlab-web-r8suyU",
	}
	fx.initialCursor = "fab-flowx-explore"
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
		"agentic-workflows-codify",
		"fab-flowx-explore",
		"designlab-web-r8suyU",
	}
	fx.initialCursor = "fab-flowx-explore"
	fx.initialGoneFlagged = []string{"fab-flowx-explore"}
	return fx
}

func sessionsBurstOpeningFixture() *Fixture {
	fx := sessionsFlatFixture()
	fx.name = "sessions-burst-opening"
	fx.initialMultiSelect = []string{
		"agentic-workflows-codify",
		"fab-flowx-explore",
		"designlab-web-r8suyU",
	}
	fx.initialBurstOpening = [2]int{2, 3}
	return fx
}

// The project store is populated deliberately: the zero-tags condition, not a
// missing store, must drive the signpost.
func sessionsNoTagsSignpostFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 3, Attached: true, Dir: "/home/user/code/fab"},
		{Name: "agentic-workflows-codify", Windows: 1, Attached: false, Dir: "/home/user/code/agentic-workflows"},
		{Name: "flowx-7UKPZH", Windows: 2, Attached: false, Dir: "/home/user/code/flowx"},
		{Name: "aviva-proxy-qNyfEO", Windows: 4, Attached: false, Dir: "/home/user/code/aviva"},
	}

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
// declares no render size, so that width comes from the capturing terminal: at
// a wide one the same frame renders the confirm on a single row.
func themePanelConfirmFixture() *Fixture {
	fx := themePanelConstantPreviewingFixture()
	fx.name = "theme-panel-confirm"
	fx.initialThemeConfirm = true
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

const themePanelBrokenSlug = "nord-lee"

// The dir-unreadable warning is viewport chrome, not a list row — only
// observable after paging past where a list row would have scrolled away,
// hence the Ctrl+↓ and a capture height that forces a second page. Capture
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
	return fx
}

// Sorts between the nord* rows and the tokyo-night* ones, putting one badged
// persisted row on each page of the captured frame.
const themePanelUnreachableLightSlug = "solarized-lee"

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
	sessions := []tmux.Session{
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true, Dir: "/Users/leeovery/Code/agentic-workflows"},
		{Name: "portal", Windows: 2, Attached: false, Dir: "/Users/leeovery/Code/portal"},
		{Name: "flowx-7UKPZH", Windows: 1, Attached: false, Dir: "/Users/leeovery/Code/fabric/flowx"},
	}

	// flow-v1-api's tags render as the edit-project modal's seeded chips.
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
		// Both editors wired so `e` opens the edit modal (nil-guarded otherwise);
		// aliases are keyed to the same path the project record carries so the
		// modal's per-project lookup matches.
		projectEditor: fakeProjectEditor{},
		aliasEditor: fakeAliasEditor{aliases: map[string]string{
			"fapi": flowPath,
			"v1":   flowPath,
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

// The scrollback stays generic terminal output: no fixture may reference a
// specific tool.
func previewScreenFixture() *Fixture {
	sessions := []tmux.Session{
		{Name: "aviva-proxy-qNyfEO", Windows: 1, Attached: false, Dir: "/home/user/code/aviva"},
		{Name: "agentic-workflows-code-based", Windows: 3, Attached: true, Dir: "/home/user/code/agentic-workflows"},
		{Name: "fab-flowx-explore", Windows: 1, Attached: false, Dir: "/home/user/code/fab"},
		{Name: "evvi-sync-engine", Windows: 1, Attached: false, Dir: "/home/user/code/evvi"},
	}

	scrollback := "$ kubectl rollout status deploy/aviva-proxy\n" +
		"deployment \"aviva-proxy\" successfully rolled out\n" +
		"$ make build\n" +
		"go build -o bin/aviva-proxy ./cmd/aviva-proxy\n" +
		"build complete in 4.2s\n" +
		"$ ./bin/aviva-proxy --check\n" +
		"config ok · 3 routes · listening on :8080\n"

	return &Fixture{
		name:             "preview-screen",
		Lister:           &fakeLister{sessions: sessions},
		projectStore:     &fakeProjectStore{projects: nil},
		initialMode:      prefs.ModeFlat,
		scrollback:       scrollback,
		enumeratorGroups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}},
		captureKeys:      []tea.KeyPressMsg{{Code: tea.KeySpace}},
	}
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
