package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

// openBurstDeps holds injectable dependencies for the multi-target open burst.
// When nil, buildOpenBurstDeps uses production defaults.
var openBurstDeps *OpenBurstDeps

// OpenBurstDeps allows injecting dependencies for the multi-target open burst
// pipeline (Task 3-6). Its shared seams default from the same productionSpawnSeams
// bundle (cmd/spawn_seams.go) the picker burst reads, but it is FIRST-trigger and
// attach/mint-aware: the trigger absorbs the FIRST target and
// self-connects LAST — via either the inside/outside-tmux Connector (an attach
// trigger) or LocalMint (a mint trigger) — while the N−1 non-trigger surfaces are
// spawned FIRST through the Burster. Every field defaults to its production
// implementation when unset (see buildOpenBurstDeps), so a test overrides exactly
// the seams it needs and drives the whole pipeline without a real tmux server,
// osascript, or process handoff.
type OpenBurstDeps struct {
	// Detector resolves the host terminal identity for the N≥2 burst. Defaults to
	// the shared production process-tree detector (via spawnDetector).
	Detector TerminalDetector
	// Resolve maps an identity to its window-opening adapter plus the resolution
	// classification. Defaults to the config-aware buildResolver().Resolve.
	Resolve spawn.AdapterResolver
	// Connector self-connects an ATTACH trigger (switch-client inside tmux / exec
	// attach outside). Defaults to buildSessionConnector(tmuxClient(cmd)).
	Connector SessionConnector
	// ExePath resolves the picker's own binary for the spawned windows' attach/mint
	// argv composition. Defaults to os.Executable.
	ExePath spawn.ExecutableResolver
	// Getenv reads the environment (PATH) for argv composition. Defaults to os.Getenv.
	Getenv func(string) string
	// Ack is the token-ack channel the burst cleans after the N−1 windows spawn and
	// before the trigger self-connect handoff. Defaults to the shared server-option
	// ack channel.
	Ack spawn.AckChannelFull
	// NewBurster constructs the burst orchestrator from the resolved adapter.
	// Defaults to a production spawn.Burster reading the defaulted Ack/ExePath/Getenv.
	NewBurster func(adapter spawn.Adapter) *spawn.Burster
	// Logger receives the unsupported-terminal outcome line, the batch summary,
	// and per-window records. Defaults to the spawn-component logger (log.For("spawn")).
	Logger *slog.Logger
	// LocalMint self-connects a MINT trigger: it mints a fresh session at the
	// resolved literal dir in the invoking terminal, threading the mint-scoped
	// command (the same create-or-attach path a spawned mint window takes). Defaults
	// to openPath (via openPathFunc).
	LocalMint func(cmd *cobra.Command, dir string, command []string) error
}

// buildOpenBurstDeps returns a fully-populated OpenBurstDeps for the open burst:
// injected fields (from openBurstDeps in tests) are kept, and every unset field
// falls back to its production default. The shared production seams
// (Resolve/Ack/ExePath/Getenv/Logger) come from the SAME buildProductionSpawnSeams
// bundle the picker (openTUI's tuiConfig population) also reads, so the two burst
// paths cannot silently diverge. The bundle is memoised lazily — built at most
// once, and only when a shared field actually needs defaulting — so a
// fully-injected caller never resolves the tmux client (there is none in context
// under nopRunner) nor loads terminals.json. The Detector default routes through
// spawnDetector (the standalone host-terminal detector, cmd/spawn_seams.go) — the
// same detector the picker and the `portal doctor` host-terminal line use.
func buildOpenBurstDeps(cmd *cobra.Command) *OpenBurstDeps {
	deps := &OpenBurstDeps{}
	if openBurstDeps != nil {
		*deps = *openBurstDeps
	}

	var (
		seams      productionSpawnSeams
		seamsBuilt bool
	)
	sharedSeams := func() productionSpawnSeams {
		if !seamsBuilt {
			seams = buildProductionSpawnSeams(tmuxClient(cmd))
			seamsBuilt = true
		}
		return seams
	}

	if deps.Detector == nil {
		deps.Detector = spawnDetector(cmd)
	}
	if deps.Resolve == nil {
		deps.Resolve = sharedSeams().Resolve
	}
	if deps.Connector == nil {
		deps.Connector = buildSessionConnector(tmuxClient(cmd))
	}
	if deps.ExePath == nil {
		deps.ExePath = sharedSeams().Exe
	}
	if deps.Getenv == nil {
		deps.Getenv = sharedSeams().Getenv
	}
	if deps.Ack == nil {
		deps.Ack = sharedSeams().Ack
	}
	if deps.NewBurster == nil {
		// Lazy closure: reads the (now-defaulted) Ack/ExePath/Getenv at burst time,
		// so it never re-resolves the tmux client here and composes the same
		// production burster the N−1 external half drives.
		deps.NewBurster = func(adapter spawn.Adapter) *spawn.Burster {
			return spawn.NewBurster(adapter, deps.Ack, deps.ExePath, deps.Getenv)
		}
	}
	if deps.Logger == nil {
		deps.Logger = sharedSeams().Logger
	}
	if deps.LocalMint == nil {
		deps.LocalMint = func(c *cobra.Command, dir string, command []string) error {
			return openPathFunc(c, dir, command)
		}
	}
	return deps
}

// runOpenBurstWithDeps opens the N≥2 resolved surfaces of a multi-target open with
// a FIRST-trigger split: the trigger (the FIRST surface in command-line order)
// absorbs the invoking terminal and self-connects LAST, while the N−1 non-trigger
// surfaces are spawned FIRST into host-terminal windows (spec § The trigger absorbs
// the first target; § Atomic pre-flight & partial failure).
//
// Execution order is load-bearing OUTSIDE tmux: the trigger's connector may
// exec-replace the Portal process (exec attach) and a local mint likewise hands
// off, so the N−1 windows MUST be spawned before the trigger self-connects —
// connecting first would destroy the burster and open only one surface.
//
// The current session is never special-cased: a session gets a window only when it
// appears in the surface set, and duplicates are honored (never deduped) — the
// surfaces slice is taken literally. The inside/outside-tmux split selects ONLY the
// trigger's connector; the N−1 always run the spawned out-of-tmux `open … --ack`
// argv.
//
// Precondition: len(surfaces) >= 2 (dispatchOpenBurst routes a single surface to
// the plain single-target connect).
func runOpenBurstWithDeps(cmd *cobra.Command, surfaces []spawn.Surface, command []string, deps *OpenBurstDeps) error {
	// Multi-target zero-mint command guard (spec § Mint-only command): a command
	// (-e/--) rides mint windows only, so a multi-target set carrying a command with
	// ZERO mint surfaces (every surface is an attach) has nowhere to run it. Refuse
	// with the Task 2-6 message BEFORE detect/resolve/spawn — the multi-target arity
	// of the single-target attach-command guard (openResolved's *SessionResult arm).
	if len(command) > 0 && !hasMintSurface(surfaces) {
		return NewUsageError(commandAttachOnlyMessage)
	}

	trigger, external := spawn.SplitTriggerFirst(surfaces)

	// Detect the host terminal, then resolve its window-opening adapter. Order is
	// load-bearing: detect first, then resolve.
	id := deps.Detector.Detect()
	adapter, resolution := deps.Resolve(id)

	// Atomic no-op gate: an N≥2 burst on an unsupported/NULL terminal cannot open
	// its N−1 external windows (no adapter is available), so refuse before spawning
	// OR self-connecting. The trigger does NOT half-connect (RESOLVED 2026-07-18:
	// block N≥2 outright on unsupported/remote — a partial "trigger only" open would
	// violate the all-surfaces intent). The error names the detected identity.
	if resolution == spawn.ResolutionUnsupported {
		spawn.LogUnsupported(deps.Logger, id)
		return errors.New(spawn.UnsupportedNoopMessage(id))
	}

	// Spawn the N−1 external surfaces FIRST (before the trigger self-connects). A
	// pre-spawn abort — the executable or an ack id failed to resolve before any
	// window opened — returns immediately, so the trigger never connects on a burst
	// that could not even start. The per-window ~8s ack timeout is provided by the
	// burster itself (spawnAckTimeout + awaitToken, each window timed from its OWN
	// spawn); this path adds no timeout logic of its own.
	batch, results, err := deps.NewBurster(adapter).Run(context.Background(), external, command, nil)
	if err != nil {
		return err
	}

	// Clean the batch markers on every post-burst path, BEFORE the trigger
	// self-connect handoff (a point of no return outside tmux, where exec attach
	// replaces the process). Best-effort: bounded, harmless leaks self-expire with
	// the tmux server.
	_ = deps.Ack.Clean(batch)

	// Report the N−1 external outcomes on every post-burst path — leave-what-opened.
	// The trigger's self-connect below is INDEPENDENT of these outcomes (spec §211
	// trigger-independence): its target is unrelated to the externals', so this
	// reporting must NOT gate the connect. It logs to portal.log (the durable
	// surface) and, at most, prints ONE best-effort stderr line — swallowed by a
	// successful attach, directly visible only in the trigger's own-target-failed
	// skip case (connectTrigger returns an error).
	//
	// DIVERGENCE FROM the picker's all-attach burst (internal/tui,
	// handleBurstPartialFailure): the picker SKIPS its trigger self-attach on ANY
	// not-all-confirmed batch — a permission wall or a single failed/un-acked window
	// leaves it in multi-select mode with no landing. open's burst NEVER returns a
	// partial-failure error and ALWAYS connects the trigger to its own target:
	// external failures — and even a permission wall on an external window — do not
	// cost the trigger its landing (RESOLVED 2026-07-18). Only the trigger's OWN
	// connect failure (connectTrigger below) surfaces as the command's error.
	// triggerAttached is therefore always true in the batch summary: the trigger is
	// unconditionally about to connect, so the summary is emitted at this
	// just-before-connect point.
	confirmed, failed := spawn.PartitionResults(results)
	total := len(surfaces) // N, including the trigger's own self-connect target.
	if perm, ok := spawn.FirstPermission(results); ok {
		// Permission-required stopped the burst on the first (source, target) wall
		// (every later external window would hit the identical wall). Take the shared
		// permission arm — LogWindowResults + LogPermission, NO batch summary (the
		// picker's emitPermission takes the same path) — and surface the driver's
		// guidance ONCE, best-effort.
		spawn.LogWindowResults(deps.Logger, results)
		spawn.LogPermission(deps.Logger, id, resolution, perm.Result.Detail)
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), perm.Result.Guidance)
	} else if len(failed) > 0 {
		// Leave-what-opened: the opened (confirmed) windows stay; the failed/un-acked
		// surfaces are neither auto-retried nor torn down. One batch summary + a
		// best-effort one-line stderr summary naming the failed window(s).
		spawn.LogBatchSummary(deps.Logger, id, resolution, results, total, true, batch)
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), spawn.PartialFailureMessage(failed, len(confirmed) > 0))
	} else {
		// Full success: every external window confirmed. One batch summary.
		spawn.LogBatchSummary(deps.Logger, id, resolution, results, total, true, batch)
	}

	// Self-connect the trigger LAST, after every external window has been spawned —
	// regardless of the external outcomes reported above. The batch summary above
	// optimistically counted the trigger's self-attach in `opened` because it MUST
	// be emitted before this connect (a successful outside-tmux attach exec-replaces
	// the process and never returns to emit it). Only the rare connect-failure path
	// falls through here, so emit a corrective WARN naming the trigger that did NOT
	// attach — the durable portal.log must not be left claiming an `opened N/N` that
	// counts a trigger which never landed.
	if err := connectTrigger(cmd, trigger, command, deps); err != nil {
		spawn.LogTriggerConnectFailed(deps.Logger, trigger.Value, err.Error())
		return err
	}
	return nil
}

// hasMintSurface reports whether any surface in the set is a mint (a fresh session
// at a directory) — the surface kind a mint-scoped command can run in. Used by the
// multi-target zero-mint command guard.
func hasMintSurface(surfaces []spawn.Surface) bool {
	for _, s := range surfaces {
		if s.Kind == spawn.SurfaceMint {
			return true
		}
	}
	return false
}

// connectTrigger self-connects the trigger surface in the invoking terminal: a
// MINT trigger mints a fresh session locally at the resolved literal dir, threading
// the mint-scoped command (openPath's create-or-attach — the same path a spawned
// mint window takes); an ATTACH trigger routes through the inside/outside-tmux
// Connector (switch-client inside / exec attach outside). It is the SOLE site the
// inside/outside split selects the trigger's connector; the N−1 external windows
// always run the spawned out-of-tmux `open … --ack` argv.
func connectTrigger(cmd *cobra.Command, trigger spawn.Surface, command []string, deps *OpenBurstDeps) error {
	if trigger.Kind == spawn.SurfaceMint {
		return deps.LocalMint(cmd, trigger.Value, command)
	}
	return deps.Connector.Connect(trigger.Value)
}
