package tmux

import (
	"errors"
	"fmt"
	"time"

	"github.com/leeovery/portal/internal/state"
)

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

// portalSaverCommand is the shell command run as the saver session's initial
// process. tmux owns the daemon's lifecycle: when this session is killed (or
// the server dies), the kernel delivers SIGHUP to the daemon for graceful
// shutdown.
const portalSaverCommand = "portal state daemon"

// BootstrapAliveCheck is the function used to test whether a daemon is alive
// for a given state directory. It is a package-level seam so tests can stub
// the check without writing real PID files. Defaults to state.DaemonAlive.
var BootstrapAliveCheck = state.DaemonAlive

// portalSaverReadVersionFile is the function used by EnsurePortalSaverVersion
// to read the stored daemon.version from a state directory. It is a
// package-level seam so tests can simulate read errors (including non-absent
// I/O failures) without touching the filesystem. Defaults to
// state.ReadVersionFile.
var portalSaverReadVersionFile = state.ReadVersionFile

// portalSaverWriteVersionFile is the function used by EnsurePortalSaverVersion
// on the alive+absent branch to defensively write daemon.version before
// falling through to BootstrapPortalSaver. It is a package-level seam so tests
// can record invocations and inject errors without touching the filesystem.
//
// The seam binds the logger choice on the production side by passing nil to
// state.WriteVersionFile — Logger's nil-receiver contract treats this as a
// no-op, so the bootstrap breadcrumb from Task 1-2 does not land for this
// defensive call site. Wiring a real logger here can be a follow-up if a
// breadcrumb at the call site is wanted.
var portalSaverWriteVersionFile = func(dir, version string) error {
	return state.WriteVersionFile(dir, version, nil)
}

// PortalSaverRetryDelay is the sleep between new-session retry attempts. It is
// exported as a var so tests can shrink it. Defaults to 100ms.
var PortalSaverRetryDelay = 100 * time.Millisecond

// portalSaverMaxAttempts is the maximum number of new-session attempts before
// BootstrapPortalSaver gives up and returns an error.
const portalSaverMaxAttempts = 3

// killBarrierReadPID is the function used to read the prior daemon's PID from
// the state directory before a kill-respawn cycle. It is a package-level seam
// so kill-barrier unit tests can simulate "no PID file", "corrupted PID file",
// or "unreadable PID file" cases without touching the filesystem. Defaults to
// state.ReadPIDFile.
var killBarrierReadPID = state.ReadPIDFile

// killBarrierIsAlive is the function used by the kill barrier to probe whether
// the prior daemon is still running. It is a package-level seam so tests can
// simulate "alive then dead after N ticks" / "alive forever" / "already dead"
// without spawning real processes. Defaults to state.IsProcessAlive.
var killBarrierIsAlive = state.IsProcessAlive

// killBarrierPollInterval is the cadence at which the kill barrier re-probes
// killBarrierIsAlive after issuing kill-session. Exported as a var so tests
// can shrink it. Defaults to 50ms — chosen alongside killBarrierTimeout so
// the median recycle path completes in well under a second.
var killBarrierPollInterval = 50 * time.Millisecond

// killBarrierTimeout is the upper bound on the kill barrier's wait for the
// prior daemon to exit. Sized to sit above the daemon's cold-sweep ceiling
// (3.9s on the affected user's scrollback profile) with margin, so the WARN
// path is reserved for genuinely stuck daemons rather than ordinary cold
// sweeps. Exported as a var so tests can shrink it.
var killBarrierTimeout = 5 * time.Second

// BarrierLogger is the minimal logging seam killSaverAndWaitForDaemon needs.
// A single Warn method is the smallest surface that conveys the spec's
// observable shape: one WARN-level event on barrier timeout.
//
// *state.Logger satisfies this interface structurally; defining a local
// interface mirrors the MigrationLogger precedent in hooks_register.go,
// avoiding the import cycle that would otherwise force callers to depend on
// internal/state directly.
type BarrierLogger interface {
	Warn(component, format string, args ...any)
}

// noopBarrierLogger satisfies BarrierLogger with a no-op Warn so the kill
// barrier always has a safe sink when production wiring has not installed a
// real logger.
type noopBarrierLogger struct{}

func (noopBarrierLogger) Warn(component, format string, args ...any) {}

// killBarrierLogger is the package-level sink for kill-barrier WARN
// emissions. Production wiring (Task 2.2) replaces this with a real
// *state.Logger via SetBarrierLogger, invoked from internal/bootstrapadapter
// at the same site that constructs the HookRegistrar's *state.Logger. Tests
// install a recording fake via the seam and reset it through t.Cleanup.
var killBarrierLogger BarrierLogger = noopBarrierLogger{}

// SetBarrierLogger installs a BarrierLogger as the sink for kill-barrier
// WARN-on-timeout emissions. A nil argument is ignored so the package never
// loses its sink to a programming error in the wiring layer; the default is
// noopBarrierLogger which already swallows all calls safely.
//
// Production wiring calls this once from internal/bootstrapadapter as part
// of constructing the HookRegistrar, threading the same *state.Logger that
// the rest of bootstrap uses. *state.Logger structurally satisfies
// BarrierLogger via its Warn(component, format string, args ...any) method.
func SetBarrierLogger(l BarrierLogger) {
	if l == nil {
		return
	}
	killBarrierLogger = l
}

// killSaverAndWaitForDaemonFn is the package-level seam that both
// production kill call sites route through. It defaults to the production
// helper; unit tests swap it with a recorder or a no-op via the
// KillSaverAndWaitForDaemonFnSeam test export. Routing both call sites
// through this single var keeps the wiring symmetric and lets a single
// stub disable barrier semantics for tests that only care about call-site
// behaviour, while leaving the real helper available for tests that want
// the full kill+poll flow (with the inner seams stubbed).
var killSaverAndWaitForDaemonFn = killSaverAndWaitForDaemon

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
//  1. Read the prior PID via killBarrierReadPID. On any error (file absent,
//     unreadable, corrupted) → tolerant kill, return nil immediately. Polling
//     is skipped because there is no prior PID to wait for.
//  2. If the prior PID is already dead (killBarrierIsAlive returns false on
//     the first probe) → tolerant kill, return nil immediately. Zero polling.
//  3. Otherwise issue kill-session exactly once, tolerating kill errors
//     (the session may have auto-destroyed between probe and kill — that is
//     equivalent to "already absent" for our purposes). Then enter the poll
//     loop: re-probe killBarrierIsAlive every killBarrierPollInterval until
//     it returns false or killBarrierTimeout elapses.
//  4. On timeout → emit exactly one WARN via killBarrierLogger and return
//     nil. The new daemon's lock acquisition is the safety net for the
//     genuinely-stuck case.
//
// The helper never writes to the state directory.
func killSaverAndWaitForDaemon(c *Client, stateDir string) error {
	priorPID, readErr := killBarrierReadPID(stateDir)
	if readErr != nil {
		// No usable prior PID. Tolerant kill, no polling.
		_ = c.KillSession(PortalSaverName)
		return nil
	}

	if !killBarrierIsAlive(priorPID) {
		// Prior daemon already dead. Tolerant kill, no polling.
		_ = c.KillSession(PortalSaverName)
		return nil
	}

	// Prior daemon alive — issue kill-session once and wait for exit.
	_ = c.KillSession(PortalSaverName)

	ticker := time.NewTicker(killBarrierPollInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(killBarrierTimeout)
	for range ticker.C {
		if !killBarrierIsAlive(priorPID) {
			return nil
		}
		if !time.Now().Before(deadline) {
			killBarrierLogger.Warn(
				state.ComponentBootstrap,
				"prior daemon (pid=%d) did not exit within %v",
				priorPID,
				killBarrierTimeout,
			)
			return nil
		}
	}
	return nil
}

// BootstrapPortalSaver ensures the _portal-saver session exists and is hosting
// a live daemon, idempotently. The flow:
//
//  1. Probe has-session for _portal-saver.
//  2. If present, verify daemon liveness via BootstrapAliveCheck (state dir's
//     daemon.pid + signal-0 probe).
//  3. If the session is present but the daemon is dead, kill the orphan
//     (tolerantly) and fall through to the create path.
//  4. Create _portal-saver with retry on transient failures. After each
//     failed new-session, re-probe has-session — a concurrent bootstrap may
//     have won the race, in which case treat the present session as success.
//  5. Always set destroy-unattached=off on the session (defensive against
//     users with destroy-unattached on globally in their tmux.conf).
//
// Does not touch @portal-restoring or version-marker logic — those are owned
// by adjacent bootstrap stages.
func BootstrapPortalSaver(c *Client, stateDir string) error {
	sessionPresent := c.HasSession(PortalSaverName)

	if sessionPresent && !BootstrapAliveCheck(stateDir) {
		// Session lingering with a dead daemon — kill via the synchronous
		// barrier so the prior daemon's exit precedes the respawn, then
		// fall through to recreate.
		_ = killSaverAndWaitForDaemonFn(c, stateDir)
		sessionPresent = false
	}

	if !sessionPresent {
		if err := createPortalSaverWithRetry(c); err != nil {
			return err
		}
	}

	if err := c.SetSessionOption(PortalSaverName, "destroy-unattached", "off"); err != nil {
		return fmt.Errorf("bootstrap _portal-saver: set destroy-unattached: %w", err)
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
// portalSaverVersionMismatch retains its current external shape and is still
// covered by its own predicate-matrix test (it is also referenced by other
// code paths); this caller no longer drives the kill decision from it
// directly. The dev short-circuit and "read-error-is-mismatch" behaviours of
// the predicate are reproduced inline here, byte-equivalent in semantics, so
// the matrix above is the single source of truth for the kill decision.
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
	stored, readErr := portalSaverReadVersionFile(stateDir)
	alive := BootstrapAliveCheck(stateDir)

	switch {
	case alive && shouldKillSaverOnVersionDecision(stored, currentVersion, readErr):
		_ = killSaverAndWaitForDaemonFn(c, stateDir)
	case alive && errors.Is(readErr, state.ErrVersionFileAbsent):
		// Defensive complement: lock-loser daemons return cleanly before
		// writing daemon.version, so on every bootstrap we observe the
		// alive-daemon + missing-version-file shape until the file is
		// repaired. Write currentVersion from the bootstrap side so the
		// audit trail is restored and the kill cascade cannot re-trigger
		// silently. A write failure must propagate — BootstrapPortalSaver
		// is NOT called when the defensive write fails.
		if err := portalSaverWriteVersionFile(stateDir, currentVersion); err != nil {
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
//     not interpret an unreadable file as "stored is empty"). Byte-
//     equivalent to portalSaverVersionMismatch's dev rules.
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

// portalSaverVersionMismatch is a defensive predicate answering the question
// "if we were to kill the current daemon, should we trust that a freshly
// respawned daemon would be on a different/usable version?". It returns true
// when:
//
//  1. stored != currentVersion (a real version mismatch on a clean read), OR
//  2. either side of the pair is "" or "dev" (dev-build short-circuit — we
//     cannot reason about version equality when either side is unstamped), OR
//  3. the read failed for any reason, INCLUDING errors.Is(readErr,
//     state.ErrVersionFileAbsent) — an unreadable or absent file does not
//     let the predicate answer "no" with confidence, so it answers "yes"
//     defensively at this layer.
//
// It returns false otherwise (clean read, neither side dev/empty, versions
// equal).
//
// NOTE — not load-bearing for the kill decision: as of the saver-kill-respawn
// fix, EnsurePortalSaverVersion no longer drives its kill decision from this
// predicate. The authoritative kill gate is now BootstrapAliveCheck(stateDir)
// FIRST — a dead daemon is never killed and an alive daemon with an absent
// version file is repaired defensively (via portalSaverWriteVersionFile)
// rather than recycled. The predicate's "absent counts as mismatch" rule
// therefore no longer translates into "absent triggers a kill"; it only
// expresses the predicate's local, defensive contract. See
// EnsurePortalSaverVersion (and shouldKillSaverOnVersionDecision, which
// encodes the alive-daemon kill-decision matrix inline) for the authoritative
// kill-decision rules.
//
// This predicate is preserved for callers that want the dev-short-circuit /
// read-error-folded shape and is covered by its own predicate-matrix test.
func portalSaverVersionMismatch(stored, currentVersion string, readErr error) bool {
	if readErr != nil {
		return true
	}
	if currentVersion == "" || currentVersion == "dev" {
		return true
	}
	if stored == "" || stored == "dev" {
		return true
	}
	return stored != currentVersion
}

// createPortalSaverWithRetry attempts to create the saver session up to
// portalSaverMaxAttempts times, sleeping PortalSaverRetryDelay between
// attempts. After a failure it re-probes has-session to detect concurrent
// bootstraps that may already have created the session — that is treated as
// success so we do not stack duplicate-creation errors.
func createPortalSaverWithRetry(c *Client) error {
	var lastErr error
	for attempt := 1; attempt <= portalSaverMaxAttempts; attempt++ {
		err := c.NewDetachedSessionNoCwd(PortalSaverName, portalSaverCommand)
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
