package cmd

import "github.com/leeovery/portal/internal/log"

// cmd spans several taxonomy components, so it binds one logger per component it
// emits under rather than the usual single package-scope logger.
var (
	daemonLogger    = log.For("daemon")
	hydrateLogger   = log.For("hydrate")
	notifyLogger    = log.For("notify")
	hooksLogger     = log.For("hooks")
	bootstrapLogger = log.For("bootstrap")
	// The component (subsystem) is orthogonal to process_role (binary): the
	// signal-hydrate command's role stays `hydrate` while the subsystem it
	// instruments is signal.
	signalLogger  = log.For("signal")
	captureLogger = log.For("capture")
)
