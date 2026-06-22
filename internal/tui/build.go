package tui

import (
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
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
	// Appearance is the persisted colour-scheme preference (auto/light/dark). It
	// is the SINGLE driver of the owned canvas (§1): Build injects it via
	// WithAppearance and the model resolves the painted canvas mode from it plus
	// OSC 11 detection (the §2.6 detect-or-timeout gate). A pinned light/dark
	// appearance paints that canvas from frame one with no detection; auto detects
	// with a dark fallback. The offline capture harness drives a deterministic
	// canvas by PINNING Appearance to light/dark (the pin path), so its frames are
	// un-gated and byte-stable. There is no separate injected CanvasMode — the
	// former temporary 1-6 seam is gone now that detection resolves the mode.
	Appearance    prefs.Appearance
	InitialFilter string
	// InitialFlash seeds the §11.2 inline WARNING flash on the first frame (the
	// orange ▌ bar + ⚠ + message on the bg.warning tint). It exists for the offline
	// capture harness: the flash is otherwise transient (set only by the
	// preview-bail path), so the fixture seeds the band directly to screenshot it.
	// Empty (the production default) leaves no flash. Only the warning variant is
	// seedable — the success variant is not separately captured.
	InitialFlash   string
	Command        []string
	ServerStarted  bool
	InsideTmux     bool
	CurrentSession string
	// NoColor is the NO_COLOR carve-out decision (§2.5). The cmd layer reads
	// os.Getenv("NO_COLOR") (present and non-empty, the no-color.org convention)
	// and injects the boolean here so internal/tui stays env-free. Build sets ONE
	// colourless flag on the model (WithColourless); every canvas-dependent surface
	// inherits that single flag rather than re-deriving NO_COLOR. When true, Portal
	// paints no canvas at all and skips light/dark detection + the first-paint wait
	// — there is no canvas to select.
	NoColor bool
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
	// Appearance is always injected — AppearanceAuto is a valid explicit value
	// and the sole driver of the owned canvas mode. The model resolves the
	// painted canvas from it (pin → immediate; auto → OSC 11 detect-or-timeout).
	opts = append(opts, WithAppearance(deps.Appearance))
	// NoColor is the single NO_COLOR carve-out (§2.5). When set it WINS over the
	// appearance-driven gate (New consumes it after the options apply): the canvas
	// is suppressed and detection is skipped. Always injected — false is the
	// no-op coloured path, so omitting it leaves the canvas painted.
	opts = append(opts, WithColourless(deps.NoColor))
	if deps.ModePersister != nil {
		opts = append(opts, WithModePersister(deps.ModePersister))
	}

	// Seed the §11.2 inline warning flash for the capture harness (no-op when empty,
	// the production default). Applied as an Option so it is set before the
	// armAppearanceDetection / first WindowSizeMsg resync reserves the band row.
	opts = append(opts, WithInitialFlash(deps.InitialFlash))

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
	// Open the §2.6 detect-or-timeout first-paint window for the LIVE picker. New
	// constructs an auto gate already resolved to the dark fallback (so directly
	// constructed test models paint immediately); production opens the window here
	// so the live program holds the neutral blank frame until OSC 11 resolves the
	// mode — no paint-then-flip. armAppearanceDetection is a no-op on a pinned
	// (light/dark) appearance and on a WithCanvasMode capture override, so those
	// keep painting from frame one. The capture harness drives the pin path, so
	// its frames stay deterministic and un-gated.
	m.armAppearanceDetection()
	return m
}
