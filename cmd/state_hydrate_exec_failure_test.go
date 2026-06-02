// Tests in this file mutate the package-level osExit seam and the process-wide
// log handler (via log.SetTestHandler) and MUST NOT use t.Parallel.
//
// Coverage for portal-observability-layer Task 8-3: the hydrate exec-failure
// fall-through in defaultExecShell. syscall.Exec only returns on error; on that
// rare path the helper must NOT vanish via an unmarked bare os.Exit — it must
// pair a terminal marker (a WARN naming the exec failure, then log.Close(1)
// emitting "process: exit code=1") before routing through the osExit seam,
// mirroring the daemon self-eject's marked-termination discipline (spec §
// Defensive invariants — the prohibition on bare os.Exit outside main, plus its
// single sanctioned exception).
package cmd

import (
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

// newSharedExecFailureCapture installs a single capture sink as the
// process-wide log handler (via log.SetTestHandler) and returns the sink plus a
// hydrate-component logger bound over that same sink. Records emitted by the
// returned logger (the exec-failure WARN) and by log.Close(1) -> the process
// component (process: exit) interleave in the one sink.lines buffer in emission
// order, so a test can assert their relative ordering.
func newSharedExecFailureCapture(t *testing.T) (*logtest.Sink, *slog.Logger) {
	t.Helper()
	sink := &logtest.Sink{}
	// Route both log.For("process") (process: exit via log.Close) and the
	// hydrate-component WARN through the same sink.
	log.SetTestHandler(t, sink)
	return sink, slog.New(sink).With("component", "hydrate")
}

// TestDefaultExecShell_ExecFailure_MarksTerminationBeforeExit drives the
// exec-failure fall-through with a prog that cannot be exec'd (a non-existent
// absolute path → syscall.Exec returns ENOENT). With osExit stubbed (so the
// test process survives) and the log handler captured, it asserts the
// termination is MARKED: a WARN naming the exec failure, a paired
// "process: exit code=1" terminal marker (via log.Close), and a single
// osExit(1) call — and that the markers land BEFORE osExit fires.
func TestDefaultExecShell_ExecFailure_MarksTerminationBeforeExit(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")

	sink, logger := newSharedExecFailureCapture(t)
	// defaultExecShell logs through the package-level hydrateLogger; point it at
	// the shared sink for the duration of the test so the WARN is captured.
	prev := hydrateLogger
	hydrateLogger = logger
	t.Cleanup(func() { hydrateLogger = prev })

	// At the instant osExit fires, snapshot the captured lines so we can assert
	// both the WARN and the process: exit marker were already emitted (in order)
	// before the exit call. Panic to unwind out of defaultExecShell — the real
	// os.Exit would have terminated the process, so the stub must not fall
	// through and let any post-exit statement run.
	var linesAtExit []string
	var exitCode int32 = -1
	var exitCalls int32
	withOsExitFake(t, func(code int) {
		atomic.AddInt32(&exitCalls, 1)
		atomic.StoreInt32(&exitCode, int32(code))
		linesAtExit = sink.Lines()
		panic("osExit invoked")
	})

	func() {
		defer func() { _ = recover() }()
		// A path that cannot be exec'd → syscall.Exec returns a non-nil error,
		// driving the exec-failure fall-through under test.
		defaultExecShell("/nonexistent/portal-exec-failure-probe", []string{"sh"})
	}()

	if got := atomic.LoadInt32(&exitCalls); got != 1 {
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
