package cmd

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

// initTestLogToStateDir wires the production internal/log handler so log
// records emitted via the package-level component loggers (daemonLogger,
// hydrateLogger, …) land in <dir>/portal.log with the pid/version/process_role
// baseline attrs injected — exactly as main -> log.Init does in the real
// binary. Used by tests that drive a command body directly (without going
// through main) yet assert on portal.log contents. processRole defaults to
// "daemon".
func initTestLogToStateDir(t *testing.T, dir, version string) {
	t.Helper()
	initTestLogToStateDirAs(t, dir, version, "daemon")
}

// initTestLogToStateDirAs is initTestLogToStateDir with an explicit
// process_role baseline. It ensures dir exists (log.Init's Phase-1 writer does
// not create parent directories) and brackets log.Init with a SetTestHandler
// snapshot-and-restore so the process-wide handler swap does not leak into
// sibling tests (t.Cleanup runs LIFO, so the pre-test handler is restored
// after log.Init's file handler).
func initTestLogToStateDirAs(t *testing.T, dir, version, processRole string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	log.SetTestHandler(t, slog.New(slog.NewTextHandler(io.Discard, nil)).Handler())
	if err := log.Init(dir, version, processRole); err != nil {
		t.Fatalf("log.Init: %v", err)
	}
}

// newCaptureLoggerForComponent is a thin wrapper over the shared
// logtest.Sink that binds the given component so it renders on every line
// (matching production text output). It returns the bound *slog.Logger plus
// the sink, so a cmd test can inject the logger into a *Deps / config struct
// and assert on the rendered body. The capture-handler base lives in
// internal/logtest.
func newCaptureLoggerForComponent(t *testing.T, component string) (*slog.Logger, *logtest.Sink) {
	t.Helper()
	sink := &logtest.Sink{}
	return slog.New(sink).With("component", component), sink
}
