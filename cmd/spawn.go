package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// spawnLogger binds the closed "spawn" log component once for the whole spawn
// command (introduced in Phase 1). The pipeline emits its cycle summary and
// per-window detail through it.
var spawnLogger = log.For("spawn")

// TerminalDetector resolves the host terminal's identity for the spawn
// command's --detect dry-run. It is the seam that lets the command body be
// Executed with a fabricated detector — no real tmux, ps, or defaults reads.
type TerminalDetector interface {
	Detect() spawn.Identity
}

// spawnDeps holds injectable dependencies for the spawn command. When nil,
// real implementations are used.
var spawnDeps *SpawnDeps

// SpawnDeps allows injecting dependencies for testing. Every field defaults to
// its production implementation when unset (see buildSpawnDeps), so a test can
// override exactly the seams it needs and drive the whole pipeline without a
// real tmux server, osascript, or process handoff.
type SpawnDeps struct {
	// Detector resolves the host terminal identity (--detect and the pipeline's
	// detect step). Defaults to the production process-tree detector.
	Detector TerminalDetector
	// Resolve maps an identity to its window-opening adapter plus the resolution
	// classification. Defaults to buildResolver().Resolve — the config-aware
	// resolver loaded from terminals.json (config → native → unsupported).
	Resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)
	// Connector performs the single self-attach to the Nth session. Defaults to
	// buildSessionConnector, which branches on tmux.InsideTmux().
	Connector SessionConnector
	// ExePath resolves the picker's own binary for attach-command composition.
	// Defaults to os.Executable.
	ExePath spawn.ExecutableResolver
	// Getenv reads the environment (PATH) for attach-command composition.
	// Defaults to os.Getenv.
	Getenv func(string) string
	// Exists probes whether a session still exists for the pre-flight gate.
	// Defaults to tmuxClient(cmd).HasSession, which folds any tmux probe error
	// to false — so an unprobeable session is conservatively treated as gone.
	Exists func(name string) bool
	// Ack is the token-ack channel (Collect + Clean) the N≥2 gate uses: the
	// burster polls Collect to confirm each spawned window, and runSpawn sweeps
	// the batch markers via Clean before the self-attach exec handoff. Defaults
	// to a spawn.NewServerOptionAckChannel over the shared tmux client.
	Ack spawn.AckChannelFull
	// NewBurster constructs the burst orchestrator for the N≥2 path from the
	// resolved adapter. It is the seam that lets a test inject a fake ack channel
	// + fake clock. Defaults to a production spawn.Burster (spawn.NewBurster).
	NewBurster func(adapter spawn.Adapter) *spawn.Burster
	// Logger receives the cycle summary and per-window detail. Defaults to the
	// package-level spawnLogger.
	Logger *slog.Logger
}

var spawnCmd = &cobra.Command{
	Use:   "spawn [sessions...]",
	Short: "Detect the host terminal (--detect) or open sessions in host-local windows",
	// SilenceUsage/SilenceErrors keep cobra from printing its own usage/error
	// text; main's classify owns exit codes and stderr. The FlagErrorFunc
	// bridges cobra's flag-parse errors (e.g. an unknown flag) into a
	// *cmd.UsageError so they exit 2 like the empty-invocation usage gate.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		detect, err := cmd.Flags().GetBool("detect")
		if err != nil {
			return err
		}

		if detect {
			id := spawnDetector(cmd).Detect()
			if id.IsNull() {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no host-local terminal detected")
				return err
			}
			// "Name · BundleID" echoes the design separator, e.g.
			// "Apple Terminal · com.apple.Terminal".
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s · %s\n", id.Name, id.BundleID)
			return err
		}

		if len(args) == 0 {
			return NewUsageError("spawn: provide one or more sessions, or use --detect")
		}

		return runSpawn(cmd, args, buildSpawnDeps(cmd))
	},
}

// runSpawn is the spawn burst: pre-flight the whole batch, then — for N≥2 —
// detect the host terminal, resolve its adapter, open the N−1 external windows
// sequentially, confirm each via its token ack, and only when EVERY external
// window confirms self-attach the calling window to the Nth session (net-N
// windows, never N+1), cleaning the batch markers first. N=1 (no external
// windows) self-attaches immediately with no ack wait. On any not-all-confirmed
// batch it leaves the opened windows in place (no teardown), skips the
// self-attach, and returns a plain error naming every failed window (an adapter
// spawn-failed or an ack timeout, unified); the opaque Result.Detail goes to the
// log, never the user message.
func runSpawn(cmd *cobra.Command, args []string, deps *SpawnDeps) error {
	sessions := args
	n := len(sessions)
	// Split by the list-order convention: the trailing session is the trigger
	// the calling window self-attaches to; the rest are externally spawned. The
	// split is derived through spawn.SplitNetN — the single computation the picker
	// (dispatchBurst) also uses — so the "net N, never N+1" invariant can't drift
	// between the two callers. Safe here: the empty-args usage gate guarantees
	// n >= 1 before this point.
	external, trigger := spawn.SplitNetN(sessions)

	// Pre-flight has-session gate (spec: pre-flight + all-or-nothing). Probe
	// EVERY selected session — the external windows AND the trigger's self-attach
	// target — before touching detect/resolve/spawn/self-attach. If any is gone,
	// abort atomically: nothing opens, no self-attach. Runs FIRST — ahead of the
	// N≥2 unsupported gate — so a gone session aborts with the more-actionable
	// gone-session message even on an unsupported terminal (both exit 1). Runs
	// for all N: an N=1 batch whose sole session is gone aborts here. Exists is
	// HasSession, which folds a probe fault to false → gone → conservative abort.
	// A plain (non-UsageError, non-silenced) error → exit 1 on stderr.
	if gone := spawn.PreflightMissing(sessions, deps.Exists); len(gone) > 0 {
		logSpawnGone(deps.Logger, gone)
		return fmt.Errorf("spawn: %s", spawn.GoneMessage(gone))
	}

	// N=1 (empty external set): no external windows to spawn or confirm — a plain
	// single attach. Self-attach immediately, no detector/adapter/burster/ack
	// wait needed (spec's N=0/N=1 boundary: "no special-casing" beyond a plain
	// attach).
	if len(external) == 0 {
		return deps.Connector.Connect(trigger)
	}

	// N≥2. Order is load-bearing: detect first, then resolve the adapter.
	id := deps.Detector.Detect()
	adapter, resolution := deps.Resolve(id)

	// Atomic no-op gate: an N≥2 batch on an unsupported/NULL terminal cannot open
	// its external windows — they need an adapter that isn't available. Refuse
	// before touching any adapter so nothing opens and nothing self-attaches.
	if resolution == spawn.ResolutionUnsupported {
		logSpawnUnsupported(deps.Logger, id)
		return errors.New(unsupportedSpawnMessage(id))
	}

	batch, results, err := deps.NewBurster(adapter).Run(context.Background(), external, nil)
	if err != nil {
		// Executable or ack-id resolution failed before any window opened; exit 1.
		return err
	}
	// Clean the batch markers on every post-burst path (success or failure), and
	// — critically — BEFORE the self-attach exec handoff (a point of no return).
	// Best-effort: bounded, harmless leaks self-expire with the tmux server.
	_ = deps.Ack.Clean(batch)

	logger := deps.Logger
	// The confirmed/failed partition drives both the branch decision and the
	// leave-what-opened error message; it derives from the shared count-semantics
	// chokepoint so this path and the picker's cannot drift. A window is "failed"
	// when it is not Confirmed() — unifying an adapter spawn-failed (AckFailed) and
	// an ack timeout (AckTimeout). The batch is all-confirmed exactly when failed is
	// empty. The opened count for the summary is derived inside logSpawnSummary from
	// the same chokepoint.
	_, failed := spawn.PartitionResults(results)

	if len(failed) > 0 {
		// Permission-required is the burst-stop and takes precedence over the
		// generic not-all-confirmed branch below. Its window is also a failed
		// window (result.OK() is false → AckFailed), so it lands here; checking it
		// FIRST means the permission case surfaces the driver's guidance ONCE and
		// never double-reports as the generic failed-window line. Earlier-opened
		// windows are left in place (no teardown), the trigger self-attach is
		// skipped, and the batch markers were already Cleaned above. General code
		// switches on Outcome alone: the opaque Result.Detail (never an AppleEvent
		// number this layer interpreted) rides up only as the log detail attr.
		if perm, ok := spawn.FirstPermission(results); ok {
			// The CLI emits the per-window DEBUG detail before the permission INFO
			// (the picker's permission arm deliberately does not — that asymmetry is
			// preserved per caller). The batch summary is skipped: the burst stopped,
			// so there is no cycle summary to report.
			spawn.LogWindowResults(logger, results)
			logSpawnPermission(logger, id, resolution, perm.Result.Detail)
			return errors.New(perm.Result.Guidance)
		}

		// Leave-what-opened: a post-pre-flight per-window hiccup (an adapter
		// spawn-failed or an ack timeout — both a non-confirmed window) leaves every
		// opened window in place. Portal does not own the host windows and has no
		// teardown path, so there is nothing to close; the trigger self-attach is
		// simply skipped (the trigger stays in its calling context, never self-execs).
		// The batch markers were already Cleaned above, on every post-burst path. The
		// opaque Result.Detail for each failure went only to the DEBUG log (the summary
		// emits its per-window loop), never the user-facing message below.
		logSpawnSummary(logger, id, resolution, results, n, false, batch)
		return fmt.Errorf("spawn: %s", spawn.PartialFailureMessage(failed))
	}

	// Every external window confirmed: the trigger self-attach is about to occur, so
	// it is counted (triggerAttached=true) before the connector self-execs away (the
	// outside-tmux path exec-replaces the process and never returns).
	logSpawnSummary(logger, id, resolution, results, n, true, batch)
	return deps.Connector.Connect(trigger)
}

// unsupportedSpawnMessage composes the one-line user-facing message for the
// N≥2 atomic no-op, naming the detected identity. A NULL identity (remote/mosh
// / no host-local client) gets the honest no-host-local line; a recognised-but-
// undriven identity names its friendly name and bundle id, separated by the
// U+00B7 middle dot that mirrors the --detect echo and the design banner.
func unsupportedSpawnMessage(id spawn.Identity) string {
	return "spawn: " + spawn.UnsupportedNoopMessage(id)
}

// logSpawnGone emits the pre-flight abort outcome line via the shared
// spawn.LogGone renderer (the closed message lives in internal/spawn, not here).
func logSpawnGone(logger *slog.Logger, gone []string) {
	spawn.LogGone(logger, gone)
}

// logSpawnUnsupported emits the atomic-no-op outcome line via the shared
// spawn.LogUnsupported renderer.
func logSpawnUnsupported(logger *slog.Logger, id spawn.Identity) {
	spawn.LogUnsupported(logger, id)
}

// logSpawnPermission emits the permission-required outcome line via the shared
// spawn.LogPermission renderer.
func logSpawnPermission(logger *slog.Logger, id spawn.Identity, resolution spawn.Resolution, detail string) {
	spawn.LogPermission(logger, id, resolution, detail)
}

// logSpawnSummary emits the batch cycle summary via the shared spawn.LogBatchSummary
// renderer: the per-window DEBUG loop plus one INFO `opened <opened>/<total>`. total
// is N (all sessions including the trigger self-attach target); opened is derived
// inside the shared helper from spawn.PartitionResults (confirmed windows) plus the
// trigger self-attach when triggerAttached. batch is the burst's ack batch id.
func logSpawnSummary(logger *slog.Logger, id spawn.Identity, resolution spawn.Resolution, results []spawn.WindowResult, total int, triggerAttached bool, batch string) {
	spawn.LogBatchSummary(logger, id, resolution, results, total, triggerAttached, batch)
}

// spawnDetector resolves the host-terminal detector, preferring an injected one
// and otherwise building the production detector against the shared tmux client.
// It is the single detector-resolution authority: the --detect dry-run uses it
// directly (needing no other spawn deps), and buildSpawnDeps defaults its
// Detector field through it.
func spawnDetector(cmd *cobra.Command) TerminalDetector {
	if spawnDeps != nil && spawnDeps.Detector != nil {
		return spawnDeps.Detector
	}
	return spawn.NewDetector(tmuxClient(cmd))
}

// productionSpawnSeams bundles the shared production spawn dependencies that
// both the spawn CLI (buildSpawnDeps) and the picker (openTUI's tuiConfig
// population) wire from the same *tmux.Client. Constructing them in one place
// keeps the two paths from silently diverging: because SpawnDeps and tuiConfig
// are distinct struct shapes, the compiler cannot catch a seam that is added,
// swapped, or re-constructed on only one side — this bundle is the single
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
// construction site the CLI and picker both read, so their production wiring
// cannot drift.
func buildProductionSpawnSeams(client *tmux.Client) productionSpawnSeams {
	return productionSpawnSeams{
		Detector: spawn.NewDetector(client),
		Resolve:  buildResolver().Resolve,
		Ack:      spawn.NewServerOptionAckChannel(client, client),
		Exe:      os.Executable,
		Getenv:   os.Getenv,
		Exists:   client.HasSession,
		Logger:   log.For("spawn"),
	}
}

// buildSpawnDeps returns a fully-populated SpawnDeps for the spawn pipeline:
// injected fields (from spawnDeps in tests) are kept, and every unset field
// falls back to its production default. The six shared production seams (Resolve,
// Ack, ExePath, Getenv, Exists, Logger) come from the single
// buildProductionSpawnSeams bundle that the picker also reads. That bundle is
// built at most once and only when a shared field actually needs defaulting, so
// a fully-injected caller (the spawn pipeline suite) never resolves the tmux
// client (there is none in context under nopRunner) nor loads terminals.json —
// exactly as before. The Detector default deliberately routes through
// spawnDetector — the standalone --detect authority — so its resolution stays
// byte-for-byte unchanged, and Connector / the lazy NewBurster remain CLI-only.
func buildSpawnDeps(cmd *cobra.Command) *SpawnDeps {
	deps := &SpawnDeps{}
	if spawnDeps != nil {
		*deps = *spawnDeps
	}

	// Lazily memoise the shared production seams: consulted only for genuinely
	// unset fields (the injected-field precedence above always wins), and built
	// at most once so buildResolver / NewServerOptionAckChannel run no more often
	// than they did as inline defaults.
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
	if deps.Exists == nil {
		deps.Exists = sharedSeams().Exists
	}
	if deps.Ack == nil {
		deps.Ack = sharedSeams().Ack
	}
	if deps.NewBurster == nil {
		// Lazy closure: reads the (now-defaulted) Ack/ExePath/Getenv at burst
		// time, so it never re-resolves the tmux client here and composes the
		// same production burster the N≥2 path drives.
		deps.NewBurster = func(adapter spawn.Adapter) *spawn.Burster {
			return spawn.NewBurster(adapter, deps.Ack, deps.ExePath, deps.Getenv)
		}
	}
	if deps.Logger == nil {
		deps.Logger = sharedSeams().Logger
	}
	return deps
}

// buildResolver constructs the config-aware host-terminal adapter resolver: it
// resolves the terminals.json path through the XDG configFilePath chain, loads
// the escape-hatch config once via TerminalsStore, and wraps it in a
// spawn.Resolver (config override → native → unsupported).
//
// It FAILS SAFE: an undeterminable home/XDG path (a rare configFilePath error)
// degrades to an EMPTY config — native-only resolution — rather than aborting the
// spawn command, so a broken environment never disables the whole feature.
// TerminalsStore.Load is itself tolerant (missing/unreadable/malformed →
// empty config), so this reads terminals.json without ever crashing the pipeline.
func buildResolver() *spawn.Resolver {
	cfg := spawn.TerminalsConfig{}
	if path, err := configFilePath("PORTAL_TERMINALS_FILE", "terminals.json"); err == nil {
		cfg = spawn.NewTerminalsStore(path).Load()
	}
	return spawn.NewResolver(cfg)
}

func init() {
	spawnCmd.Flags().Bool("detect", false, "print the detected host terminal identity and exit without opening anything")
	spawnCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return NewUsageError(err.Error())
	})
	rootCmd.AddCommand(spawnCmd)
}
