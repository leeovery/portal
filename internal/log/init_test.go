package log

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// snapshotInitState captures the package-private Init-owned state (handler +
// startTime) and returns a restore func so Init-exercising tests do not leak
// state into siblings. setHandler is restored via the shared snapshotHandler;
// startTime is restored directly because it is package-private to this _test.go.
func snapshotInitState(t *testing.T) {
	t.Helper()
	restoreHandler := snapshotHandler()
	prevStart := startTime
	t.Cleanup(func() {
		restoreHandler()
		startTime = prevStart
	})
}

func TestInit_RoutesPreInitCachedLoggerToConfiguredHandler(t *testing.T) {
	snapshotInitState(t)

	// Cache a logger BEFORE Init, mirroring package-init binding.
	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	cached.Info("after init")

	line := readPortalLog(t, dir)
	if !strings.Contains(line, " daemon: after init ") {
		t.Errorf("expected component prefix from cached logger, got: %q", line)
	}
	for _, want := range []string{"pid=", "version=0.5.0", "process_role=tui"} {
		if !strings.Contains(line, want) {
			t.Errorf("expected baseline %q on cached-logger line, got: %q", want, line)
		}
	}
	wantPID := "pid=" + strconv.Itoa(os.Getpid())
	if !strings.Contains(line, wantPID) {
		t.Errorf("expected captured pid baseline %q, got: %q", wantPID, line)
	}
}

func TestInit_AppliesResolvedLevelFromEnv(t *testing.T) {
	snapshotInitState(t)
	t.Setenv("PORTAL_LOG_LEVEL", "error")

	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	cached.Info("info-suppressed")
	cached.Error("error-emitted")

	line := readPortalLog(t, dir)
	if strings.Contains(line, "info-suppressed") {
		t.Errorf("INFO must be suppressed when resolved level is error, got: %q", line)
	}
	if !strings.Contains(line, "error-emitted") {
		t.Errorf("ERROR must be emitted when resolved level is error, got: %q", line)
	}
}

func TestInit_SecondInitRePointsHandlerWithoutPanic(t *testing.T) {
	snapshotInitState(t)

	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("first Init returned error: %v", err)
	}

	// Second Init with a different process_role must re-point without panicking.
	dir2 := t.TempDir()
	if err := Init(dir2, "0.5.0", "daemon"); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}

	cached.Info("after second init")

	line := readPortalLog(t, dir2)
	if !strings.Contains(line, "process_role=daemon") {
		t.Errorf("expected new process_role baseline after second Init, got: %q", line)
	}
	if strings.Contains(line, "process_role=tui") {
		t.Errorf("must not carry stale process_role after re-point, got: %q", line)
	}
}

func TestInit_CapturesStartTimeAndCloseComputesNonNegativeTook(t *testing.T) {
	snapshotInitState(t)

	before := time.Now()
	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	after := time.Now()

	if startTime.Before(before) || startTime.After(after) {
		t.Fatalf("startTime %v not captured within Init window [%v, %v]", startTime, before, after)
	}

	took := computeTook()
	if took < 0 {
		t.Errorf("computeTook returned negative duration %v", took)
	}
}

func TestInit_SecondInitResetsStartTime(t *testing.T) {
	snapshotInitState(t)

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("first Init returned error: %v", err)
	}
	first := startTime

	// Force an observable gap, then re-Init.
	startTime = time.Time{}.Add(time.Hour) // sentinel distinct from any real now
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}

	if !startTime.After(first) {
		t.Errorf("second Init must reset startTime to a later instant; first=%v second=%v", first, startTime)
	}
	if startTime.Equal(time.Time{}.Add(time.Hour)) {
		t.Error("second Init did not overwrite the sentinel startTime")
	}
}

func TestClose_ReturnsWithoutTerminatingProcess(t *testing.T) {
	snapshotInitState(t)

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// If Close called os.Exit, this test process would terminate and this test
	// would be reported as failed-to-complete (its t.Cleanup would not run, and
	// every sibling test would be skipped). Returning normally from the test
	// function is itself the proof that Close owns no control flow.
	Close(0)
}

func TestClose_SafeBeforeAnyInit(t *testing.T) {
	snapshotInitState(t)

	// Reset startTime to its zero value to model a never-Init'd process.
	startTime = time.Time{}

	// Must not panic.
	Close(0)
}

func TestInit_WritesThroughDateAwareSinkToDatedFileAndSymlink(t *testing.T) {
	snapshotInitState(t)

	day := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	cached := For("daemon")

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "tui"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	cached.Info("dated")

	// The record must land in the date-keyed file, proving Init wired the
	// date-aware rotating sink (not the Phase-1 plain portal.log open).
	datedPath := filepath.Join(dir, "portal.log.2026-05-29")
	b, err := os.ReadFile(datedPath)
	if err != nil {
		t.Fatalf("reading dated file %s: %v", datedPath, err)
	}
	if !strings.Contains(string(b), " daemon: dated ") {
		t.Errorf("expected record in dated file, got: %q", string(b))
	}

	// portal.log must be the live-target symlink pointing at today's file.
	target, err := os.Readlink(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("readlink portal.log: %v", err)
	}
	if filepath.Base(target) != "portal.log.2026-05-29" {
		t.Errorf("portal.log symlink target = %q, want portal.log.2026-05-29", target)
	}
}

func TestInit_FallsBackToStderrAndReturnsErrorOnOpenFailure(t *testing.T) {
	snapshotInitState(t)

	// A stateDir that cannot hold the day file (a regular file in the path)
	// forces the eager open probe to fail; Init must surface the error
	// advisorily and still install a usable (stderr-fallback) handler.
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	badDir := filepath.Join(blocker, "state") // path component is a regular file.

	if err := Init(badDir, "0.5.0", "tui"); err == nil {
		t.Error("expected advisory open error from Init on an unwritable stateDir, got nil")
	}

	// The handler must still be usable (no panic) even after the open failure.
	For("daemon").Info("after-failure")
}

func TestInit_DoesNotImportInternalState(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, imp := range af.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasSuffix(path, "internal/state") {
				t.Errorf("%s imports %q — internal/log must not depend on internal/state (import-cycle guard)", f, path)
			}
		}
	}
}

// readPortalLog reads the portal.log written under dir, failing the test if it
// is missing or empty. Returns the full file contents.
func readPortalLog(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("reading portal.log under %s: %v", dir, err)
	}
	if len(b) == 0 {
		t.Fatalf("portal.log under %s is empty", dir)
	}
	return string(b)
}
