package cmd

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
)

// countLogLines matches on the "<LEVEL> <msg>" prefix so an attr value
// containing the phrase cannot false-match.
func countLogLines(body, level, msg string) int {
	prefix := level + " " + msg
	n := 0
	for line := range strings.SplitSeq(body, "\n") {
		if line == prefix || strings.HasPrefix(line, prefix+" ") {
			n++
		}
	}
	return n
}

// Scoped to the INFO line rather than the whole body: the per-cause WARNs also
// carry path=<file>, so a body-wide check could match one of those instead.
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
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: scrollback, HookKey: "fmle:0.0",
		OpenFIFO: openFIFOWithTimeout, HandleFileMissing: handleHydrateFileMissing, Logger: logger})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	info := scrollbackMissingINFO(t, sink.Body())
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
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: scrollback, HookKey: "fmlp:0.0",
		OpenFIFO: openFIFOWithTimeout, HandleFileMissing: handleHydrateFileMissing, Logger: logger})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	body := sink.Body()
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
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "fmlg:0.0",
		OpenFIFO: unexpectedOpenFIFO(t),
		Stdout:   stdout,
		Logger:   logger,
	})

	genericErr := errors.New("synthetic generic I/O failure")
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: genericErr}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}

	body := sink.Body()
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
	stdout.WriteString(hydrateResetPreamble)
	stdout.WriteString("partial-bytes")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:     fifo,
		File:     scrollback,
		HookKey:  "fmlm:0.0",
		OpenFIFO: unexpectedOpenFIFO(t),
		Stdout:   stdout,
		Logger:   logger,
	})

	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: errors.New("read: I/O error")}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}

	body := sink.Body()
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
	cfg := hydrateCfg(t, hydrateCfgOpts{FIFO: fifo, File: scrollback, HookKey: "fmord:0.0",
		OpenFIFO: openFIFOWithTimeout, HandleFileMissing: handleHydrateFileMissing, Logger: logger,
		// A nil HookStore takes the bare-shell exec path, which emits the exec INFO.
		HookStore: nil})

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	body := sink.Body()

	info := scrollbackMissingINFO(t, body)
	if !strings.Contains(info, "path="+scrollback) {
		t.Errorf("scrollback missing INFO path must equal cfg.File=%s: %q", scrollback, info)
	}
	if strings.Contains(info, "target=") {
		t.Errorf("scrollback missing INFO must use the reserved path attr, not target: %q", info)
	}

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

	cmder := commandertest.Quiet()
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   "fmpre:0.0",
		OpenFIFO:  unexpectedOpenFIFO(t),
		Stdout:    stdout,
		Commander: cmder,
		Logger:    logger,
	})

	start := time.Now()
	if err := handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: fs.ErrNotExist}); err != nil {
		t.Fatalf("handleHydrateFileMissing: %v", err)
	}
	elapsed := time.Since(start)

	body := sink.Body()

	if n := strings.Count(body, "scrollback file not found"); n != 1 {
		t.Errorf("want exactly one per-cause ENOENT WARN, got %d: %q", n, body)
	}
	if n := countLogLines(body, "INFO", "scrollback missing"); n != 1 {
		t.Errorf("want exactly one additive scrollback missing INFO, got %d: %q", n, body)
	}

	if elapsed >= 100*time.Millisecond {
		t.Errorf("handleHydrateFileMissing elapsed %v; expected << 100ms (no settle sleep)", elapsed)
	}
}

// A missing FIFO makes os.OpenFile return ENOENT immediately, and that branch
// hard-returns rather than exec'ing — a distinct exit path from the timeout one.
func TestHydrateFifoMissingLog_EmitsFifoMissingPathOnNonTimeoutOpenError(t *testing.T) {
	dir := t.TempDir()
	// Deliberately not created, so the open fails immediately.
	fifo := filepath.Join(dir, "hydrate-fifo__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	exec := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:              fifo,
		File:              filepath.Join(dir, "sb"),
		HookKey:           "fifo:0.0",
		OpenFIFO:          openFIFOWithTimeout,
		Logger:            logger,
		ExecShell:         exec.fn(),
		HandleFileMissing: handleHydrateFileMissing,
		HandleTimeout:     handleHydrateTimeout,
	})

	err := runHydrate(cfg)
	if err == nil {
		t.Fatal("runHydrate must return the open-fifo error on a missing FIFO (hard return, no exec)")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected returned error to wrap ENOENT, got %v", err)
	}

	body := sink.Body()
	info := execLogLine(t, body, "INFO", "fifo missing")
	if !strings.Contains(info, "path="+fifo) {
		t.Errorf("fifo missing INFO missing path=%s: %q", fifo, info)
	}

	if exec.called {
		t.Error("missing-FIFO path must NOT exec a shell (it hard-returns)")
	}
	if strings.Contains(body, "signal timeout") {
		t.Errorf("missing-FIFO path must NOT collapse into signal timeout: %q", body)
	}
}
