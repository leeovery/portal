package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// daemonDeps bundles the inputs the daemon's run loop needs to operate.
// Held in one struct so the test seam (daemonRunFunc) can capture and assert
// against everything the RunE prepared on its behalf.
//
// HashMap and PrevIndex are mutable across ticks and updated by the loop —
// HashMap by WriteScrollbackIfChanged, PrevIndex by captureAndCommit.
// LastSaveAt is updated by tick when a capture-and-commit succeeds.
type daemonDeps struct {
	Dir          string
	Logger       *state.Logger
	Client       *tmux.Client
	HashMap      state.HashMap
	PrevIndex    *state.Index
	LastSaveAt   time.Time
	TickerPeriod time.Duration
	MaxGap       time.Duration
}

// daemonRunFunc and daemonShutdownFunc are package-level seams. Tests replace
// them via t.Cleanup so the unit-level tests never block on signals or run
// the production tick loop.
//
// Production callers leave them at the defaults below.
var (
	daemonRunFunc      = defaultDaemonRun
	daemonShutdownFunc = defaultShutdownFlush
)

// daemonTickLoopFunc is a sub-seam invoked by defaultDaemonRun AFTER the
// lock-acquire + pidfile ceremony. Tests swap this seam to drive the
// post-startup behavior (capture deps, short-circuit) without bypassing the
// acquire+pid block at the head of defaultDaemonRun. Production callers
// leave it at defaultDaemonTickLoop.
var daemonTickLoopFunc = defaultDaemonTickLoop

// acquireDaemonLock is the test seam over state.AcquireDaemonLock. Tests in
// this package swap it to simulate the contention path (ErrDaemonLockHeld)
// and the non-EWOULDBLOCK error path without contending for a real OS lock.
// Production callers leave it at the default.
var acquireDaemonLock = state.AcquireDaemonLock

// osExit is the package-level seam over os.Exit, used exclusively by the
// Component D self-eject path. Tests swap this var to a recorder so the test
// process is not actually terminated when the eject fires.
//
// Production callers leave it at os.Exit. Direct use of os.Exit anywhere in
// this package is forbidden — always go through osExit so the eject path is
// observable in tests.
var osExit = os.Exit

// saverMembershipProbe is the package-level seam consumed by Component D's
// per-tick saver-membership self-check (integrated into the tick loop by
// Task 5-3). Tests swap this var to inject deterministic
// absent/present/mismatch sequences without spinning up a real tmux server.
// Production callers leave it at defaultSaverMembershipProbe.
//
// Signature: (c *tmux.Client, selfPID int) → bool. true means "this daemon
// is bound to the live saver pane"; false means "absent" — which the spec
// explicitly broadens to cover every failure mode (HasSession returning
// false, SaverPanePID returning any error, pid mismatch). The Component D
// hysteresis counter increments on every false and resets on every true.
var saverMembershipProbe = defaultSaverMembershipProbe

// defaultSaverMembershipProbe is the production implementation of the
// saverMembershipProbe seam. It executes the spec § Component D self-check
// sequence steps 1–3:
//
//  1. has-session -t =_portal-saver. Any falsy / errored result → "absent".
//  2. list-panes -t =_portal-saver -F '#{pane_pid}' (via tmux.SaverPanePID).
//     Any error (including ErrNoSuchSession from the HasSession→list-panes
//     race, ErrEmptyPaneList, ErrPanePIDParse, or a generic exec failure) →
//     "absent". Per the spec's "treat any error as absent" rule the probe
//     never tries to discriminate further than "non-nil error".
//  3. Compare the returned pid to selfPID. Match → true ("legitimate
//     daemon"); mismatch → false ("orphan daemon — another process owns
//     the saver pane").
//
// The probe is intentionally pure observation — it never writes to tmux,
// never writes to the state directory, never logs. Logging and the
// self-eject decision live in the tick-loop integration (Task 5-3).
func defaultSaverMembershipProbe(c *tmux.Client, selfPID int) bool {
	if !c.HasSession(tmux.PortalSaverName) {
		return false
	}
	pid, err := tmux.SaverPanePID(c, tmux.PortalSaverName)
	if err != nil {
		return false
	}
	return pid == selfPID
}

// daemonLockFile retains the daemon's advisory-lock fd for the entire lifetime
// of the process. Storing the *os.File in a package-level var prevents Go's
// finalizer from closing the fd (which would silently release the kernel-side
// flock and re-introduce the very race the lock exists to close). See spec
// § Fix Part 1: "Fd retention is load-bearing."
//
// MUST NOT be wrapped in any value with a finalizer that closes the fd.
// MUST NOT have runtime.SetFinalizer attached. The lock is released by the
// kernel on process exit; no explicit close is required or desired.
var daemonLockFile *os.File

// selfSupervisionHysteresisTicks is the number of consecutive ticks
// the daemon must observe a failing saver-membership probe before
// self-ejecting (Component D, spec § Component D — Daemon
// Self-Supervision Against the Saver Session).
//
// Scenarios measured (real tmux 3.6b, real `portal state daemon`
// subprocess on darwin/arm64, 5 runs each, sampled at 1s TickerPeriod):
//
//  1. Steady-state (no interaction, 30s window)       max=0 ticks
//  2. Attach/detach cycles (15s window)               max=0 ticks
//  3. client-attached hook fires (15s window)         max=0 ticks
//  4. Bootstrap kill-and-recreate (10s window)        max=0 ticks
//
// max-observed × 2 safety factor                       → 0 ticks
// Clamped to [3, 9]                                    → 3 ticks
// upstream-defect flag (max×2 > 5)                     → false
//
// Measured 2026-05-23, binary version "dev" (from `go build .`).
// Memo path (relative to repo root):
//
//	.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/component-d-hysteresis-measurement.md
//
// Harness: cmd/state_daemon_hysteresis_measurement_test.go (build tag
// `integration`, re-runnable to verify the safety-factor invariant;
// assertion fires loudly on regression).
const selfSupervisionHysteresisTicks = 3

// defaultDaemonRun is the production daemon body: a 1-second ticker that fires
// captures when the dirty flag is set or the 30-second max-gap has elapsed,
// returning to delegate the final flush to daemonShutdownFunc on ctx-cancel.
//
// Singleton-lock + pidfile ceremony runs at the head of this function (spec
// § Component C step 4 — Post-acquire daemon.pid write). The acquireDaemonLock
// call, its err-guard, and the state.WritePIDFile call (plus its err-guard)
// MUST appear as immediately consecutive statements: no other production work
// is permitted between them. TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire
// walks this function's AST to enforce that adjacency invariant.
//
// Per-tick errors are logged and swallowed — the loop never aborts on a
// transient failure (disk full, tmux glitch). See spec § Failure Modes
// → Disk full during save and § In-Flight Capture Atomicity.
//
// Component D — Saver-membership self-supervision (spec § Component D
// self-check sequence steps 4-5): on every ticker fire, before tick runs,
// invoke saverMembershipProbe. False (any failure mode — absent saver, pid
// mismatch, transient tmux error) increments a consecutive-absence counter.
// True resets the counter to 0 (NOT decrement — the spec mandates reset, so
// any single successful probe re-establishes legitimacy).
//
// The self-check lives in the ticker for-loop, NOT inside tick. Placing it
// inside tick would route it through IsRestoringSet's early-return — and the
// spec is explicit that a divergent-view daemon must self-eject regardless
// of @portal-restoring state. Restoring is a legitimate condition for
// suppressing capture work; it is NOT a license to ignore membership
// divergence. Keeping the check above tick guarantees it observes every
// tick uniformly.
//
// On reaching selfSupervisionHysteresisTicks, the daemon logs INFO and calls
// osExit(0) directly. The eject bypasses daemonShutdownFunc / defaultShutdownFlush
// intentionally:
//
//   - Same reasoning as Component A's straight-to-SIGKILL choice: a daemon
//     whose view diverges from tmux's view must NOT execute one more
//     captureAndCommit / gcOrphanScrollback cycle on its way out.
//   - daemon.pid is left on disk on purpose. Phase 4 Component C's pre-check
//     on the next AcquireDaemonLock detects the stale value (the recorded PID
//     is dead) and proceeds. Deleting daemon.pid here would be racy against
//     a concurrent pre-check and would invert the layered-enforcement
//     contract — Component C is the authoritative cleanup site.
func defaultDaemonRun(ctx context.Context, deps *daemonDeps) error {
	lockFile, err := acquireDaemonLock(deps.Dir)
	if err != nil {
		if errors.Is(err, state.ErrDaemonLockHeld) {
			if deps.Logger != nil {
				deps.Logger.Warn(state.ComponentDaemon, "another daemon holds the lock; exiting")
			}
			return nil
		}
		if deps.Logger != nil {
			deps.Logger.Error(state.ComponentDaemon, "acquire daemon lock: %v", err)
		}
		return fmt.Errorf("acquire daemon lock: %w", err)
	}
	if err := state.WritePIDFile(deps.Dir, os.Getpid()); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	daemonLockFile = lockFile

	return daemonTickLoopFunc(ctx, deps)
}

// defaultDaemonTickLoop is the production tick loop body: a ticker that fires
// captures (via tick) and runs the saver-membership self-check (Component D)
// on every fire. Extracted from defaultDaemonRun so tests can short-circuit
// the loop without bypassing the acquire+pid ceremony at the head of
// defaultDaemonRun.
func defaultDaemonTickLoop(ctx context.Context, deps *daemonDeps) error {
	ticker := time.NewTicker(deps.TickerPeriod)
	defer ticker.Stop()
	// consecutiveAbsenceTicks is closure-scoped: it lives for the lifetime
	// of this daemon process only and is reset to zero on every probe-true.
	var consecutiveAbsenceTicks int
	for {
		select {
		case <-ticker.C:
			if saverMembershipProbe(deps.Client, os.Getpid()) {
				consecutiveAbsenceTicks = 0
			} else {
				consecutiveAbsenceTicks++
				if consecutiveAbsenceTicks >= selfSupervisionHysteresisTicks {
					deps.Logger.Info(state.ComponentDaemon,
						"self-supervision: saver-membership lost for %d consecutive ticks, exiting",
						consecutiveAbsenceTicks)
					osExit(0)
					return nil
				}
			}
			tick(ctx, deps)
		case <-ctx.Done():
			return daemonShutdownFunc(deps)
		}
	}
}

// tick runs one iteration of the save loop. It never returns an error: errors
// are logged at WARN and the next tick retries. See spec § Save-Side
// Architecture → Daemon Tick Loop.
//
// The order of checks matters:
//  1. @portal-restoring suppresses the entire tick (incl. clearing the dirty
//     flag) so a save.requested touch during restore survives until restore
//     completes.
//  2. !dirty && !gap is the no-op fast path (per-tick idle cost is one stat).
//  3. captureAndCommit failures leave LastSaveAt and save.requested untouched
//     so the next tick retries.
func tick(ctx context.Context, deps *daemonDeps) {
	restoring, err := state.IsRestoringSet(deps.Client)
	if err != nil {
		deps.Logger.Warn(state.ComponentDaemon, "read @portal-restoring: %v", err)
		return
	}
	if restoring {
		return
	}

	dirty := fileExists(state.SaveRequested(deps.Dir))
	gap := time.Since(deps.LastSaveAt) >= deps.MaxGap
	if !dirty && !gap {
		return
	}

	if err := captureAndCommit(ctx, deps); err != nil {
		deps.Logger.Warn(state.ComponentDaemon, "tick: %v", err)
		return
	}

	deps.LastSaveAt = time.Now()

	if err := os.Remove(state.SaveRequested(deps.Dir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		deps.Logger.Warn(state.ComponentDaemon, "remove save.requested: %v", err)
	}
}

// captureAndCommit runs a full save cycle: list skeleton markers, capture the
// structural index (merging skeleton-marked panes from the prior index),
// capture and dedup-write per-pane scrollback, then atomically commit
// sessions.json. On success, deps.PrevIndex is replaced with the fresh index
// so subsequent merges have an up-to-date baseline.
//
// Per-pane errors (capture-pane fail, write fail) are logged and the cycle
// continues — one bad pane must not abort the rest of the save. Errors at the
// "phase boundary" steps (list markers, capture structure, commit) propagate
// to the caller so tick can log and back off.
func captureAndCommit(ctx context.Context, deps *daemonDeps) error {
	// observation point 1 of 3: pre-enumeration; ensures a cancellation that
	// arrives between ticker fire and tick entry returns immediately without
	// any tmux work or commit. See spec § Change 2.
	//
	// Return nil (not an error) — tick logs WARN on non-nil return, and a
	// cancellation must not produce a log line.
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	skipSet, err := state.ListSkeletonMarkers(deps.Client)
	if err != nil {
		return fmt.Errorf("list markers: %w", err)
	}

	idx, err := state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex, deps.Logger)
	if err != nil {
		return fmt.Errorf("capture structure: %w", err)
	}

	// observation point 2 of 3: post-enumeration, pre-first-iteration; covers
	// cancellation during the CaptureStructure subprocess call. Returns before
	// any per-pane work or Commit invocation. See spec § Change 2.
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	anyScrollbackChanged := false
	for _, sess := range idx.Sessions {
		for _, win := range sess.Windows {
			for _, pane := range win.Panes {
				// observation point 3 of 3: between per-pane iterations; caps
				// worst-case exit latency at one pane's capture-pane wall time.
				// Returns before this iteration's CaptureAndHashPane. Per-pane
				// scrollback writes from prior iterations in this cycle are not
				// rolled back — they are atomic, and the spec's no-partial-commit
				// invariant is about sessions.json, not per-pane files. See spec
				// § Change 2.
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				paneKey := state.SanitizePaneKey(sess.Name, win.Index, pane.Index)
				if _, skipped := skipSet[paneKey]; skipped {
					continue
				}
				target := tmux.PaneTarget(sess.Name, win.Index, pane.Index)
				data, hash, err := state.CaptureAndHashPane(deps.Client, target)
				if err != nil {
					deps.Logger.Warn(state.ComponentDaemon, "capture pane %s: %v", target, err)
					continue
				}
				written, err := state.WriteScrollbackIfChanged(deps.Dir, paneKey, data, hash, deps.HashMap)
				if err != nil {
					deps.Logger.Warn(state.ComponentDaemon, "write scrollback %s: %v", paneKey, err)
					continue
				}
				if written {
					anyScrollbackChanged = true
				}
			}
		}
	}

	if err := state.Commit(deps.Dir, idx, anyScrollbackChanged, deps.Logger); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	deps.PrevIndex = &idx
	return nil
}

// defaultShutdownFlush implements the SIGHUP/SIGTERM final-flush handler from
// the spec's Save-Side Architecture → Signal Handling section.
//
// If @portal-restoring is set we skip the flush — a half-restored topology
// must not become the committed snapshot. If we cannot determine the marker
// state (tmux call errored), we conservatively skip too: a stale snapshot is
// preferable to a guaranteed-bad one.
//
// Final-flush capture errors are logged but swallowed: the daemon is exiting
// anyway and the prior on-disk save remains valid (per AtomicWrite atomicity).
func defaultShutdownFlush(deps *daemonDeps) error {
	restoring, err := state.IsRestoringSet(deps.Client)
	if err != nil {
		deps.Logger.Warn(state.ComponentDaemon, "read @portal-restoring at shutdown: %v; skipping final flush", err)
		return nil
	}
	if restoring {
		deps.Logger.Info(state.ComponentDaemon, "skipping final flush: @portal-restoring set")
		return nil
	}
	deps.Logger.Info(state.ComponentDaemon, "final flush")
	// shutdown flush is non-cancellable — the cancelled context is what
	// triggered the flush; passing it through would abort the very save we
	// are exiting to perform.
	if err := captureAndCommit(context.Background(), deps); err != nil {
		deps.Logger.Warn(state.ComponentDaemon, "final flush: %v", err)
	}
	return nil
}

// fileExists reports whether path resolves to an existing filesystem entry.
// Used as a hot-path stat for the daemon's per-tick dirty-flag check.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stateDaemonCmd is the long-running save daemon hosted in the
// _portal-saver tmux session. Hidden from --help; invoked internally by tmux
// via "new-session -d -s _portal-saver 'portal state daemon'".
//
// RunE wires up the state directory, log file, PID/version markers, the
// per-pane hash map seed, the prior on-disk index (for skeleton merge), and a
// signal-cancelled context, then delegates the run loop to daemonRunFunc.
var stateDaemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the Portal save daemon (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := state.EnsureDir()
		if err != nil {
			return fmt.Errorf("ensure state dir: %w", err)
		}

		logger, err := state.OpenLogger(state.PortalLog(dir), true)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer func() { _ = logger.Close() }()

		logger.Info(state.ComponentDaemon, "starting, version=%s, pid=%d", version, os.Getpid())

		// Defensive dirty-flag clear: a stale save.requested from a crashed or
		// version-mismatch-restarted daemon must not trigger an immediate save
		// during the restore window. See spec § Defensive Dirty-Flag Clear on
		// Daemon Startup.
		_ = os.Remove(state.SaveRequested(dir))

		// Singleton-lock acquisition + WritePIDFile have moved into
		// defaultDaemonRun (spec § Component C step 4 — adjacency invariant
		// is enforced by an AST-walking test against the run-func body).
		// daemonShutdownFunc / final-flush sequencing is unaffected: the
		// run func still returns via daemonShutdownFunc on ctx-cancel, and
		// the lock-held / lock-error paths short-circuit before reaching
		// the ticker loop.
		if err := state.WriteVersionFile(dir, version, logger); err != nil {
			return fmt.Errorf("write version file: %w", err)
		}

		// Seed the per-pane content-hash map from any existing scrollback
		// files so the first cycle dedupes against what is already on disk.
		hm := state.SeedHashMap(dir, logger)

		// Load the prior structural index so skeleton-marked panes can be
		// merged from authoritative pre-boot state during the first capture.
		// state.ReadIndex shares its corrupt-vs-missing classification with
		// the restore path (internal/restore/restore.go): a clean miss maps
		// to prevIdx=nil with no log entry, while a read or decode failure
		// maps to prevIdx=nil with a WARN that surfaces ErrCorruptIndex when
		// applicable.
		var prevIdx *state.Index
		if idx, skip, err := state.ReadIndex(dir); !skip {
			prevIdx = &idx
		} else if err != nil {
			logger.Warn(state.ComponentDaemon, "ReadIndex: %v", err)
		}

		client := tmux.DefaultClient()
		deps := &daemonDeps{
			Dir:          dir,
			Logger:       logger,
			Client:       client,
			HashMap:      hm,
			PrevIndex:    prevIdx,
			TickerPeriod: 1 * time.Second,
			MaxGap:       30 * time.Second,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		return daemonRunFunc(ctx, deps)
	},
}

func init() {
	stateCmd.AddCommand(stateDaemonCmd)
}
