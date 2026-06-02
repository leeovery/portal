// Tests in this file mutate package-level cobra command state and MUST NOT use t.Parallel.
//
// Coverage for the Phase 6 hydrate-helper forensic trail (Task
// portal-observability-layer-6-3): the file-missing exit-path INFO
// "hydrate: scrollback missing path=<file>" emitted inside
// handleHydrateFileMissing, covering ALL causes (ENOENT, permission, generic
// I/O, mid-stream io.Copy failure) with a single INFO, plus the FIFO-absence
// exit-path INFO "hydrate: fifo missing path=<fifo>" emitted at the
// non-timeout open-error branch in runHydrate.
//
// fifo-missing resolution (the task's [needs-info]): the live FIFO open
// (openFIFOWithTimeout, O_RDONLY blocking) yields ErrHydrateTimeout as its
// only timeout-class non-success outcome — but a MISSING FIFO makes
// os.OpenFile return ENOENT immediately, a distinct non-timeout error the
// select surfaces verbatim. runHydrate's non-timeout open-error branch
// (`return fmt.Errorf("open fifo %s: %w", ...)`) is therefore a still-live,
// distinct exit path. It is wired with the `fifo missing` INFO. NOTE: that
// branch HARD-RETURNS (it does NOT exec a shell), so this INFO is a non-exec
// exit-path INFO and diverges from the spec's "then exec" framing for the
// `fifo missing` row.
//
// Spec reference: § Hook-firing observability limit (Mechanical rule 3 —
// scrollback-missing & fifo-missing rows; Mechanical rule 2 — `path` reserved).
package cmd

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
)

// fileMissingCfg builds a hydrateConfig wired for the production file-missing
// path: OpenFIFO is the real blocking open (unblocked by signalFIFOAsync in
// the caller) and HandleFileMissing points at handleHydrateFileMissing. The
// scrollback File is supplied by the caller so each test controls the cause
// (absent file → ENOENT, chmod 000 → permission, etc.).
func fileMissingCfg(t *testing.T, fifo, scrollback, hookKey string, stdout io.Writer, cmder *recordingCommander, exec func(string, []string), logger *slog.Logger) hydrateConfig {
	t.Helper()
	return hydrateConfig{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           hookKey,
		Stdout:            stdout,
		Client:            tmux.NewClient(cmder),
		Logger:            logger,
		ExecShell:         exec,
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}
}

// countLogLines counts captured lines whose message is the given terse phrase,
// matched on the "<LEVEL> <msg>" prefix (mirrors execLogLine's matching so an
// attr value containing the phrase cannot false-match).
func countLogLines(body, level, msg string) int {
	prefix := level + " " + msg
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if line == prefix || strings.HasPrefix(line, prefix+" ") {
			n++
		}
	}
	return n
}

// scrollbackMissingINFO returns the single "INFO scrollback missing ..." line.
// Scoping the assertion to the INFO line (not the body) is load-bearing: the
// per-cause WARNs also carry path=<file>, so a body-wide path= check could not
// prove the INFO itself carries the reserved attr.
func scrollbackMissingINFO(t *testing.T, body string) string {
	t.Helper()
	return execLogLine(t, body, "INFO", "scrollback missing")
}

func TestHydrateFileMissingLog_ENOENT_EmitsScrollbackMissingPath(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fmle__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := fileMissingCfg(t, fifo, scrollback, "fmle:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	info := scrollbackMissingINFO(t, sink.body())
	if !strings.Contains(info, "path="+scrollback) {
		t.Errorf("scrollback missing INFO missing path=%s: %q", scrollback, info)
	}
}

func TestHydrateFileMissingLog_Permission_EmitsOneScrollbackMissingINFO(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fmlp__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, []byte("HIDDEN"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}
	if err := os.Chmod(scrollback, 0o000); err != nil {
		t.Fatalf("chmod 0: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(scrollback, 0o600) })

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := fileMissingCfg(t, fifo, scrollback, "fmlp:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	body := sink.body()
	// Exactly one INFO scrollback missing — not one per cause.
	if n := countLogLines(body, "INFO", "scrollback missing"); n != 1 {
		t.Fatalf("want exactly one INFO scrollback missing, got %d: %q", n, body)
	}
	info := scrollbackMissingINFO(t, body)
	if !strings.Contains(info, "path="+scrollback) {
		t.Errorf("scrollback missing INFO missing path=%s: %q", scrollback, info)
	}
}

func TestHydrateFileMissingLog_GenericIO_EmitsOneScrollbackMissingINFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-fmlg__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	stdout := new(bytes.Buffer)
	stdout.WriteString(hydrateResetPreamble)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fmlg:0.0",
		Stdout: stdout,
		Client: tmux.NewClient(&recordingCommander{}),
		Logger: logger,
	}

	genericErr := errors.New("synthetic generic I/O failure")
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: genericErr}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}

	body := sink.body()
	if n := countLogLines(body, "INFO", "scrollback missing"); n != 1 {
		t.Fatalf("want exactly one INFO scrollback missing for generic I/O, got %d: %q", n, body)
	}
	info := scrollbackMissingINFO(t, body)
	if !strings.Contains(info, "path="+scrollback) {
		t.Errorf("scrollback missing INFO missing path=%s: %q", scrollback, info)
	}
}

func TestHydrateFileMissingLog_MidStreamCopy_SharesScrollbackMissingINFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-fmlm__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	stdout := new(bytes.Buffer)
	// Simulate runHydrate having already written the preamble + partial bytes
	// before the mid-stream io.Copy failure routed here.
	stdout.WriteString(hydrateResetPreamble)
	stdout.WriteString("partial-bytes")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fmlm:0.0",
		Stdout: stdout,
		Client: tmux.NewClient(&recordingCommander{}),
		Logger: logger,
	}

	// A mid-stream io.Copy failure routes through the same handler with a
	// generic Cause (not ENOENT/permission).
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: errors.New("read: I/O error")}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}

	body := sink.body()
	if n := countLogLines(body, "INFO", "scrollback missing"); n != 1 {
		t.Fatalf("want exactly one INFO scrollback missing for mid-stream copy failure, got %d: %q", n, body)
	}
	info := scrollbackMissingINFO(t, body)
	if !strings.Contains(info, "path="+scrollback) {
		t.Errorf("scrollback missing INFO missing path=%s: %q", scrollback, info)
	}
}

func TestHydrateFileMissingLog_PathAttrIsFileAndPrecedesExecINFO(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fmord__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	// HookStore left nil → bare-shell exec via execShellAndExit (emits exec INFO).
	cfg := fileMissingCfg(t, fifo, scrollback, "fmord:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	body := sink.body()

	// path attr value is cfg.File, NOT the exec INFO's target= attr.
	info := scrollbackMissingINFO(t, body)
	if !strings.Contains(info, "path="+scrollback) {
		t.Errorf("scrollback missing INFO path must equal cfg.File=%s: %q", scrollback, info)
	}
	if strings.Contains(info, "target=") {
		t.Errorf("scrollback missing INFO must use the reserved path attr, not target: %q", info)
	}

	// Ordering: scrollback missing INFO precedes the exec INFO.
	scrollbackIdx := strings.Index(body, "INFO scrollback missing")
	execIdx := strings.Index(body, "INFO exec")
	if scrollbackIdx < 0 {
		t.Fatalf("no INFO scrollback missing line: %q", body)
	}
	if execIdx < 0 {
		t.Fatalf("no INFO exec line: %q", body)
	}
	if scrollbackIdx >= execIdx {
		t.Errorf("scrollback missing INFO must precede the exec INFO; scrollbackIdx=%d execIdx=%d body=%q", scrollbackIdx, execIdx, body)
	}
}

func TestHydrateFileMissingLog_PreservesPerCauseWARNsAndNoSettleSleep(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "hydrate-fmpre__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	stdout := new(bytes.Buffer)
	stdout.WriteString(hydrateResetPreamble)

	cmder := &recordingCommander{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateConfig{
		FIFO: fifo, File: scrollback, HookKey: "fmpre:0.0",
		Stdout: stdout,
		Client: tmux.NewClient(cmder),
		Logger: logger,
	}

	// ENOENT cause → the "scrollback file not found" WARN must still fire.
	start := time.Now()
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: fs.ErrNotExist}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}
	elapsed := time.Since(start)

	body := sink.body()

	// Per-cause WARN retained exactly once.
	if n := strings.Count(body, "scrollback file not found"); n != 1 {
		t.Errorf("want exactly one per-cause ENOENT WARN, got %d: %q", n, body)
	}
	// The additive INFO is present alongside the WARN.
	if n := countLogLines(body, "INFO", "scrollback missing"); n != 1 {
		t.Errorf("want exactly one additive scrollback missing INFO, got %d: %q", n, body)
	}

	// No-settle-sleep posture preserved (the handler must not sleep).
	if elapsed >= 100*time.Millisecond {
		t.Errorf("handleHydrateFileMissing elapsed %v; expected << 100ms (no settle sleep)", elapsed)
	}

	// Marker-unset still attempted via set-option -su.
	wantUnset := "set-option -su @portal-skeleton-fmpre__0.0"
	found := false
	for _, c := range cmder.Calls {
		if strings.Join(c, " ") == wantUnset {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected marker-unset call %q; calls: %v", wantUnset, cmder.Calls)
	}
}

// TestHydrateFifoMissingLog_EmitsFifoMissingPathOnNonTimeoutOpenError pins the
// [needs-info] resolution (1): a MISSING FIFO is a distinct, live, non-timeout
// open-error exit path. os.OpenFile returns ENOENT immediately (NOT a timeout
// block), so runHydrate's non-timeout open-error branch fires. That branch
// HARD-RETURNS the error — it does NOT exec — so this INFO is a non-exec
// exit-path INFO (diverges from the spec's "then exec" framing; documented in
// the file header and the SUMMARY).
func TestHydrateFifoMissingLog_EmitsFifoMissingPathOnNonTimeoutOpenError(t *testing.T) {
	dir := t.TempDir()
	// Do NOT create the FIFO — os.OpenFile returns ENOENT immediately.
	fifo := filepath.Join(dir, "hydrate-fifo__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO: fifo, File: filepath.Join(dir, "sb"), HookKey: "fifo:0.0",
		Stdout:            io.Discard,
		Client:            tmux.NewClient(&recordingCommander{}),
		Logger:            logger,
		ExecShell:         exec.fn(),
		OpenFIFO:          openFIFOWithTimeout,
		HandleTimeout:     handleHydrateTimeout,
		HandleFileMissing: handleHydrateFileMissing,
	}

	err := runHydrate(cfg)
	if err == nil {
		t.Fatal("runHydrate must return the open-fifo error on a missing FIFO (hard return, no exec)")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected returned error to wrap ENOENT, got %v", err)
	}

	// The fifo-missing INFO carries path=<cfg.FIFO>.
	body := sink.body()
	info := execLogLine(t, body, "INFO", "fifo missing")
	if !strings.Contains(info, "path="+fifo) {
		t.Errorf("fifo missing INFO missing path=%s: %q", fifo, info)
	}

	// This path does NOT exec (hard return) and does NOT emit the timeout INFO.
	if exec.called {
		t.Error("missing-FIFO path must NOT exec a shell (it hard-returns)")
	}
	if strings.Contains(body, "signal timeout") {
		t.Errorf("missing-FIFO path must NOT collapse into signal timeout: %q", body)
	}
}
