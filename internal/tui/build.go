package tui

import (
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Deps is the compiler-enforced seam set from which Build assembles a Model.
//
// It is the single shared description of every dependency the TUI model needs:
// production (cmd/open.go) populates it with the real *tmux.Client and the
// concrete config stores; the offline capture harness (cmd/capturetool) populates
// it with in-memory fakes. Because both paths construct the same struct, a fake
// that drifts from the real seam set is a compile error, not a silent render
// divergence — which is exactly the property the visual-verification harness
// depends on (the captured TUI is the production TUI).
//
// Optional seams are nil-tolerant: a nil interface field leaves that capability
// unwired, mirroring the behaviour of omitting the corresponding With* option.
// The exceptions are Lister (the one required seam, mirroring New's required
// first argument) and InitialMode (Flat is a valid explicit value that always
// applies).
type Deps struct {
	// Required seam — the session source (mirrors New's first argument).
	Lister SessionLister

	// Core action seams (always passed in production, no-ops in the harness).
	Killer  SessionKiller
	Renamer SessionRenamer
	Creator SessionCreator

	// Optional seams — a nil field leaves the capability unwired (matching an
	// omitted With* option).
	ProjectStore    ProjectStore
	ProjectEditor   ProjectEditor
	AliasEditor     AliasEditor
	Enumerator      TmuxEnumerator
	Reader          ScrollbackReader
	PreviewAttacher PreviewAttacher
	DirReader       session.PaneCurrentPathReader
	DirRunner       resolver.CommandRunner
	ModePersister   ModePersister

	// Scalar configuration.
	CWD         string
	InitialMode prefs.SessionListMode
	// Appearance is the persisted colour-scheme preference (auto by default). Like
	// InitialMode it is a scalar with a meaningful zero value (AppearanceAuto), so
	// Build always injects it via WithAppearance.
	Appearance prefs.Appearance
	// CanvasMode is the RESOLVED light/dark appearance the owned canvas (§1) is
	// painted for — distinct from Appearance (the pref). theme.Dark is the
	// zero-value default (the §2.6 no-answer fallback), so Build always injects it
	// via WithCanvasMode. Detection (1-7) will resolve it from Appearance + OSC 11
	// before constructing Deps; the capture harness injects it from its
	// --appearance flag so the light canvas can be captured.
	CanvasMode     theme.Mode
	InitialFilter  string
	Command        []string
	ServerStarted  bool
	InsideTmux     bool
	CurrentSession string
}

// Build constructs a Model from the shared Deps seam set. It is the single
// model-construction chokepoint used by BOTH cmd/open.go (real *tmux.Client) and
// cmd/capturetool (in-memory fakes), so the harness captures the exact model
// production renders.
//
// The option assembly below mirrors the legacy inline construction in
// cmd/open.go one-for-one: the same nil-guards, the always-injected initial
// mode, and the post-construction WithCommand / WithInitialFilter / WithInsideTmux
// chained mutations applied in the same order. It is a behaviour-preserving lift,
// not a new construction path.
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
	// Initial mode is always injected — Flat is a valid explicit value, and New
	// recomputes the list title after options apply so the first frame paints the
	// correct mode heading.
	opts = append(opts, WithInitialMode(deps.InitialMode))
	// Appearance is always injected — AppearanceAuto is a valid explicit value.
	opts = append(opts, WithAppearance(deps.Appearance))
	// Canvas mode is always injected — theme.Dark is a valid explicit value and
	// the §2.6 no-answer fallback, so an unset CanvasMode paints the dark canvas.
	opts = append(opts, WithCanvasMode(deps.CanvasMode))
	if deps.ModePersister != nil {
		opts = append(opts, WithModePersister(deps.ModePersister))
	}

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
	return m
}
