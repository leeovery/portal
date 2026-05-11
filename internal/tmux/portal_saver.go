package tmux

import (
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
// emissions. Production wiring (added in Task 2.2) replaces this with a real
// *state.Logger via init() or the bootstrap-adapter path. Tests install a
// recording fake via the seam and reset it through t.Cleanup.
var killBarrierLogger BarrierLogger = noopBarrierLogger{}

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
				"bootstrap",
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
		// Session lingering with a dead daemon — kill tolerantly and recreate.
		_ = c.KillSession(PortalSaverName)
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
// version-marker upgrade protocol. Before delegating to BootstrapPortalSaver,
// it compares the daemon's recorded version (daemon.version inside stateDir)
// to currentVersion (the invoking binary's cmd.version). On any mismatch — and
// always for dev builds (currentVersion == "" or "dev") — it kills the live
// _portal-saver session so the new binary can take over on the subsequent
// recreation. Mismatch sources, all treated equally:
//
//   - Read error from state.ReadVersionFile (including ErrVersionFileAbsent —
//     first-ever bootstrap or user-initiated state-dir cleanup).
//   - currentVersion is "" or "dev" (dev-build workflow).
//   - stored version is "" or "dev" (previous run was a dev build, or the
//     daemon crashed before writing).
//   - stored != currentVersion (release-build upgrade).
//
// kill-session is invoked tolerantly: errors are intentionally swallowed
// because the session may have auto-destroyed between has-session and
// kill-session. This function never writes daemon.version itself — the new
// daemon owns that on its own startup. After the optional kill,
// BootstrapPortalSaver always runs to (re)create the session and apply the
// defensive destroy-unattached=off option.
func EnsurePortalSaverVersion(c *Client, stateDir, currentVersion string) error {
	stored, readErr := state.ReadVersionFile(stateDir)
	if portalSaverVersionMismatch(stored, currentVersion, readErr) && c.HasSession(PortalSaverName) {
		// Tolerant: kill may fail if the session vanished mid-flight; that is
		// equivalent to "already absent" for our purposes.
		_ = c.KillSession(PortalSaverName)
	}
	return BootstrapPortalSaver(c, stateDir)
}

// portalSaverVersionMismatch encodes the version-comparison rules for
// EnsurePortalSaverVersion. Any failure to read the version file, a dev /
// empty currentVersion, a dev / empty stored version, or a non-equal stored
// vs. current pair all count as a mismatch.
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
