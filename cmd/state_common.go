package cmd

import (
	"os"

	"github.com/leeovery/portal/internal/log"
)

// cmd spans several taxonomy components, so it binds one logger per component it
// emits under rather than the usual single package-scope logger.
var (
	hydrateLogger   = log.For("hydrate")
	bootstrapLogger = log.For("bootstrap")
	// The component (subsystem) is orthogonal to process_role (binary): the
	// signal-hydrate command's role stays `hydrate` while the subsystem it
	// instruments is signal.
	signalLogger = log.For("signal")
)

// osExit is the seam over os.Exit; never call os.Exit directly in this package.
// Every call site must first log.Close(N), or the run leaves an unpaired
// "process: start" behind.
var osExit = os.Exit
