package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
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
	Dir     string
	Version string
	Logger  *slog.Logger
	Client  *tmux.Client

	// HookStore is built once at daemon startup via loadHookStore(); it MUST
	// resolve the same hooks.json foreground commands mutate (relies on the
	// daemon inheriting the same PORTAL_HOOKS_FILE / XDG_CONFIG_HOME env — the
	// same env-inheritance rule the state daemon already depends on for
	// PORTAL_STATE_DIR). It is the *hooks.Store the daemon-owned hooks
	// stale-cleanup gate (tasks 3-2/3-3) drives via runHookStaleCleanup; the
	// lister for that call is the existing Client (*tmux.Client satisfies
	// AllPaneLister via ListAllPanes) — no new client, no new seam.
	HookStore *hooks.Store

	// lastCleanup is the throttle anchor for the daemon-owned hooks
	// stale-cleanup gate (tasks 3-2/3-3); initialised to the daemon-START time
	// so the first cleanup fires one interval (~10s) after start, not on the
	// first idle tick (~1s).
	lastCleanup time.Time

	HashMap      state.HashMap
	PrevIndex    *state.Index
	LastSaveAt   time.Time
	TickerPeriod time.Duration
	MaxGap       time.Duration

	// shutdownSignal holds the OS signal that triggered shutdown, recorded by
	// the RunE signal goroutine BEFORE it cancels the run context and read by
	// defaultShutdownFlush AFTER ctx.Done(). The store-then-cancel /
	// cancel-then-read ordering makes the access race-free on its own; the
	// atomic.Pointer is belt-and-braces so -race stays clean regardless. A nil
	// pointer means no signal was recorded (a non-signal ctx cancel — tests, or
	// an edge-path shutdown) and maps to reason="exit".
	shutdownSignal atomic.Pointer[os.Signal]
}

// recordShutdownSignal stores the signal that triggered shutdown so
// shutdownReason can later classify it. Called by the RunE signal goroutine
// before it cancels the run context.
func (d *daemonDeps) recordShutdownSignal(sig os.Signal) {
	d.shutdownSignal.Store(&sig)
}

// shutdownReason maps the recorded shutdown signal to the closed
// daemon-shutdown reason value space {sighup, signal, exit}:
//
//	SIGHUP  → "sighup"
//	SIGTERM → "signal"
//	none    → "exit"  (no signal recorded — a non-signal ctx cancel)
func (d *daemonDeps) shutdownReason() string {
	p := d.shutdownSignal.Load()
	if p == nil {
		return "exit"
	}
	switch *p {
	case syscall.SIGHUP:
		return "sighup"
	case syscall.SIGTERM:
		return "signal"
	default:
		return "exit"
	}
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

// osExit is the package-level seam over os.Exit. Two marked-termination call
// sites route through it: the Component D daemon self-eject (the spec's single
// sanctioned bare-exit exception) and the hydrate helper's exec-failure
// fall-through (defaultExecShell — Task 8-3). Both pair a terminal marker
// (self-eject INFO / exec-failure WARN) plus log.Close(N) BEFORE the exit, so
// neither vanishes unmarked. Tests swap this var to a recorder so the test
// process is not actually terminated when either path fires.
//
// Production callers leave it at os.Exit. Direct use of os.Exit anywhere in
// this package is forbidden — always go through osExit so the exit paths stay
// observable in tests (and so "no bare os.Exit outside main" holds).
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
//  2. list-panes -t =_portal-saver -F '#{pane_pid}' (via
//     tmux.SaverPanePIDOrAbsent — the shared helper that centralizes the
//     "ErrNoSuchSession / ErrEmptyPaneList → absent" sentinel collapse for
//     both this probe and bootstrap step 4's orphan-sweep adapter). Per the
//     spec's "treat any error as absent" rule the probe broadens further:
//     a non-nil err return (e.g., ErrPanePIDParse, a generic exec failure)
//     is also treated as "absent". The helper's (pid, present, err) shape
//     is collapsed here into a single bool.
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
	pid, present, err := tmux.SaverPanePIDOrAbsent(c, tmux.PortalSaverName)
	if err != nil || !present {
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
//
// Harness: cmd/state_daemon_hysteresis_measurement_test.go (build tag
// `integration`, re-runnable to verify the safety-factor invariant;
// assertion fires loudly on regression). The harness file header
// documents the scenario-2 `refresh-client` substitution rationale.
const selfSupervisionHysteresisTicks = 3

// hookCleanupInterval is the throttle interval for the daemon-owned hooks
// stale-cleanup gate; the 1s tick stays light (capture/scrollback save is the
// priority and can exceed 1s) while stale hooks are inert so precise timing is
// irrelevant. Tuning detail — 10s default.
const hookCleanupInterval = 10 * time.Second

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
// On reaching selfSupervisionHysteresisTicks, the daemon emits the cataloged
// "self-eject" INFO, then log.Close(0) (the paired "process: exit code=0"
// terminal marker), then calls osExit(0) directly. The eject bypasses
// daemonShutdownFunc / defaultShutdownFlush intentionally:
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
	// Startup write sequence (load-bearing order):
	//   1. acquireDaemonLock     — singleton flock (spec § Component C step 4)
	//   2. WritePIDFile          — must be the immediately-next statement after
	//                              the acquire err-guard (AST-pinned by
	//                              TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire)
	//   3. WriteVersionFile      — recorded post-pidfile so EnsurePortalSaverVersion's
	//                              version-mismatch detection has a fresh marker; the
	//                              AST adjacency invariant only forbids work BETWEEN
	//                              acquire and pidfile-write, so versionfile-write is
	//                              permitted (and required) AFTER the pidfile if-stmt.
	lockFile, err := acquireDaemonLock(deps.Dir)
	if err != nil {
		if errors.Is(err, state.ErrDaemonLockHeld) {
			deps.Logger.Warn("another daemon holds the lock; exiting")
			return nil
		}
		deps.Logger.Warn("acquire daemon lock failed", "error", err)
		return fmt.Errorf("acquire daemon lock: %w", err)
	}
	if err := state.WritePIDFile(deps.Dir, os.Getpid()); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	if err := state.WriteVersionFile(deps.Dir, deps.Version, deps.Logger); err != nil {
		return fmt.Errorf("write version file: %w", err)
	}
	daemonLockFile = lockFile

	// Additive subsystem milestone (spec § Saver and daemon lifecycle event
	// taxonomy — daemon "lock acquired"). The OS-process-boundary marker is
	// "process: start process_role=daemon" (Phase 2); this line carries the
	// orphaned tmux_pane attr that the dropped redundant "daemon: spawn" event
	// would have held. pid is the auto-injected baseline — not passed here.
	deps.Logger.Info("lock acquired", "tmux_pane", os.Getenv("TMUX_PANE"))

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
					// Hysteresis trip — cataloged daemon "self-eject"
					// lifecycle event (spec § Saver and daemon lifecycle
					// event taxonomy). ticks = consecutive-absence count at
					// the trip; threshold = the configured ejection threshold.
					deps.Logger.Info("self-eject", "ticks", consecutiveAbsenceTicks, "threshold", selfSupervisionHysteresisTicks)
					// Load-bearing terminal-marker pairing (spec § Defensive
					// invariants — sanctioned exception): emit the self-eject
					// INFO, THEN log.Close(0) (which emits "process: exit
					// code=0" and does NOT itself call os.Exit), THEN osExit(0).
					// Without log.Close(0) the direct osExit would leave an
					// unpaired "process: start". Order MUST NOT be reordered and
					// log.Close(0) MUST NOT be skipped.
					log.Close(0)
					osExit(0)
					// Unreachable in production (osExit terminates the process).
					// Retained so the osExit-stubbed-to-no-op test seam falls
					// through cleanly without running tick on the eject tick.
					return nil
				}
				// Below the threshold: one DEBUG breadcrumb per failing probe
				// (spec § Log-level discipline — hysteresis-internal failures
				// are DEBUG). No INFO until the trip above.
				deps.Logger.Debug("saver-membership probe failed", "ticks", consecutiveAbsenceTicks, "threshold", selfSupervisionHysteresisTicks)
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
//  2. !dirty && !gap is the idle fast path — after the no-op stat, run the
//     throttled daemon-owned hooks stale-cleanup gate (maybeRunHookCleanup;
//     ~10s throttle) then return. Cleanup lives HERE — on the idle branch,
//     after the @portal-restoring check — so it fires on a mostly-idle warm
//     server; placing it after the capture branch would gate it behind capture
//     work and it would never run on an idle server. It is skipped entirely
//     while @portal-restoring is set (whole tick skipped) and on capture-pending
//     ticks (dirty||gap -> capture runs, cleanup skipped; scrollback always
//     wins).
//  3. captureAndCommit failures leave LastSaveAt and save.requested untouched
//     so the next tick retries.
func tick(ctx context.Context, deps *daemonDeps) {
	restoring, err := state.IsRestoringSet(deps.Client)
	if err != nil {
		deps.Logger.Warn("read @portal-restoring failed", "error", err)
		return
	}
	if restoring {
		return
	}

	dirty := fileExists(state.SaveRequested(deps.Dir))
	gap := time.Since(deps.LastSaveAt) >= deps.MaxGap
	if !dirty && !gap {
		maybeRunHookCleanup(deps)
		return
	}

	if err := captureAndCommit(ctx, deps); err != nil {
		deps.Logger.Warn("tick failed", "error", err)
		return
	}

	deps.LastSaveAt = time.Now()

	if err := os.Remove(state.SaveRequested(deps.Dir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		deps.Logger.Warn("remove save.requested failed", "error", err)
	}
}

// maybeRunHookCleanup is the throttled gate for the daemon-owned hooks
// stale-cleanup (spec § Daemon-Owned Hooks Cleanup → Operational contract).
// Below the throttle interval it is a pure no-op (no cleanup call, lastCleanup
// untouched); once time.Since(deps.lastCleanup) >= hookCleanupInterval it
// invokes the shared runHookStaleCleanup helper verbatim with the four pinned
// arguments — lister=deps.Client, store=deps.HookStore, swallowListError=true,
// onRemoved=nil — reusing that helper's mass-deletion hazard guard and its
// EmitCleanStaleSummary audit breadcrumb (no new audit event is introduced).
//
// deps.lastCleanup is reset AFTER the cleanup body runs (whether it succeeded or
// errored), so a failing cleanup still advances the throttle and retries next
// cadence rather than hammering the store every tick. A cleanup error is logged
// WARN and swallowed (mirroring the tick loop's "tick failed" handling) — the
// gate never returns an error and never crashes the daemon.
//
// Task 3-3 places this on the tick's idle branch; here it is standalone.
func maybeRunHookCleanup(deps *daemonDeps) {
	if time.Since(deps.lastCleanup) < hookCleanupInterval {
		return
	}
	if err := runHookStaleCleanup(deps.Client, deps.HookStore, deps.Logger, true, nil); err != nil {
		deps.Logger.Warn("hooks stale-cleanup failed", "error", err)
	}
	deps.lastCleanup = time.Now()
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
	// Drift-mirror: cmd/bootstrap/daemon_tick_test_helpers_test.go runDaemonTick
	// shadows this body byte-for-byte under the integration build tag for AC4
	// coverage. Mirror any structural change here in that helper or AC4 may pass
	// under a broken production tick.

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

	// start anchors the tick-complete cycle summary's took attr. Captured
	// after obs-point-1's ctx check so a cancellation that arrives before any
	// capture work returns nil without arming a summary.
	start := time.Now()

	skipSet, err := state.ListSkeletonMarkers(deps.Client)
	if err != nil {
		return fmt.Errorf("list markers: %w", err)
	}

	idx, err := state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex, deps.Logger)
	if err != nil {
		return fmt.Errorf("capture structure: %w", err)
	}

	// Cycle-summary counters (spec § Cycle-level summary cadence and shape).
	// sessions is the structural session count; panes counts every PROCESSED
	// pane (skipSet entries are excluded); naturalChurn counts panes that
	// vanished mid-tick by normal action (a user closing a pane/session —
	// distinguished from an anomalous failure via the tmux pane-vanished
	// signal); anomalous counts genuine capture/write failures that did not
	// terminate the cycle (each also emits a per-pane WARN).
	sessions := len(idx.Sessions)
	var panes, naturalChurn, anomalous int

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
				// Processed pane: count it and drop a capture-component DEBUG
				// breadcrumb (silent at INFO, the summary is the INFO truth).
				panes++
				captureLogger.Debug("pane captured", "pane_key", paneKey, "session", sess.Name)
				target := tmux.PaneTarget(sess.Name, win.Index, pane.Index)
				data, hash, err := state.CaptureAndHashPane(deps.Client, target)
				if err != nil {
					// A pane/session the index still references can vanish
					// mid-tick because the user closed it — the expected,
					// normal action. CapturePane surfaces that as a
					// tmux "can't find {session,window,pane}" *CommandError
					// (it does not sentinel-wrap as ErrNoSuchSession). Classify
					// it as natural_churn with a DEBUG (error_class=expected),
					// NOT a WARN — the close is not an anomaly.
					if isPaneVanishedError(err) {
						naturalChurn++
						captureLogger.Debug("pane vanished", "pane_key", paneKey, "error_class", "expected")
						continue
					}
					anomalous++
					deps.Logger.Warn("capture pane failed", "pane_key", target, "error", err)
					continue
				}
				written, err := state.WriteScrollbackIfChanged(deps.Dir, paneKey, data, hash, deps.HashMap)
				if err != nil {
					// Write failures are disk-write faults (AtomicWrite0600),
					// never a vanished pane, so they are always anomalous.
					anomalous++
					deps.Logger.Warn("write scrollback failed", "pane_key", paneKey, "error", err)
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

	// Tick-complete cycle summary (spec § Cycle-level summary cadence and
	// shape, daemon-tick row). Emitted ONLY on the successful post-Commit
	// return — never on a ctx-cancel obs point or a phase-boundary error
	// return — so exactly one INFO summary fires per tick that did capture
	// work.
	captureLogger.Info("tick complete",
		"sessions", sessions,
		"panes", panes,
		"natural_churn", naturalChurn,
		"anomalous", anomalous,
		log.Took(start),
	)
	return nil
}

// isPaneVanishedError reports whether err signals an expected mid-tick
// pane/session disappearance (the user closed a pane or session while the tick
// was capturing) rather than a genuine capture failure. CapturePane does NOT
// sentinel-wrap this case as ErrNoSuchSession, so the live signal is the
// underlying *tmux.CommandError's stderr carrying tmux's "can't find
// {session,window,pane}" phrasing (verified against tmux 3.6b). The
// errors.Is(ErrNoSuchSession) check is a defensive belt-and-braces for any
// chain that did wrap the sentinel; the *CommandError stderr inspection is the
// load-bearing classifier.
func isPaneVanishedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, tmux.ErrNoSuchSession) {
		return true
	}
	var cmdErr *tmux.CommandError
	if errors.As(err, &cmdErr) && strings.Contains(strings.ToLower(cmdErr.Stderr), "can't find ") {
		return true
	}
	return false
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
	// Each return path emits exactly one "shutdown" lifecycle INFO (spec
	// § Saver and daemon lifecycle event taxonomy — daemon "shutdown") with the
	// captured reason and whether the final commit completed. The pre-existing
	// "skipping final flush" / "final flush" lines are demoted to DEBUG
	// breadcrumbs — the INFO truth is the single shutdown line.
	restoring, err := state.IsRestoringSet(deps.Client)
	if err != nil {
		deps.Logger.Warn("read @portal-restoring at shutdown failed; skipping final flush", "error", err)
		deps.Logger.Info("shutdown", "reason", deps.shutdownReason(), "flush_completed", false)
		return nil
	}
	if restoring {
		deps.Logger.Debug("skipping final flush: @portal-restoring set")
		deps.Logger.Info("shutdown", "reason", deps.shutdownReason(), "flush_completed", false)
		return nil
	}
	deps.Logger.Debug("final flush")
	// shutdown flush is non-cancellable — the cancelled context is what
	// triggered the flush; passing it through would abort the very save we
	// are exiting to perform.
	flushErr := captureAndCommit(context.Background(), deps)
	if flushErr != nil {
		deps.Logger.Warn("final flush failed", "error", flushErr)
	}
	deps.Logger.Info("shutdown", "reason", deps.shutdownReason(), "flush_completed", flushErr == nil)
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

		logger := daemonLogger

		// Daemon startup is observable via "process: start process_role=daemon"
		// (emitted by log.Init in main) plus the cataloged "daemon: lock
		// acquired" — see spec § Saver and daemon lifecycle event taxonomy →
		// Process/subsystem boundary. The previously-emitted "daemon: starting"
		// INFO was a redundant subsystem milestone (same instant, same data) and
		// has been dropped.

		// Defensive dirty-flag clear: a stale save.requested from a crashed or
		// version-mismatch-restarted daemon must not trigger an immediate save
		// during the restore window. See spec § Defensive Dirty-Flag Clear on
		// Daemon Startup.
		_ = os.Remove(state.SaveRequested(dir))

		// Singleton-lock acquisition, WritePIDFile, and WriteVersionFile have
		// all moved into defaultDaemonRun (spec § Component C step 4 — adjacency
		// invariant is enforced by an AST-walking test against the run-func body).
		// daemonShutdownFunc / final-flush sequencing is unaffected: the
		// run func still returns via daemonShutdownFunc on ctx-cancel, and
		// the lock-held / lock-error paths short-circuit before reaching
		// the ticker loop.

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
			logger.Warn("ReadIndex failed", "error", err)
		}

		// Build the hooks store once, from the SAME resolver foreground commands
		// use (loadHookStore → hooksFilePath → configFilePath("PORTAL_HOOKS_FILE",
		// "hooks.json")), so the daemon-owned stale-cleanup gate (tasks 3-2/3-3)
		// cleans the identical hooks.json the user edits. A resolution failure
		// surfaces here rather than silently leaving the daemon with a nil store
		// (which would silently disable cleanup for the whole daemon lifetime).
		hookStore, err := loadHookStore()
		if err != nil {
			return fmt.Errorf("load hook store: %w", err)
		}

		client := tmux.DefaultClient()
		deps := &daemonDeps{
			Dir:     dir,
			Version: version,
			Logger:  logger,
			Client:  client,
			// lastCleanup is anchored to daemon-START time so the first hooks
			// stale-cleanup (tasks 3-2/3-3) fires one interval after start.
			HookStore:    hookStore,
			lastCleanup:  time.Now(),
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
			// Record the arriving signal BEFORE cancelling so
			// defaultShutdownFlush (which reads only after ctx.Done()) can
			// classify the shutdown reason. The store-then-cancel ordering is
			// what makes the read race-free; the atomic.Pointer is the -race
			// belt-and-braces.
			sig := <-sigCh
			deps.recordShutdownSignal(sig)
			cancel()
		}()

		return daemonRunFunc(ctx, deps)
	},
}

func init() {
	stateCmd.AddCommand(stateDaemonCmd)
}
