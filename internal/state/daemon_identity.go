package state

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// IdentifyResult is meaningful only when the accompanying error is nil.
type IdentifyResult int

const (
	// IdentifyIsPortalDaemon means the pid is alive and its comm and argv both
	// match a portal state daemon.
	IdentifyIsPortalDaemon IdentifyResult = iota

	// IdentifyNotPortalDaemon means the pid is alive but is something else — a
	// recycled pid, another binary, or portal on a different subcommand.
	IdentifyNotPortalDaemon

	// IdentifyDead means the pid does not exist. A pid <= 0 is also dead.
	IdentifyDead
)

// portalProgram is portal as a daemon's argv[0] names it: bare, or an absolute
// path whose last element is exactly portal, as the saver runs it. The path
// admits no space, which is what keeps a wrapper such as
// "/bin/sh -c /x/portal state daemon" from reading as a daemon.
const portalProgram = `(/([^ ]*/)?)?portal`

// PortalDaemonArgvPattern's trailing "( |$)" accepts an exact match or trailing
// flags while rejecting a suffix such as "portal state daemon-foo".
const PortalDaemonArgvPattern = `^` + portalProgram + ` state daemon( |$)`

var daemonArgvPattern = regexp.MustCompile(PortalDaemonArgvPattern)

var identifyPS = defaultIdentifyPS

func defaultIdentifyPS(pid int) (string, error) {
	// The helper still returns captured stdout on the error path, which
	// IdentifyDaemon's dead-vs-transient discrimination keys on. ucomm is the
	// name of the file the process exec'd, as the kernel keeps it: darwin's
	// comm is argv[0], cut to 16 bytes in any column but the last, so a
	// daemon at an absolute path would read as no portal.
	cmd := exec.Command("ps", "-o", "ucomm=,args=", "-p", strconv.Itoa(pid))
	out, err := log.CombinedOutputWithContext(cmd)
	return string(out), err
}

// IdentifyDaemon's non-nil error means the check itself failed and the result is
// meaningless — callers decide their own policy for an unidentifiable pid.
func IdentifyDaemon(pid int) (IdentifyResult, error) {
	if pid <= 0 {
		return IdentifyDead, nil
	}

	stdout, execErr := identifyPS(pid)
	trimmed := strings.TrimSpace(stdout)

	if execErr != nil {
		// A ps that ran and exited non-zero with no output is the canonical
		// pid-not-found shape; one that never ran says nothing of the pid.
		if _, exited := errors.AsType[*exec.ExitError](execErr); exited && trimmed == "" {
			return IdentifyDead, nil
		}
		return 0, fmt.Errorf("identify pid %d: ps failed with stdout %q: %w", pid, trimmed, execErr)
	}

	if trimmed == "" {
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
