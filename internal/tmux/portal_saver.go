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
