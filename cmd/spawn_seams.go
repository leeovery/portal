package cmd

import (
	"log/slog"
	"os"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// spawnLogger binds the closed "spawn" log component once for the cmd layer's
// spawn-adjacent emitters. The open command's --ack marker-write chokepoint
// (writeAckMarker) emits its best-effort DEBUG failure line through it.
var spawnLogger = log.For("spawn")

// TerminalDetector resolves the host terminal's identity. It is the Detect()
// seam that lets host-terminal-aware command bodies be Executed with a
// fabricated detector — no real tmux, ps, or defaults reads. Consumers:
// OpenBurstDeps.Detector (the multi-target open burst) and DoctorDeps.Detector
// (the doctor host-terminal line).
type TerminalDetector interface {
	Detect() spawn.Identity
}

// productionSpawnSeams bundles the shared production host-terminal seams that
// both the open burst (buildOpenBurstDeps) and the picker (openTUI's tuiConfig
// population) wire from the same *tmux.Client. Constructing them in one place
// keeps the two paths from silently diverging: because OpenBurstDeps and
// tuiConfig are distinct struct shapes, the compiler cannot catch a seam that is
// added, swapped, or re-constructed on only one side — this bundle is the single
// source both read.
type productionSpawnSeams struct {
	Detector *spawn.Detector
	Resolve  func(spawn.Identity) (spawn.Adapter, spawn.Resolution)
	Ack      spawn.AckChannelFull
	Exe      spawn.ExecutableResolver
	Getenv   func(string) string
	Exists   func(string) bool
	Logger   *slog.Logger
}

// buildProductionSpawnSeams constructs the shared production spawn seams from
// the resolved tmux client: the host-terminal detector, the config-aware
// resolver's Resolve (terminals.json loaded once via buildResolver), the
// server-option ack channel, the executable/env composition seams, the
// has-session pre-flight probe, and the spawn-component logger. It is the single
// construction site the open burst and picker both read, so their production
// wiring cannot drift.
func buildProductionSpawnSeams(client *tmux.Client) productionSpawnSeams {
	return productionSpawnSeams{
		Detector: spawn.NewDetector(client),
		Resolve:  buildResolver().Resolve,
		Ack:      spawn.NewServerOptionAckChannel(client, client),
		Exe:      os.Executable,
		Getenv:   os.Getenv,
		Exists:   client.HasSession,
		Logger:   spawnLogger,
	}
}

// spawnDetector resolves the host-terminal detector against the shared tmux
// client. It is the Detector default the open burst's buildOpenBurstDeps routes
// through when no detector is injected.
func spawnDetector(cmd *cobra.Command) TerminalDetector {
	return spawn.NewDetector(tmuxClient(cmd))
}

// buildResolver constructs the config-aware host-terminal adapter resolver: it
// resolves the terminals.json path through the XDG configFilePath chain, loads
// the escape-hatch config once via TerminalsStore, and wraps it in a
// spawn.Resolver (config override → native → unsupported).
//
// It FAILS SAFE: an undeterminable home/XDG path (a rare configFilePath error)
// degrades to an EMPTY config — native-only resolution — rather than aborting the
// caller, so a broken environment never disables the whole feature.
// TerminalsStore.Load is itself tolerant (missing/unreadable/malformed →
// empty config), so this reads terminals.json without ever crashing the caller.
func buildResolver() *spawn.Resolver {
	cfg := spawn.TerminalsConfig{}
	if path, err := configFilePath("PORTAL_TERMINALS_FILE", "terminals.json"); err == nil {
		cfg = spawn.NewTerminalsStore(path).Load()
	}
	return spawn.NewResolver(cfg)
}
