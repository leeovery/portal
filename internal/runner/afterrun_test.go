package runner_test

import (
	"context"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/runner"
)

// afterRunEnv switches the test binary into one attach to a fake child,
// detached by key, after which it sends itself SIGINT. Unless Run left the
// signal caught, that ends it; it exits 0 only if it survives.
const afterRunEnv = "PORTAL_RUNNER_TEST_AFTER_RUN"

func interruptAfterRun() int {
	user, tty, err := pty.Open()
	if err != nil {
		return 100
	}
	if err := pty.Setsize(user, &pty.Winsize{Cols: 80, Rows: 24}); err != nil {
		return 100
	}
	exe, err := os.Executable()
	if err != nil {
		return 100
	}
	go detachOnceReady(user)
	_, err = runner.Run(context.Background(), runner.Config{
		Session: "fake",
		Command: backend.Command{Argv: []string{exe}, Env: append(os.Environ(), childModeEnv+"="+modeZmx)},
		In:      tty,
		Out:     tty,
		Scanner: keys,
	})
	if err != nil {
		return 101
	}
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	time.Sleep(settle)
	return 0
}

// detachOnceReady presses the detach key once the fake child says it is
// ready, then takes the rest of the terminal's output.
func detachOnceReady(user *os.File) {
	var seen []byte
	buf := make([]byte, 4096)
	for !readyLine.Match(seen) {
		n, err := user.Read(buf)
		if err != nil {
			return
		}
		seen = append(seen, buf[:n]...)
	}
	_, _ = user.Write([]byte(keyDetach))
	_, _ = io.Copy(io.Discard, user)
}
