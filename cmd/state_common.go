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
)
