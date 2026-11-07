package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/theme"
)

// Deps is the seam set Build assembles a Model from. Optional seams are
// nil-tolerant — a nil field leaves that capability unwired; Lister is required.
type Deps struct {
	Lister SessionLister

	Killer   SessionKiller
	Labeller SessionLabeller
	Creator  SessionCreator
	// CreateFatal is the fatal a mint's failure is, nil for one that is none
	// and so flashes. Nil maps none.
	CreateFatal func(error) error

	ProjectStore  ProjectStore
	ProjectEditor ProjectEditor
	AliasEditor   AliasEditor
	History       HistoryReader
	// Transcripts reads the transcript a waiting session was restored from,
	// which its preview shows. Nil previews its History.
	Transcripts TranscriptReader
	// DirRunner resolves the git root of a session's cwd, which grouping falls
	// back to for a session with no recorded directory.
	DirRunner resolver.CommandRunner
	// DirKey reduces a directory to the key sessions are matched to projects
	// by. Nil takes project.CanonicalDirKey, which resolves symlinks on disk; a
	// capture pins one that reads nothing.
	DirKey        func(string) string
	PendingReader PendingResumeReader
	// Hydrating keeps the cold route re-reading the pending set until no
	// restored session is still hydrating, for at most PendingSettleBound.
	Hydrating          HydratingReader
	PendingSettleBound time.Duration
	ModePersister      ModePersister
	ThemePersister     ThemePersister
	ThemeSource        ThemeSource
	Detector           TerminalDetector
	Resolve            spawn.AdapterResolver

	// The picker burst reuses Resolve above rather than taking a second resolver.
	SessionExists func(string) bool
	AckChannel    spawn.AckChannelFull
	SpawnExe      spawn.ExecutableResolver
	SpawnEnv      spawn.WindowEnv
	SpawnLogger   *slog.Logger
	// Flash is a warning the picker opens on, such as why the attach before it
	// failed to do what was asked.
	Flash string
	// KillAsk is the session the kill key asked to kill from inside it: the
	// picker opens on its kill prompt once its row is listed, whose Esc
	// attaches it again. Keys typed before then are dropped.
	KillAsk string
	// AskedKiller kills on y at the prompt KillAsk opens; without one, no
	// prompt opens.
	AskedKiller SessionKiller

	CWD string
	// Home is the home directory session directories are abbreviated against.
	// Empty takes the process's, which New resolves once; a capture pins one so
	// no frame reads $HOME.
	Home        string
	InitialMode prefs.SessionListMode
	Theme       theme.Nomination
	// ThemeKeys are prefs.json's keys as read — post-translation, with no
	// defaults substituted (the zero value is the shipped adaptive pair). A
	// snapshot: re-reading prefs.json would import another instance's commit.
	ThemeKeys theme.RawKeys

	// Search nil means the picker was not reached by a search form.
	Search *SearchForm

	InitialFilter string
	Command       []string
	// CurrentSession is the session the picker runs inside, empty outside one.
	CurrentSession string
	// ProgressReceiver is the bootstrap running behind the picker: non-nil
	// opens it on its loading page, which the bootstrap's events dismiss.
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
	// Decide classifies the search against the live session list at the point
	// that list can answer for it. A non-empty name is the session to attach,
	// ("", nil) opens the picker, and a non-nil error is a failed session-list
	// read. Nil leaves the classification unwired.
	Decide func() (string, error)
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
		WithAskedKiller(deps.AskedKiller),
		WithLabeller(deps.Labeller),
		WithProjectStore(deps.ProjectStore),
		WithSessionCreator(deps.Creator),
		WithCreateFatal(deps.CreateFatal),
		WithCWD(deps.CWD),
	}
	if deps.Home != "" {
		opts = append(opts, WithHome(deps.Home))
	}
	if deps.ProgressReceiver != nil {
		opts = append(opts, WithBootstrapping(deps.ProgressReceiver))
	}
	if deps.ProjectEditor != nil {
		opts = append(opts, WithProjectEditor(deps.ProjectEditor))
	}
	if deps.AliasEditor != nil {
		opts = append(opts, WithAliasEditor(deps.AliasEditor))
	}
	if deps.History != nil {
		opts = append(opts, WithHistory(deps.History))
	}
	if deps.Transcripts != nil {
		opts = append(opts, WithTranscripts(deps.Transcripts))
	}
	if deps.DirRunner != nil {
		opts = append(opts, WithDirRunner(deps.DirRunner))
	}
	if deps.DirKey != nil {
		opts = append(opts, WithDirKey(deps.DirKey))
	}
	if deps.PendingReader != nil {
		opts = append(opts, WithPendingResumeReader(deps.PendingReader))
	}
	if deps.Hydrating != nil {
		opts = append(opts, WithPendingSettle(deps.Hydrating, deps.PendingSettleBound))
	}
	if deps.Search != nil {
		opts = append(opts, WithSearchForm(deps.Search.Term), WithSearchDecision(deps.Search.Decide))
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
	opts = append(opts, WithSpawnEnv(deps.SpawnEnv))
	opts = append(opts, WithSpawnLogger(deps.SpawnLogger))
	opts = append(opts, WithOpeningFlash(deps.Flash))
	opts = append(opts, WithKillAsk(deps.KillAsk))

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
	if deps.Search == nil && deps.InitialFilter != "" {
		m = m.WithInitialFilter(deps.InitialFilter)
	}
	if deps.CurrentSession != "" {
		m = m.WithCurrentSession(deps.CurrentSession)
	}
	m.armAppearanceDetection()
	return m
}
