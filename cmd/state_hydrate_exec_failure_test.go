package cmd

import (
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

// newSharedExecFailureCapture routes both the hydrate-component logger and
// log.Close's process-component marker through one sink, so their records
// interleave in emission order and a test can assert their relative ordering.
func newSharedExecFailureCapture(t *testing.T) (*logtest.Sink, *slog.Logger) {
	t.Helper()
	sink := logtest.Install(t)
	return sink, slog.New(sink).With("component", "hydrate")
}

func TestDefaultExecShell_ExecFailure_MarksTerminationBeforeExit(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")

	sink, logger := newSharedExecFailureCapture(t)
	prev := hydrateLogger
	hydrateLogger = logger
	t.Cleanup(func() { hydrateLogger = prev })

	// Snapshot at the instant osExit fires so the markers can be asserted as
	// already emitted, then panic to unwind: the real os.Exit would never let a
	// post-exit statement run.
	var linesAtExit []string
	var exitCode int32 = -1
	var exitCalls atomic.Int32
	withOsExitFake(t, func(code int) {
		exitCalls.Add(1)
		atomic.StoreInt32(&exitCode, int32(code))
		linesAtExit = sink.Lines()
		panic("osExit invoked")
	})

	func() {
		defer func() { _ = recover() }()
		// A path that cannot be exec'd, so syscall.Exec returns and drives the
		// fall-through under test.
		defaultExecShell("/nonexistent/portal-exec-failure-probe", []string{"sh"})
	}()

	if got := exitCalls.Load(); got != 1 {
		t.Fatalf("osExit invoked %d times; want exactly 1", got)
	}
	if got := atomic.LoadInt32(&exitCode); got != 1 {
		t.Fatalf("osExit code = %d; want 1 (non-zero exit preserved on exec failure)", got)
	}

	warnIdx := indexOfLineContaining(linesAtExit, "WARN", "component=hydrate")
	processExitIdx := indexOfLineContaining(linesAtExit, "exit", "component=process", "code=1")
	if warnIdx < 0 {
		t.Fatalf("exec-failure WARN not recorded before osExit; lines at exit:\n%s", strings.Join(linesAtExit, "\n"))
	}
	if processExitIdx < 0 {
		t.Fatalf("process: exit code=1 (via log.Close) not recorded before osExit; lines at exit:\n%s", strings.Join(linesAtExit, "\n"))
	}
	if warnIdx >= processExitIdx {
		t.Errorf("ordering violated: WARN at index %d, process: exit at index %d; want WARN FIRST then process: exit\nlines at exit:\n%s",
			warnIdx, processExitIdx, strings.Join(linesAtExit, "\n"))
	}
}

func indexOfLineContaining(lines []string, substrs ...string) int {
	for i, line := range lines {
		all := true
		for _, s := range substrs {
			if !strings.Contains(line, s) {
				all = false
				break
			}
		}
		if all {
			return i
		}
	}
	return -1
}
