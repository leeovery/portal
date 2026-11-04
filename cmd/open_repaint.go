package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

// repaintSignal is kill(2), the one way the repaint reaches a process. cmd's
// TestMain makes it panic: the pids a test's backend reports are live
// processes on the developer's machine, none of them the test's.
var repaintSignal = unix.Kill

// readSessionProcess reads, from the kernel as it is called, what the repaint
// proves of the process a session runs.
var readSessionProcess = func(pid int) (sessionProcess, error) {
	group, err := unix.Getpgid(pid)
	if err != nil {
		return sessionProcess{}, fmt.Errorf("read the process group of pid %d: %w", pid, err)
	}
	args, err := proctable.ReadArgs(pid)
	if err != nil {
		return sessionProcess{}, err
	}
	return sessionProcess{Group: group, Argv: args.Argv}, nil
}

// sessionProcess is a session's process as the repaint reads it.
type sessionProcess struct {
	// Group is the process group the process is in.
	Group int
	// Argv is the argv the process was exec'd with.
	Argv []string
}

// repaintBound bounds the read of a waiting session's process, so a session
// that doesn't answer holds its attach back no longer.
const repaintBound = time.Second

// repaintWaiting asks the resume panel of session name, if it waits, to draw
// itself again, so a panel drawn at restore shows the theme in force now: the
// panel redraws on SIGWINCH. The zmx pinned today sends one of its own as a
// client attaches, but nothing in zmx promises it. The repaint signals only
// the process group of the process the backend names for the session just
// before, once that process is proved to lead its own group and run the
// session's parked resume chain; so the group is the session's own, and holds
// its panel. A session that isn't waiting is sent nothing. A repaint that
// fails is logged and holds back nothing.
func repaintWaiting(ctx context.Context, b backend.Backend, name string) {
	pid, err := repaintTarget(ctx, b, name)
	if err == nil && pid != 0 {
		err = repaintSignal(-pid, unix.SIGWINCH)
	}
	switch {
	case err != nil:
		attachLogger.Warn("repaint failed", "session", name, "error", err)
	case pid != 0:
		attachLogger.Debug("repaint sent", "session", name, "target_pid", pid)
	}
}

// repaintTarget is the process whose group the repaint signals, 0 when the
// session's resume-pending flag is down.
func repaintTarget(ctx context.Context, b backend.Backend, name string) (int, error) {
	dir, err := state.Dir()
	if err != nil {
		return 0, err
	}
	waiting, err := state.ResumePending.Exists(dir, name)
	if err != nil || !waiting {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, repaintBound)
	defer cancel()
	session, err := b.Session(ctx, name)
	if err != nil {
		return 0, err
	}
	if err := proveRepaintTarget(session.PID); err != nil {
		return 0, err
	}
	return session.PID, nil
}

// proveRepaintTarget refuses pid unless, read as it is called, it leads its
// own process group and runs a parked resume chain. It never passes pid 0 or
// 1, this process, or the group this process is in.
func proveRepaintTarget(pid int) error {
	if pid <= 1 || pid == os.Getpid() || pid == unix.Getpgrp() {
		return fmt.Errorf("the session names pid %d, which is never signalled", pid)
	}
	process, err := readSessionProcess(pid)
	switch {
	case err != nil:
		return err
	case process.Group != pid:
		return fmt.Errorf("pid %d doesn't lead its process group", pid)
	case !isParkedResumeChain(process.Argv):
		return fmt.Errorf("pid %d runs no resume panel", pid)
	}
	return nil
}
