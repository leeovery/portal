package tmux

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// saverLogger is the component=saver sink for saver-lifecycle breadcrumbs that
// must render under the saver component regardless of the bootstrap-component
// WARN sink threaded through saver.Barrier.Logger. log.For never returns nil
// (root is constructed at package init), so calls are safe at any level and
// against a discard sink. Today its sole consumer is the SIGKILL-escalation
// DEBUG breadcrumb in escalateKillToSIGKILL.
var saverLogger = log.For("saver")

// discardLogger is the canonical silent *slog.Logger used as the default sink
// for the saver-side barrier and version-writer seams when production wiring
// has not installed a real logger. It writes to io.Discard so calls are safe
// no-ops without a nil-pointer risk.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// PortalSaverName is the tmux session name that hosts the long-running save daemon.
// The leading underscore marks the session as Portal-internal: Client.ListSessions
// applies a chokepoint underscore-prefix filter so this session is excluded from
// every user-facing listing (the TUI picker, `portal list`, and any future
// ListSessions consumer). It is also excluded from sessions.json capture by the
// separate keepSessionNames pass in internal/state/capture.go.
const PortalSaverName = "_portal-saver"

// PortalBootstrapName is the tmux session name used by Client.StartServer when
// it creates the initial detached session that keeps the freshly-started tmux
// server alive long enough for Portal's bootstrap Restore step to run. The
// leading underscore marks the session as Portal-internal: Client.ListSessions
// applies a chokepoint underscore-prefix filter so this session is excluded
// from every user-facing listing (the TUI picker, `portal list`, and any
// future ListSessions consumer). The constant is the canonical reference —
// other code MUST NOT create or re-use a session with this name.
const PortalBootstrapName = "_portal-bootstrap"

// portalSaverPlaceholderCommand is the shell command run as the saver
// session's initial pane process at session-creation time, before
// destroy-unattached=off has been applied. It is intentionally inert:
//
//  1. `exec tail -f /dev/null` blocks indefinitely on both macOS and Linux
//     without consuming CPU — `tail` waits on inotify/kqueue for a file that
//     never changes.
//  2. The more idiomatic-looking `sleep infinity` is NOT used because the
//     BSD `sleep(1)` shipped with macOS rejects the literal "infinity" with a
//     parse error. `tail -f /dev/null` is portable across both platforms with
//     no behavioural difference.
//  3. The placeholder is structurally incapable of writing to the state
//     directory or contending for the daemon lock — it never opens
//     `daemon.lock`, never writes `daemon.pid`, never writes
//     `daemon.version`. This is the whole point of running it before the
//     real daemon: Component F applies destroy-unattached=off to the
//     placeholder session, then `respawn-pane -k` swaps in the daemon, so
//     the daemon never sees a session that could auto-destroy out from under
//     it.
//  4. The placeholder process lives until either `respawn-pane -k`
//     terminates it (the normal Component F handoff) or `tmux kill-session`
//     tears down `_portal-saver` outright. tmux does not deliver SIGHUP to
//     this process when the server dies in any way that requires graceful
//     shutdown — the placeholder has no state to flush.
const portalSaverPlaceholderCommand = "sh -c 'exec tail -f /dev/null'"

// portalSaverDaemonCommand is the real saver pane process — the long-running
// `portal state daemon` invocation that owns sessions.json capture, FIFO
// sweeps, and the daemon lock. It is installed into the saver pane by
// `respawn-pane -k` after destroy-unattached=off has been applied to the
// session by Component F's reorder; running it as the initial pane process
// (the pre-Component-F shape) would race the destroy-unattached default
// against the daemon's startup. tmux owns the daemon's lifecycle: when this
// session is killed (or the server dies), the kernel delivers SIGHUP to the
// daemon for graceful shutdown.
const portalSaverDaemonCommand = "portal state daemon"

// BootstrapAliveCheck is the function used to test whether a daemon is alive
// for a given state directory. It is a package-level seam so tests can stub
// the check without writing real PID files. Defaults to state.DaemonAlive.
var BootstrapAliveCheck = state.DaemonAlive

// PortalSaverRetryDelay is the sleep between new-session retry attempts. It is
// exported as a var so tests can shrink it. Defaults to 100ms.
var PortalSaverRetryDelay = 100 * time.Millisecond

// portalSaverMaxAttempts is the maximum number of new-session attempts before
// BootstrapPortalSaver gives up and returns an error.
const portalSaverMaxAttempts = 3

// SaverBarrierSeams groups the kill-barrier-specific seams driving
// killSaverAndWaitForDaemon's poll loop and escalation path, plus the
// shared WARN-emission Logger sink also consumed by the readiness barrier
// (waitForSaverDaemonReady). Embedded under SaverSeams.Barrier.
//
//   - IsAlive: probe of whether the prior daemon is still running. Tests can
//     simulate "alive then dead after N ticks" / "alive forever" / "already
//     dead" without spawning real processes. Defaults to state.IsProcessAlive.
//     Kill-barrier scope only.
//   - SendSIGKILL: escalation-path signal emission. Routing through the seam
//     makes the "no SIGTERM ever sent" invariant trivially verifiable — only
//     SIGKILL flows through this seam. Kill-barrier scope only.
//   - PollInterval: cadence at which the kill barrier re-probes IsAlive after
//     issuing kill-session. Defaults to 50ms — chosen alongside Timeout so
//     the median recycle path completes in well under a second. Kill-barrier
//     scope only.
//   - Timeout: upper bound on the kill barrier's wait for the prior daemon
//     to exit. Sized to sit above the daemon's cold-sweep ceiling (3.9s on
//     the affected user's scrollback profile) with margin, so the WARN path
//     is reserved for genuinely stuck daemons rather than ordinary cold
//     sweeps. Kill-barrier scope only.
//   - EscalationTimeout: upper bound on the post-SIGKILL poll window. Sized
//     to a single second — SIGKILL is unblockable, so the process should be
//     reaped almost immediately on any healthy system; the budget exists
//     only to surface persistently-undead processes via a single WARN.
//     Kill-barrier scope only.
//   - Logger: shared sink for BOTH the kill-barrier WARN-on-timeout /
//     escalation paths AND the readiness-barrier WARN-on-timeout path
//     (waitForSaverDaemonReady). Production wiring replaces this with the
//     bootstrap component's *slog.Logger via SetBarrierLogger. Tests install
//     a capturing slog handler via log.SetTestHandler. Defaults to the
//     io.Discard-backed discardLogger so it is never nil.
type SaverBarrierSeams struct {
	IsAlive           func(int) bool
	SendSIGKILL       func(int) error
	PollInterval      time.Duration
	Timeout           time.Duration
	EscalationTimeout time.Duration
	Logger            *slog.Logger
}

// SaverReadinessSeams groups the readiness-barrier-specific seams driving
// waitForSaverDaemonReady's poll loop. PID-read and identity-classification
// primitives are shared with the kill barrier and live at the top level of
// SaverSeams. Embedded under SaverSeams.Readiness.
//
//   - PollInterval: cadence at which the readiness barrier re-probes
//     ReadPIDFile + IdentifyDaemon. Defaults to 50ms.
//   - Timeout: upper bound on the wait for the freshly respawned daemon to
//     publish daemon.pid AND be identifiable as `portal state daemon`. Sized
//     to cover normal daemon startup latency (fork + exec + flock + PID-file
//     write) with margin while keeping the bootstrap step bounded.
type SaverReadinessSeams struct {
	PollInterval time.Duration
	Timeout      time.Duration
}

// SaverVersionSeams groups the version-file read/write seams plus the
// DEBUG-breadcrumb logger consulted by the bootstrap-side defensive
// write. Embedded under SaverSeams.Version.
//
//   - ReadVersionFile: function used by EnsurePortalSaverVersion to read the
//     stored daemon.version. Tests can simulate read errors (including
//     non-absent I/O failures) without touching the filesystem. Defaults to
//     state.ReadVersionFile.
//   - WriteVersionFile: function used on the alive+absent branch to
//     defensively write daemon.version before falling through to
//     BootstrapPortalSaver. The default wrapper forwards WriterLogger to
//     state.WriteVersionFile so the bootstrap-side defensive write emits the
//     same "daemon.version write" DEBUG breadcrumb as the daemon-startup
//     call site. The default sink is the io.Discard-backed discardLogger so
//     it is never nil.
//   - WriterLogger: sink for the "daemon.version write" DEBUG breadcrumb.
//     Production wiring installs the daemon component's *slog.Logger via
//     SetVersionWriterLogger from internal/bootstrapadapter at the same site
//     that calls SetBarrierLogger.
//
// Wiring-order invariant: callers that fire WriteVersionFile before bootstrap
// step 2 (RegisterPortalHooks) has run write through the discard default, so
// the breadcrumb routes to io.Discard. Today the only production producer is
// EnsurePortalSaverVersion (bootstrap step 5), which always runs after step 2
// installs the logger — so the breadcrumb is reliable.
type SaverVersionSeams struct {
	ReadVersionFile  func(string) (string, error)
	WriteVersionFile func(dir, version string) error
	WriterLogger     *slog.Logger
}

// SaverOperationSeams groups the two operation-level function seams that
// callers route through to substitute the entire kill-and-wait or
// readiness-wait flows. Embedded under SaverSeams.Ops. Tests that do not
// care about the inner barrier mechanics swap these with a no-op or
// recorder and skip the full poll-loop wall time; tests that exercise the
// real loops leave these as the production helpers and stub the inner
// primitives instead.
//
//   - WaitForReady: routed through by BootstrapPortalSaver's create-branch.
//   - KillAndWait: routed through by both production kill call sites
//     (BootstrapPortalSaver session-lingering-with-dead-daemon path and
//     EnsurePortalSaverVersion's kill decision).
type SaverOperationSeams struct {
	WaitForReady func(string) error
	KillAndWait  func(*Client, string) error
}

// SaverSeams collapses every saver-side mutable seam into a single struct
// so production references and test swaps share one home rather than five
// parallel package-level vars. The shared PID-read and daemon-identity
// primitives sit at the top level (consumed by BOTH the kill barrier and
// the readiness barrier); barrier-specific, readiness-specific,
// version-specific, and operation-level seams live in named sub-structs.
//
// Top-level shared fields:
//
//   - ReadPID: function used to read the saver daemon's PID file. Defaults
//     to state.ReadPIDFile. Tests can simulate "no PID file", "corrupted PID
//     file", "unreadable PID file", and "absent then present" cases without
//     touching the filesystem.
//   - IdentifyDaemon: function used to classify whether a given PID is a
//     live `portal state daemon`. Defaults to state.IdentifyDaemon — the
//     shared three-result primitive. Tests can deterministically drive the
//     IdentifyIsPortalDaemon / IdentifyNotPortalDaemon / IdentifyDead /
//     transient-error branches without shelling out to ps.
type SaverSeams struct {
	ReadPID        func(string) (int, error)
	IdentifyDaemon func(int) (state.IdentifyResult, error)

	Barrier   SaverBarrierSeams
	Readiness SaverReadinessSeams
	Version   SaverVersionSeams
	Ops       SaverOperationSeams
}

// saver is the single package-level seam-struct instance. Production code
// references its fields directly; tests swap fields (via swapSeam) or
// whole sub-clusters via the *Seams() accessors in export_test.go.
//
// Defaults below mirror the prior bare-var initial values.
var saver = SaverSeams{
	ReadPID:        state.ReadPIDFile,
	IdentifyDaemon: state.IdentifyDaemon,

	Barrier: SaverBarrierSeams{
		IsAlive: state.IsProcessAlive,
		SendSIGKILL: func(pid int) error {
			return syscall.Kill(pid, syscall.SIGKILL)
		},
		PollInterval:      50 * time.Millisecond,
		Timeout:           5 * time.Second,
		EscalationTimeout: 1 * time.Second,
		Logger:            discardLogger,
	},

	Readiness: SaverReadinessSeams{
		PollInterval: 50 * time.Millisecond,
		Timeout:      2 * time.Second,
	},

	Version: SaverVersionSeams{
		ReadVersionFile: state.ReadVersionFile,
		// WriteVersionFile defaults are wired in init() below to break the
		// initialization cycle (the wrapper closes over saver itself to
		// read the current Version.WriterLogger sink at call time).
		WriterLogger: discardLogger,
	},

	// Ops defaults (WaitForReady, KillAndWait) wired in init() below;
	// composite literal cannot reference function symbols declared later
	// without a forward declaration, and pinning them here keeps the
	// initialization site adjacent to the WriteVersionFile wrapper.
}

func init() {
	// Wire the default WriteVersionFile wrapper and Ops defaults outside the
	// saver composite literal to avoid an initialization cycle. The
	// WriteVersionFile closure captures the package-level saver so it always
	// reads the current Version.WriterLogger sink (including any
	// SetVersionWriterLogger installation) at call time.
	saver.Version.WriteVersionFile = func(dir, version string) error {
		return state.WriteVersionFile(dir, version, saver.Version.WriterLogger)
	}
	saver.Ops.WaitForReady = waitForSaverDaemonReady
	saver.Ops.KillAndWait = killSaverAndWaitForDaemon
}

// SetBarrierLogger installs a *slog.Logger as the shared sink for BOTH
// saver-side barriers' WARN emissions: killSaverAndWaitForDaemon's
// WARN-on-timeout / escalation paths AND waitForSaverDaemonReady's
// WARN-on-readiness-timeout path. A nil argument is ignored so the package
// never loses its sink to a programming error in the wiring layer; the
// default is the io.Discard-backed discardLogger which already swallows all
// calls safely.
//
// Production wiring calls this once from internal/bootstrapadapter as part
// of constructing the HookRegistrar, threading the bootstrap component's
// *slog.Logger that the rest of bootstrap uses.
func SetBarrierLogger(l *slog.Logger) {
	if l == nil {
		return
	}
	saver.Barrier.Logger = l
}

// SetVersionWriterLogger installs a *slog.Logger as the sink for the
// "daemon.version write" DEBUG breadcrumb emitted by the bootstrap-side
// defensive WriteVersionFile call. A nil argument is ignored so the package
// never loses its sink to a programming error in the wiring layer; the
// default is the io.Discard-backed discardLogger.
//
// Production wiring calls this once from internal/bootstrapadapter
// alongside SetBarrierLogger, threading the daemon component's *slog.Logger.
// The result is a single grep anchor in portal.log — "daemon.version write"
// — that surfaces both the daemon-startup call site and the
// bootstrap-survived-path defensive repair (spec § Change 3, Acceptance
// Criterion #9).
func SetVersionWriterLogger(l *slog.Logger) {
	if l == nil {
		return
	}
	saver.Version.WriterLogger = l
}

// killSaverAndWaitForDaemon issues kill-session against _portal-saver and
// blocks until the prior daemon has actually exited or a timeout fires. The
// barrier is the "common-case quiet" half of the multi-daemon fix: without
// it, every recycle would produce a "lock held; exiting" WARN from the new
// daemon while the prior daemon finishes its synchronous tick. The daemon's
// lock acquisition remains the structural safety net — the barrier just
// makes the recycle path silent on the happy path.
//
// Flow:
//
//  1. Read the prior PID via saver.ReadPID. On any error (file absent,
//     unreadable, corrupted) → tolerant kill, return nil immediately. Polling
//     is skipped because there is no prior PID to wait for.
//  2. If the prior PID is already dead (saver.Barrier.IsAlive returns false on
//     the first probe) → tolerant kill, return nil immediately. Zero polling.
//  3. Otherwise issue kill-session exactly once, tolerating kill errors
//     (the session may have auto-destroyed between probe and kill — that is
//     equivalent to "already absent" for our purposes). Then enter the poll
//     loop: re-probe saver.Barrier.IsAlive every saver.Barrier.PollInterval until
//     it returns false or saver.Barrier.Timeout elapses.
//  4. On session-kill poll timeout → escalate (spec § Component A —
//     Kill-Barrier Escalation):
//     (a) Identity-check the prior PID via saver.IdentifyDaemon. If the
//     result is anything other than IdentifyIsPortalDaemon (recycled PID,
//     transient ps error, IdentifyDead) → emit one WARN and return nil
//     without signalling. Safety bias: never signal a PID we cannot
//     positively identify as a portal state daemon.
//     (b) IMMEDIATELY (no intervening statements that could let the PID
//     recycle between check and signal) send SIGKILL via
//     saver.Barrier.SendSIGKILL. SIGKILL is unblockable and bypasses the
//     daemon's shutdown handler — we explicitly do NOT want the orphan to
//     execute one final destructive captureAndCommit on its way out.
//     (c) Post-SIGKILL poll: probe saver.Barrier.IsAlive at
//     saver.Barrier.PollInterval cadence (50ms by default) for up to
//     saver.Barrier.EscalationTimeout (1s by default). On exit → return nil.
//     On persistent aliveness → emit one WARN and return nil.
//
// The helper never writes to the state directory. WARN is emitted at most
// once per invocation across the entire flow; the new daemon's lock
// acquisition is the safety net for genuinely-stuck cases.
func killSaverAndWaitForDaemon(c *Client, stateDir string) error {
	priorPID, readErr := saver.ReadPID(stateDir)
	if readErr != nil {
		// No usable prior PID. Tolerant kill, no polling.
		_ = c.KillSession(PortalSaverName)
		return nil
	}

	if !saver.Barrier.IsAlive(priorPID) {
		// Prior daemon already dead. Tolerant kill, no polling.
		_ = c.KillSession(PortalSaverName)
		return nil
	}

	// Prior daemon alive — issue kill-session once and wait for exit. Kill
	// error tolerated: the session may have auto-destroyed between probe and
	// kill (already absent), which is equivalent to a successful kill here.
	_ = c.KillSession(PortalSaverName)

	if waitForPriorPIDExit(priorPID, saver.Barrier.Timeout) {
		return nil
	}

	// Session-kill poll exhausted with the prior daemon still alive.
	// Escalate via identity-check + direct SIGKILL.
	return escalateKillToSIGKILL(priorPID)
}

// waitForPriorPIDExit polls saver.Barrier.IsAlive(pid) at saver.Barrier.PollInterval
// cadence until it returns false or budget elapses. Returns true on observed
// exit, false on timeout. Used by killSaverAndWaitForDaemon's session-kill
// wait stage; the post-SIGKILL wait stage shares the same probe primitive via
// an inline loop tuned to the escalation budget.
func waitForPriorPIDExit(pid int, budget time.Duration) bool {
	ticker := time.NewTicker(saver.Barrier.PollInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(budget)
	for range ticker.C {
		if !saver.Barrier.IsAlive(pid) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
	}
	return false
}

// escalateKillToSIGKILL runs the Component A escalation: identity-check
// priorPID and, only on IdentifyIsPortalDaemon, send SIGKILL via the seam as
// the IMMEDIATELY-following statement. Then poll saver.Barrier.IsAlive at
// saver.Barrier.PollInterval cadence for up to saver.Barrier.EscalationTimeout.
// All exit paths emit at most one WARN and return nil — bootstrap is
// best-effort at this stage and the new daemon's lock acquisition is the
// structural safety net.
//
// The identity-check → SIGKILL pairing is the spec's load-bearing
// residual-recycle-window invariant: the ONLY statement permitted between the
// identity check passing and the signal is the single DEBUG breadcrumb below.
// That breadcrumb is non-mutating — it does not touch priorPID, the process
// table, or any state that could let the PID recycle. It is a kernel-side write
// to an unbuffered O_APPEND fd (microseconds), not a scheduling window that
// materially widens the recycle race versus the pre-existing function-call
// overhead. A future reviewer MUST NOT "fix" the invariant by deleting the
// breadcrumb: it is the forensic record that the SIGKILL decision was committed
// (the decision-point DEBUG beneath the saver: kill-barrier escalated INFO
// lifecycle event), and it sits deliberately adjacent — immediately preceding —
// the SendSIGKILL seam call.
func escalateKillToSIGKILL(priorPID int) error {
	result, err := saver.IdentifyDaemon(priorPID)
	if err != nil || result != state.IdentifyIsPortalDaemon {
		saver.Barrier.Logger.Warn(
			"prior daemon not identity-checked as portal state daemon; skipping SIGKILL",
			"target_pid", priorPID,
		)
		return nil
	}
	saverLogger.Debug("kill-barrier escalating to SIGKILL", "target_pid", priorPID)
	_ = saver.Barrier.SendSIGKILL(priorPID)

	if waitForPriorPIDExit(priorPID, saver.Barrier.EscalationTimeout) {
		return nil
	}
	saver.Barrier.Logger.Warn(
		"prior daemon survived SIGKILL escalation",
		"target_pid", priorPID,
	)
	return nil
}

// waitForSaverDaemonReady polls daemon.pid + state.IdentifyDaemon until the
// freshly-respawned saver daemon has come up, bounded to saver.Readiness.Timeout
// total wall-clock at saver.Readiness.PollInterval cadence. It closes the
// race between the create-branch respawn-pane (which atomically swaps the
// placeholder for `portal state daemon`) and subsequent bootstrap steps
// (Restore, EagerSignalHydrate, ...) that assume a healthy daemon.
//
// Contract:
//
//   - Returns nil immediately the first time saver.ReadPID returns a
//     PID AND saver.IdentifyDaemon returns IdentifyIsPortalDaemon. This is
//     the success path.
//   - Returns nil on timeout AFTER emitting exactly one WARN through the
//     shared saver.Barrier.Logger sink under the bootstrap component
//     containing the literal "saver respawn: daemon did not come up".
//     The same Logger seam is consumed by the kill-barrier escalation paths
//     in killSaverAndWaitForDaemon / escalateKillToSIGKILL — installing it
//     via SetBarrierLogger wires both barriers' WARN sinks at once. This is
//     best-effort —
//     the daemon's own lock acquisition is the structural safety net for the
//     truly-stuck case, and the WARN gives operators a grep anchor without
//     escalating to a fatal bootstrap abort.
//   - Treats every not-yet-ready signal as a continue: ErrPIDFileAbsent,
//     transient PID-file read errors, transient IdentifyDaemon ps failures,
//     IdentifyDead (PID has not yet been re-published after fork), and
//     IdentifyNotPortalDaemon (recycled PID still being observed) all loop
//     until the deadline.
//   - Never writes to the state directory.
//   - The deadline is computed once at entry so the 2s ceiling is enforced
//     regardless of how many transient errors occur inside the loop.
func waitForSaverDaemonReady(stateDir string) error {
	deadline := time.Now().Add(saver.Readiness.Timeout)

	ticker := time.NewTicker(saver.Readiness.PollInterval)
	defer ticker.Stop()

	for {
		if isSaverDaemonReady(stateDir) {
			return nil
		}
		if !time.Now().Before(deadline) {
			saver.Barrier.Logger.Warn("saver respawn: daemon did not come up")
			return nil
		}
		<-ticker.C
	}
}

// isSaverDaemonReady is one tick of the readiness barrier: read daemon.pid,
// identify it, and return true iff the PID is alive AND identifies as a
// portal state daemon. Any other observable shape (read error, transient
// identify error, IdentifyDead, IdentifyNotPortalDaemon) returns false so
// the caller continues polling.
func isSaverDaemonReady(stateDir string) bool {
	pid, err := saver.ReadPID(stateDir)
	if err != nil {
		return false
	}
	result, err := saver.IdentifyDaemon(pid)
	if err != nil {
		return false
	}
	return result == state.IdentifyIsPortalDaemon
}

// BootstrapPortalSaver ensures the _portal-saver session exists and is hosting
// a live daemon, idempotently. The flow:
//
//  1. Probe has-session for _portal-saver.
//  2. If present, verify daemon liveness via BootstrapAliveCheck (state dir's
//     daemon.pid + signal-0 probe).
//  3. If the session is present but the daemon is dead, kill the orphan via
//     the synchronous barrier and fall through to the create path.
//  4. Create branch (entered when the session is absent, or after the kill in
//     step 3) — three load-bearing sub-steps in this exact order:
//     (a) createPortalSaverWithRetry creates _portal-saver with a benign
//     placeholder command (`sh -c 'exec tail -f /dev/null'`). Retry/race
//     logic is preserved; a concurrent-bootstrap race that finds the
//     session already present via the post-error has-session re-probe is
//     still treated as success.
//     (b) SetSessionOption applies destroy-unattached=off against the now
//     guaranteed-alive placeholder session. This is the load-bearing
//     reorder: previously the option was set AFTER the daemon was already
//     running as the initial pane process, so a lock-loser daemon exiting
//     between new-session and set-option would let tmux self-destroy the
//     session before destroy-unattached=off applied — producing "no such
//     session" log noise and a recovery doom-loop. With the placeholder
//     keeping the session alive unconditionally, set-option is safe.
//     (c) RespawnPane replaces the placeholder with `portal state daemon`
//     via `respawn-pane -k`. -k atomically kills the placeholder and
//     installs the daemon in its place; the pane survives, only the
//     process changes. Even if the daemon exits immediately as a
//     lock-loser, destroy-unattached=off is already in effect so the
//     session persists for the next bootstrap to evaluate.
//  5. Session-present-and-alive happy path: SetSessionOption still runs as
//     defence against users with destroy-unattached on globally, but
//     RespawnPane does NOT — the existing daemon is healthy and recycling
//     its pane process would be unnecessary churn.
//
// Does not touch @portal-restoring or version-marker logic — those are owned
// by adjacent bootstrap stages.
func BootstrapPortalSaver(c *Client, stateDir string) error {
	sessionPresent := c.HasSession(PortalSaverName)

	if sessionPresent && !BootstrapAliveCheck(stateDir) {
		// Session lingering with a dead daemon — kill via the synchronous
		// barrier so the prior daemon's exit precedes the respawn, then
		// fall through to recreate.
		_ = saver.Ops.KillAndWait(c, stateDir)
		sessionPresent = false
	}

	createdSession := false
	if !sessionPresent {
		if err := createPortalSaverWithRetry(c); err != nil {
			return err
		}
		createdSession = true
	}

	if err := c.SetSessionOption(PortalSaverName, "destroy-unattached", "off"); err != nil {
		return fmt.Errorf("bootstrap _portal-saver: set destroy-unattached: %w", err)
	}

	if createdSession {
		if err := c.RespawnPane(PortalSaverName, portalSaverDaemonCommand); err != nil {
			return fmt.Errorf("bootstrap _portal-saver: respawn daemon: %w", err)
		}
		// Readiness barrier: poll daemon.pid + IdentifyDaemon until the
		// freshly-respawned daemon is observably up, bounded to
		// saver.Readiness.Timeout. Routed through waitForSaverDaemonReadyFn so
		// unit tests not exercising the barrier can stub it out cheaply.
		// Always returns nil today — WARN-on-timeout is logged inside the
		// helper. The barrier closes the gap where subsequent bootstrap
		// steps (Restore, EagerSignalHydrate, ...) would otherwise observe
		// a not-yet-up daemon racing the respawn.
		_ = saver.Ops.WaitForReady(stateDir)
	}

	return nil
}

// EnsurePortalSaverVersion bootstraps _portal-saver while honoring the
// version-marker upgrade protocol. The kill decision is gated on daemon
// aliveness FIRST, then the version-mismatch matrix is consulted only when a
// live daemon is present. The full decision matrix:
//
//	| alive | version file state             | versions match | action  |
//	|-------|--------------------------------|----------------|---------|
//	| no    | (any)                          | (any)          | no kill |
//	| yes   | (any)                          | either is dev  | kill    |
//	| yes   | absent (ErrVersionFileAbsent)  | (n/a)          | no kill |
//	| yes   | read error (non-absent I/O)    | (n/a)          | kill    |
//	| yes   | reads cleanly, neither dev     | match          | no kill |
//	| yes   | reads cleanly, neither dev     | mismatch       | kill    |
//
// Rationale: a healthy daemon should not be recycled simply because
// daemon.version is missing — the absent file can be repaired defensively
// from the bootstrap side (Task 1-4) without paying the kill-respawn cost.
// The "absent" row used to be folded into the mismatch predicate, which made
// every bootstrap with a missing version file fire an unnecessary kill.
//
// kill-session is invoked tolerantly via killSaverAndWaitForDaemonFn: the
// barrier helper handles its own kill-session tolerance and the wait for the
// prior daemon's exit. This function never writes daemon.version itself for
// the kill path — the new daemon owns that on its own startup. On the
// alive+absent branch (alive=true and readErr is ErrVersionFileAbsent) it
// defensively writes daemon.version=currentVersion via
// portalSaverWriteVersionFile before falling through to BootstrapPortalSaver,
// closing the lock-loser lifecycle hole described in the spec: lock-loser
// daemons exit cleanly before writing daemon.version, leaving the file
// observably absent until the next bootstrap repairs it. In the pathological
// case where the alive daemon is running an older binary AND daemon.version
// was absent, this write effectively asserts "the version going forward"
// rather than the running daemon's actual version; any disagreement is
// resolved by the next legitimate recycle.
//
// After the optional kill or defensive write, BootstrapPortalSaver always
// runs to (re)create the session and apply the defensive
// destroy-unattached=off option.
func EnsurePortalSaverVersion(c *Client, stateDir, currentVersion string) error {
	stored, readErr := saver.Version.ReadVersionFile(stateDir)
	alive := BootstrapAliveCheck(stateDir)

	if alive && shouldKillSaverOnVersionDecision(stored, currentVersion, readErr) {
		_ = saver.Ops.KillAndWait(c, stateDir)
	} else if alive && errors.Is(readErr, state.ErrVersionFileAbsent) {
		// Defensive complement: lock-loser daemons return cleanly before
		// writing daemon.version, so on every bootstrap we observe the
		// alive-daemon + missing-version-file shape until the file is
		// repaired. Write currentVersion from the bootstrap side so the
		// audit trail is restored and the kill cascade cannot re-trigger
		// silently. A write failure must propagate — BootstrapPortalSaver
		// is NOT called when the defensive write fails.
		if err := saver.Version.WriteVersionFile(stateDir, currentVersion); err != nil {
			return fmt.Errorf("defensive daemon.version write failed: %w", err)
		}
	}
	return BootstrapPortalSaver(c, stateDir)
}

// shouldKillSaverOnVersionDecision encodes the kill-decision matrix for
// EnsurePortalSaverVersion on the alive-daemon branch only. Callers must have
// already established that the daemon is alive — this helper does not
// consult BootstrapAliveCheck. It returns true on every "kill" row of the
// matrix and false on every "no kill" row.
//
// Evaluation order, matching the spec matrix:
//
//  1. Dev-build short-circuit: either side of the version pair is "" or
//     "dev" (with the stored-side check gated on readErr == nil so we do
//     not interpret an unreadable file as "stored is empty").
//  2. Absent version file (errors.Is(readErr, ErrVersionFileAbsent)): no
//     kill — Task 1-4 layers a defensive write here from the caller.
//  3. Non-absent read error: kill (conservative).
//  4. Clean read, versions match: no kill.
//  5. Clean read, versions differ: kill.
func shouldKillSaverOnVersionDecision(stored, currentVersion string, readErr error) bool {
	// Dev-build short-circuit. currentVersion always counts regardless of
	// readErr; stored-side only counts when the file read succeeded (a read
	// error already means "stored is unknown", not "stored is empty").
	if currentVersion == "" || currentVersion == "dev" {
		return true
	}
	if readErr == nil && (stored == "" || stored == "dev") {
		return true
	}

	if errors.Is(readErr, state.ErrVersionFileAbsent) {
		// Alive daemon + missing version file: no kill. The defensive
		// write (Task 1-4) repairs the file from the bootstrap side.
		return false
	}
	if readErr != nil {
		// Non-absent I/O error — conservative: treat unknown state as
		// "needs recycle". The alive-check above guards against killing a
		// daemon when no daemon is present.
		return true
	}

	return stored != currentVersion
}

// createPortalSaverWithRetry attempts to create the saver session up to
// portalSaverMaxAttempts times, sleeping PortalSaverRetryDelay between
// attempts. After a failure it re-probes has-session to detect concurrent
// bootstraps that may already have created the session — that is treated as
// success so we do not stack duplicate-creation errors.
//
// The session is created with portalSaverPlaceholderCommand as its initial
// pane process, NOT the real daemon command. The placeholder is structurally
// incapable of contending for the daemon lock or writing to the state
// directory; BootstrapPortalSaver's caller swaps it for the real daemon via
// respawn-pane AFTER applying destroy-unattached=off. See the
// portalSaverPlaceholderCommand docstring for why `tail -f /dev/null` is
// portable while `sleep infinity` is not.
func createPortalSaverWithRetry(c *Client) error {
	var lastErr error
	for attempt := 1; attempt <= portalSaverMaxAttempts; attempt++ {
		err := c.NewDetachedSessionNoCwd(PortalSaverName, portalSaverPlaceholderCommand)
		if err == nil {
			return nil
		}
		lastErr = err

		// Concurrent-bootstrap race: another caller may have created the
		// session while we were attempting. If so, treat as success.
		if c.HasSession(PortalSaverName) {
			return nil
		}

		if attempt < portalSaverMaxAttempts {
			time.Sleep(PortalSaverRetryDelay)
		}
	}
	return fmt.Errorf("bootstrap _portal-saver: create after %d attempts: %w", portalSaverMaxAttempts, lastErr)
}
