package state

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// IdentifyResult is the three-way classification returned by IdentifyDaemon.
// It is the definitive answer when the accompanying error is nil; on a
// non-nil error the result is meaningless (returned as zero) and the caller
// applies component-specific transient-error policy.
type IdentifyResult int

const (
	// IdentifyIsPortalDaemon means the PID is alive AND its comm/argv match
	// a portal state daemon (comm == "portal" AND argv matches the anchored
	// regex "^portal state daemon( |$)").
	IdentifyIsPortalDaemon IdentifyResult = iota

	// IdentifyNotPortalDaemon means the PID is alive but is NOT a portal
	// state daemon (recycled PID, different binary, or portal invoked with
	// a different subcommand).
	IdentifyNotPortalDaemon

	// IdentifyDead means the PID does not exist (gone since last
	// observation, or never existed). pid <= 0 is also treated as dead
	// without invoking ps.
	IdentifyDead
)

// PortalDaemonArgvPattern is the canonical anchored argv match for a live
// `portal state daemon` process. The trailing "( |$)" disjunction permits
// exact match (end-of-string) or continued argv (trailing space + flags
// like "--foo"). A suffix such as "portal state daemon-foo" is rejected
// because neither space nor end follows "daemon".
//
// Exported so the bootstrap adapter's `pgrep -fx` enumeration and the
// portaltest pgrep helper share a single source of truth with the regex
// compiled by IdentifyDaemon below.
const PortalDaemonArgvPattern = `^portal state daemon( |$)`

// daemonArgvPattern is the compiled form of PortalDaemonArgvPattern used by
// IdentifyDaemon's argv match.
var daemonArgvPattern = regexp.MustCompile(PortalDaemonArgvPattern)

// identifyPS is the test seam over the `ps -o comm=,args= -p <pid>` invocation.
// Production code uses defaultIdentifyPS unchanged; tests in this package swap
// identifyPS to simulate canned ps output / exit shapes without forking a real
// process.
//
// The contract:
//   - On zero exit: return ps stdout (string) and nil error.
//   - On non-zero exit: return whatever stdout was captured (may be empty) and
//     a non-nil error.
//
// IdentifyDaemon distinguishes "PID not found" (non-zero exit + empty stdout)
// from a transient ps failure (non-zero exit + non-empty stdout) using this
// pair.
var identifyPS = defaultIdentifyPS

func defaultIdentifyPS(pid int) (string, error) {
	// Boundary class 1: capture stderr + embed argv/exit-status in the wrapped
	// error via the shared helper. The helper returns the captured stdout on
	// the error path too, so IdentifyDaemon's pid-not-found (non-zero exit +
	// empty stdout → IdentifyDead) vs transient (non-zero exit + non-empty
	// stdout) discrimination is preserved unchanged — it keys on stdout
	// emptiness, which the helper does not alter.
	cmd := exec.Command("ps", "-o", "comm=,args=", "-p", strconv.Itoa(pid))
	out, err := log.CombinedOutputWithContext(cmd)
	return string(out), err
}

// IdentifyDaemon classifies whether the process at pid is a live
// `portal state daemon`. It is the shared primitive consumed by:
//
//   - Component A (kill-barrier escalation in internal/tmux/portal_saver.go):
//     gates the post-poll SIGKILL on the prior daemon PID.
//   - Component B (bootstrap orphan-sweep in cmd/bootstrap/): gates SIGKILL on
//     each pgrep-discovered candidate PID.
//   - Component C (daemon.lock pre-check in internal/state/daemon_lock.go):
//     gates returning ErrDaemonLockHeld based on the recorded daemon.pid.
//
// Three-result contract (err == nil):
//
//   - IdentifyIsPortalDaemon: pid is alive AND argv matches
//     `^portal state daemon( |$)` AND comm == "portal".
//   - IdentifyNotPortalDaemon: pid is alive but is NOT a portal state daemon
//     (recycled to a different binary, or portal invoked with a different
//     subcommand, etc.).
//   - IdentifyDead: pid does not exist (canonical "ps -p" exits non-zero with
//     empty stdout) OR pid <= 0 (defensive guard, never shells out).
//
// Transient-error contract (err != nil): the identity check itself failed
// (ps exec failure with non-empty stdout, malformed ps output, etc.). The
// IdentifyResult return is the zero value and is meaningless; callers apply
// component-specific policy:
//
//   - Component A: skip SIGKILL (do not signal a PID we cannot identify).
//   - Component B: skip this PID (next bootstrap will re-sweep).
//   - Component C: treat as "not a portal daemon" — proceed with acquire
//     (the flock EWOULDBLOCK fallback still catches real contention; biasing
//     toward letting legitimate succession proceed is safer than spuriously
//     blocking startup).
func IdentifyDaemon(pid int) (IdentifyResult, error) {
	if pid <= 0 {
		return IdentifyDead, nil
	}

	stdout, execErr := identifyPS(pid)
	trimmed := strings.TrimSpace(stdout)

	if execErr != nil {
		if trimmed == "" {
			// Canonical "PID not found" shape: ps exited non-zero with no
			// output on stdout. Definitive — not transient.
			return IdentifyDead, nil
		}
		// Non-zero exit with output is unexpected — treat as transient so the
		// caller applies its component-specific policy rather than us
		// committing to an answer we cannot defend.
		return 0, fmt.Errorf("identify pid %d: ps failed with stdout %q: %w", pid, trimmed, execErr)
	}

	if trimmed == "" {
		// Zero exit but no parseable output. Defensive — treat as transient.
		return 0, fmt.Errorf("identify pid %d: ps produced empty output", pid)
	}

	comm, argv, ok := splitCommAndArgv(trimmed)
	if !ok {
		return 0, fmt.Errorf("identify pid %d: malformed ps output %q", pid, trimmed)
	}

	if comm == "portal" && daemonArgvPattern.MatchString(argv) {
		return IdentifyIsPortalDaemon, nil
	}
	return IdentifyNotPortalDaemon, nil
}

// splitCommAndArgv splits a trimmed ps line of the form `<comm> <argv...>`
// into its comm and argv parts on the first run of whitespace. Returns ok=false
// when there is no whitespace separator (single token), which IdentifyDaemon
// surfaces as a transient parse error.
func splitCommAndArgv(line string) (comm, argv string, ok bool) {
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return "", "", false
	}
	comm = line[:idx]
	argv = strings.TrimLeft(line[idx+1:], " \t")
	if argv == "" {
		return "", "", false
	}
	return comm, argv, true
}
