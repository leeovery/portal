package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/theme"
)

// Deps is the seam set Build assembles a Model from. Optional seams are
// nil-tolerant — a nil field leaves that capability unwired; Lister is required.
type Deps struct {
	Lister SessionLister

	Killer  SessionKiller
	Renamer SessionRenamer
	Creator SessionCreator

	ProjectStore    ProjectStore
	ProjectEditor   ProjectEditor
	AliasEditor     AliasEditor
	Enumerator      TmuxEnumerator
	Reader          ScrollbackReader
	PreviewAttacher PreviewAttacher
	DirReader       session.PaneCurrentPathReader
	DirRunner       resolver.CommandRunner
	ModePersister   ModePersister
	ThemePersister  ThemePersister
	ThemeSource     ThemeSource
	Detector        TerminalDetector
	Resolve         spawn.AdapterResolver

	// The picker burst reuses Resolve above rather than taking a second resolver.
	SessionExists func(string) bool
	AckChannel    spawn.AckChannelFull
	SpawnExe      spawn.ExecutableResolver
	SpawnGetenv   func(string) string
	SpawnLogger   *slog.Logger

	CWD         string
	InitialMode prefs.SessionListMode
	Theme       theme.Nomination
	// ThemeKeys are prefs.json's keys as read — post-translation, with no
	// defaults substituted (the zero value is the shipped adaptive pair). A
	// snapshot: re-reading prefs.json would import another instance's commit.
	ThemeKeys theme.RawKeys

	// Search nil means the picker was not reached by a search form.
	Search *SearchForm

	InitialFilter  string
	Command        []string
	ServerStarted  bool
	InsideTmux     bool
	CurrentSession string
	// ProgressReceiver non-nil selects the concurrent cold-boot path.
	ProgressReceiver tea.Cmd
	// NoColor is read by the cmd layer so internal/tui stays env-free.
	NoColor bool

	Capture CaptureSeeds
}

// SearchForm is the term a session search opened the picker with. It declares
// the session domain, so its landing is the Sessions page whatever the term
// matches.
type SearchForm struct {
	Term string
}

// CaptureSeeds declares first-frame state a one-shot fixture render cannot
// reach by keypress or async lifecycle. Every field is a no-op at its zero
// value. Seeds declare state, never text, so a fixture cannot put a
// paraphrase on a captured frame.
type CaptureSeeds struct {
	// Flash is the warning variant only; the success variant is not seedable.
	Flash       string
	MultiSelect []string
	Cursor      string
	// ThemeCursor is placement only — it applies no theme, so the palette
	// stays the one the nomination carries.
	ThemeCursor       string
	ThemeConfirm      bool
	ThemeCommitFailed bool
	Detection         *spawn.Identity
	GoneFlagged       []string
	// BurstOpening is (done, total).
	BurstOpening [2]int
}

func Build(deps Deps) Model {
	opts := []Option{
		WithKiller(deps.Killer),
		WithRenamer(deps.Renamer),
		WithProjectStore(deps.ProjectStore),
		WithSessionCreator(deps.Creator),
		WithCWD(deps.CWD),
	}
	if deps.ServerStarted {
		opts = append(opts, WithServerStarted(true))
	}
	if deps.ProgressReceiver != nil {
		opts = append(opts, WithProgressReceiver(deps.ProgressReceiver))
	}
	if deps.ProjectEditor != nil {
		opts = append(opts, WithProjectEditor(deps.ProjectEditor))
	}
	if deps.AliasEditor != nil {
		opts = append(opts, WithAliasEditor(deps.AliasEditor))
	}
	if deps.Enumerator != nil {
		opts = append(opts, WithEnumerator(deps.Enumerator))
	}
	if deps.Reader != nil {
		opts = append(opts, WithScrollbackReader(deps.Reader))
	}
	if deps.PreviewAttacher != nil {
		opts = append(opts, WithPreviewAttachPipeline(deps.PreviewAttacher))
	}
	if deps.DirReader != nil && deps.DirRunner != nil {
		opts = append(opts, WithDirResolver(deps.DirReader, deps.DirRunner))
	}
	if deps.Search != nil {
		opts = append(opts, WithSearchForm(deps.Search.Term))
	}
	opts = append(opts, WithInitialMode(deps.InitialMode))
	opts = append(opts, WithThemeNomination(deps.Theme))
	opts = append(opts, WithColourless(deps.NoColor))
	if deps.ModePersister != nil {
		opts = append(opts, WithModePersister(deps.ModePersister))
	}
	if deps.ThemePersister != nil {
		opts = append(opts, WithThemePersister(deps.ThemePersister))
	}
	opts = append(opts, WithThemeKeys(deps.ThemeKeys))
	if deps.ThemeSource != nil {
		opts = append(opts, WithThemeSource(deps.ThemeSource))
	}
	opts = append(opts, WithTerminalDetector(deps.Detector))
	opts = append(opts, WithResolve(deps.Resolve))

	opts = append(opts, WithSessionExists(deps.SessionExists))
	opts = append(opts, WithAckChannel(deps.AckChannel))
	opts = append(opts, WithSpawnExe(deps.SpawnExe))
	opts = append(opts, WithSpawnGetenv(deps.SpawnGetenv))
	opts = append(opts, WithSpawnLogger(deps.SpawnLogger))

	// Capture seeds must apply as Options — before armAppearanceDetection and
	// the first WindowSizeMsg resync reserve the band row.
	opts = append(opts, WithInitialFlash(deps.Capture.Flash))

	opts = append(opts, WithInitialMultiSelect(deps.Capture.MultiSelect))
	opts = append(opts, WithInitialCursor(deps.Capture.Cursor))
	opts = append(opts, WithInitialThemeCursor(deps.Capture.ThemeCursor))
	opts = append(opts, WithInitialThemeConfirm(deps.Capture.ThemeConfirm))
	opts = append(opts, WithInitialThemeCommitFailed(deps.Capture.ThemeCommitFailed))

	// WithInitialGoneFlagged applies after WithInitialMultiSelect so its
	// delegate refresh observes the seeded marked set.
	opts = append(opts, WithInitialDetection(deps.Capture.Detection))
	opts = append(opts, WithInitialGoneFlagged(deps.Capture.GoneFlagged))
	opts = append(opts, WithInitialBurstOpening(deps.Capture.BurstOpening[0], deps.Capture.BurstOpening[1]))

	m := New(deps.Lister, opts...)
	if len(deps.Command) > 0 {
		m = m.WithCommand(deps.Command)
	}
	if deps.InitialFilter != "" {
		m = m.WithInitialFilter(deps.InitialFilter)
	}
	if deps.InsideTmux && deps.CurrentSession != "" {
		m = m.WithInsideTmux(deps.CurrentSession)
	}
	m.armAppearanceDetection()
	return m
}
