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

// acquireDaemonLock is the test seam over state.AcquireDaemonLock. Tests in
// this package swap it to simulate the contention path (ErrDaemonLockHeld)
// and the non-EWOULDBLOCK error path without contending for a real OS lock.
// Production callers leave it at the default.
var acquireDaemonLock = state.AcquireDaemonLock

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

// defaultDaemonRun is the production daemon body: a 1-second ticker that fires
// captures when the dirty flag is set or the 30-second max-gap has elapsed,
// returning to delegate the final flush to daemonShutdownFunc on ctx-cancel.
//
// Per-tick errors are logged and swallowed — the loop never aborts on a
// transient failure (disk full, tmux glitch). See spec § Failure Modes
// → Disk full during save and § In-Flight Capture Atomicity.
func defaultDaemonRun(ctx context.Context, deps *daemonDeps) error {
	ticker := time.NewTicker(deps.TickerPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
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

	idx, err := state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex)
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

		// Singleton-lock acquisition MUST precede any state-directory write.
		// Order matters: EnsureDir → save.requested clear → acquire lock →
		// WritePIDFile / WriteVersionFile / tick loop. See spec § Fix Part 1:
		// Daemon-Side Singleton Lock.
		//
		//  - Success: retain the *os.File in daemonLockFile so the kernel-held
		//    flock survives the lifetime of the process.
		//  - ErrDaemonLockHeld: another daemon won; log one WARN line and exit
		//    status 0 by returning nil. No pidfile / version write, no tick
		//    loop. Quiet co-existence with the winner.
		//  - Other errors (open(2) failure, unexpected flock error): log ERROR
		//    and return a wrapped error so the daemon exits non-zero.
		lockFile, err := acquireDaemonLock(dir)
		if err != nil {
			if errors.Is(err, state.ErrDaemonLockHeld) {
				logger.Warn(state.ComponentDaemon, "another daemon holds the lock; exiting")
				return nil
			}
			logger.Error(state.ComponentDaemon, "acquire daemon lock: %v", err)
			return fmt.Errorf("acquire daemon lock: %w", err)
		}
		daemonLockFile = lockFile

		if err := state.WritePIDFile(dir, os.Getpid()); err != nil {
			return fmt.Errorf("write PID file: %w", err)
		}
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
