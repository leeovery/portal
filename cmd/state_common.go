package cmd

import "github.com/leeovery/portal/internal/log"

// Component-bound loggers for the cmd package. Each subcommand body logs
// under the taxonomy component it owns; the handler is configured once by
// main -> log.Init, so these bindings (made at package init via log.For)
// route through the live handler without per-process log opens. The cmd
// package hosts several subcommands spanning multiple components, so —
// unlike a single-component package — it binds one logger per component it
// emits under rather than a single package-scope `logger`.
//
// These replace the old per-process non-rotating log open + defer Close
// ceremony: log rotation and the append-only writer discipline are now
// handler-owned (Phase 2), so command bodies no longer open or close a
// logger of their own.
var (
	daemonLogger    = log.For("daemon")
	hydrateLogger   = log.For("hydrate")
	notifyLogger    = log.For("notify")
	hooksLogger     = log.For("hooks")
	bootstrapLogger = log.For("bootstrap")
	restoreLogger   = log.For("restore")
	previewLogger   = log.For("preview")
	// signalLogger is the signal-component-bound logger for the cmd-layer
	// signal-hydrate command (portal state signal-hydrate). Its
	// enumerate-markers-then-write-FIFO diagnostics render under
	// component=signal — matching the structural sibling EagerSignalHydrate
	// (cmd/bootstrap) and the lower-level plumbing (internal/state) — so
	// `grep "signal:"` reconstructs the FIFO-signaling mechanism per the
	// Subsystem prefix taxonomy (spec § signal row). component (subsystem) is
	// ORTHOGONAL to process_role (binary): the command's process_role stays
	// `hydrate` (it's the hydrate binary, resolved from argv), but the
	// subsystem it instruments is signal — exactly as EagerSignalHydrate emits
	// component=signal from a bootstrap/tui process_role.
	signalLogger = log.For("signal")
	// captureLogger is the component-bound logger for the daemon's per-tick
	// capture loop. Per the taxonomy the capture loop is promoted out of the
	// daemon component into its own "capture" component so the cycle summary
	// (capture: tick complete) and the per-pane DEBUG breadcrumbs grep cleanly
	// apart from the daemon's lifecycle lines. The per-pane WARNs stay on
	// daemonLogger (lowest-churn; preserves their existing component).
	captureLogger = log.For("capture")
)
