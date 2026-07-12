package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/spawn"
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
	// classification. Defaults to spawn.ResolveAdapter.
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

// runSpawn is the spawn burst: detect the host terminal, resolve its adapter,
// open the N−1 external windows sequentially in list order, and — only if every
// external spawn succeeds — self-attach the calling window to the Nth session
// (net-N windows, never N+1). On any external failure it skips the self-attach
// and returns a plain error naming the failed session (main maps it to exit 1);
// the opaque Result.Detail goes to the log, never the user-facing message.
func runSpawn(cmd *cobra.Command, args []string, deps *SpawnDeps) error {
	sessions := args
	n := len(sessions)
	// Split by the list-order convention: the trailing session is the trigger
	// the calling window self-attaches to; the rest are externally spawned.
	external := sessions[:n-1]
	trigger := sessions[n-1]

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
		logSpawnGone(log.OrDiscard(deps.Logger), gone)
		return fmt.Errorf("spawn: %s %s gone — nothing opened", spawn.QuoteJoin(gone), spawn.GoneVerb(len(gone)))
	}

	// Order is load-bearing: detect first, then resolve the adapter.
	id := deps.Detector.Detect()
	adapter, resolution := deps.Resolve(id)

	// Atomic no-op gate: an N≥2 batch (at least one external window) on an
	// unsupported/NULL terminal cannot open its external windows — they need an
	// adapter that isn't available. Refuse before touching any adapter so
	// nothing opens and nothing self-attaches. N=1 (empty external set) skips
	// this gate and self-attaches below: a single attach needs no adapter.
	if len(external) >= 1 && resolution == spawn.ResolutionUnsupported {
		logSpawnUnsupported(log.OrDiscard(deps.Logger), id)
		return errors.New(unsupportedSpawnMessage(id))
	}

	outcomes, err := spawn.SpawnWindows(adapter, external, deps.ExePath, deps.Getenv)
	if err != nil {
		// Executable resolution failed before any window opened; exit 1.
		return err
	}

	logger := log.OrDiscard(deps.Logger)
	opened, failedSession := tallyOutcomes(logger, outcomes)

	if failedSession != "" {
		logSpawnSummary(logger, id, resolution, opened, n)
		return fmt.Errorf("spawn: failed to open window for %q", failedSession)
	}

	// Every external window opened (or there were none): the trigger self-attach
	// is about to occur, so count it before the connector self-execs away (the
	// outside-tmux path exec-replaces the process and never returns).
	opened++
	logSpawnSummary(logger, id, resolution, opened, n)
	return deps.Connector.Connect(trigger)
}

// tallyOutcomes emits one DEBUG per external window (session + opaque detail)
// and returns the count of successful external spawns plus the first failed
// session name (empty when all succeeded).
func tallyOutcomes(logger *slog.Logger, outcomes []spawn.SpawnOutcome) (opened int, failedSession string) {
	for _, o := range outcomes {
		logger.Debug("external window", "session", o.Session, "detail", o.Result.Detail)
		if o.Result.OK() {
			opened++
			continue
		}
		if failedSession == "" {
			failedSession = o.Session
		}
	}
	return opened, failedSession
}

// unsupportedSpawnMessage composes the one-line user-facing message for the
// N≥2 atomic no-op, naming the detected identity. A NULL identity (remote/mosh
// / no host-local client) gets the honest no-host-local line; a recognised-but-
// undriven identity names its friendly name and bundle id, separated by the
// U+00B7 middle dot that mirrors the --detect echo and the design banner.
func unsupportedSpawnMessage(id spawn.Identity) string {
	if id.IsNull() {
		return "spawn: no host-local terminal — nothing opened"
	}
	return fmt.Sprintf("spawn: unsupported terminal — %s · %s — nothing opened", id.Name, id.BundleID)
}

// logSpawnGone emits the single INFO outcome line for a pre-flight abort. The
// message names the gone session(s); nothing was attempted (detect never ran),
// so it carries no per-window records and no resolution/opened/total attrs.
func logSpawnGone(logger *slog.Logger, gone []string) {
	logger.Info(fmt.Sprintf("%s %s gone — nothing opened", spawn.QuoteJoin(gone), spawn.GoneVerb(len(gone))))
}

// logSpawnUnsupported emits the single INFO outcome line for the atomic no-op.
// Nothing was attempted, so it carries only the closed resolution/terminal/
// bundle_id attrs — no per-window records and no opened/total counts.
func logSpawnUnsupported(logger *slog.Logger, id spawn.Identity) {
	logger.Info("unsupported terminal — nothing opened",
		"resolution", string(spawn.ResolutionUnsupported),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
	)
}

// logSpawnSummary emits the single INFO cycle summary. total is N (all sessions
// including the trigger's self-attach target); opened counts each successful
// external spawn plus the trigger's self-attach when it occurs.
func logSpawnSummary(logger *slog.Logger, id spawn.Identity, resolution spawn.Resolution, opened, total int) {
	logger.Info(fmt.Sprintf("opened %d/%d", opened, total),
		"resolution", string(resolution),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
		"opened", opened,
		"total", total,
	)
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

// buildSpawnDeps returns a fully-populated SpawnDeps for the spawn pipeline:
// injected fields (from spawnDeps in tests) are kept, and every unset field
// falls back to its production default. The tmux-client-backed defaults
// (Detector, Connector) resolve the shared client only when actually needed, so
// this never panics for a caller that injected those seams.
func buildSpawnDeps(cmd *cobra.Command) *SpawnDeps {
	deps := &SpawnDeps{}
	if spawnDeps != nil {
		*deps = *spawnDeps
	}
	if deps.Detector == nil {
		deps.Detector = spawnDetector(cmd)
	}
	if deps.Resolve == nil {
		deps.Resolve = spawn.ResolveAdapter
	}
	if deps.Connector == nil {
		deps.Connector = buildSessionConnector(tmuxClient(cmd))
	}
	if deps.ExePath == nil {
		deps.ExePath = os.Executable
	}
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if deps.Exists == nil {
		deps.Exists = tmuxClient(cmd).HasSession
	}
	if deps.Logger == nil {
		deps.Logger = spawnLogger
	}
	return deps
}

func init() {
	spawnCmd.Flags().Bool("detect", false, "print the detected host terminal identity and exit without opening anything")
	spawnCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return NewUsageError(err.Error())
	})
	rootCmd.AddCommand(spawnCmd)
}
