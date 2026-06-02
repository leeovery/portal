// Tests in this file mutate package-level cobra command state and MUST NOT use t.Parallel.
//
// Coverage for the Phase 6 hydrate-helper forensic trail (Task
// portal-observability-layer-6-2): the FIFO-timeout exit-path INFO
// "hydrate: signal timeout took=3s" emitted inside handleHydrateTimeout,
// preceding the terminal "hydrate: exec" INFO on the timeout recovery path.
//
// Spec reference: § Hook-firing observability limit (Mechanical rule 3 —
// timeout row); § Subsystem prefix taxonomy (time.Duration rendering).
package cmd

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/tmux"
)

// signalTimeoutRecord returns the single captured record whose component=hydrate
// and msg="signal timeout", failing if not exactly one was emitted. It is a
// thin filter over the shared logtest.Sink so the test can assert the took
// attr's Kind (substring rendering cannot distinguish a slog.KindDuration took
// attr from a stringified one).
func signalTimeoutRecord(t *testing.T, sink *logtest.Sink) logtest.Record {
	t.Helper()
	var out []logtest.Record
	for _, r := range sink.Records() {
		comp, ok := r.Attrs["component"]
		if !ok || comp.String() != "hydrate" || r.Msg != "signal timeout" {
			continue
		}
		out = append(out, r)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 hydrate: signal timeout record, got %d: %+v", len(out), sink.Records())
	}
	return out[0]
}

func TestHydrateTimeoutLog_EmitsSignalTimeoutTookOnTimeoutPath(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-stl__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "stl:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	// Exactly one INFO "signal timeout" line, rendering took=3s (unquoted).
	info := execLogLine(t, sink.Body(), "INFO", "signal timeout")
	if !strings.Contains(info, "took=3s") {
		t.Errorf("signal timeout INFO missing took=3s: %q", info)
	}
}

func TestHydrateTimeoutLog_TookAttrIsDurationNotString(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-dur__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "dur:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	rec := signalTimeoutRecord(t, sink)
	took, ok := rec.Attrs["took"]
	if !ok {
		t.Fatalf("signal timeout record missing took attr: %+v", rec.Attrs)
	}
	if took.Kind() != slog.KindDuration {
		t.Errorf("took kind = %v, want Duration (must be passed as the hydrateTimeout time.Duration, not stringified)", took.Kind())
	}
	if took.Duration() != hydrateTimeout {
		t.Errorf("took = %v, want hydrateTimeout (%v)", took.Duration(), hydrateTimeout)
	}
}

func TestHydrateTimeoutLog_SignalTimeoutPrecedesExecINFO(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-ord__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	// Drive the full timeout branch: HandleTimeout (signal timeout INFO) → exec
	// (exec INFO). HookStore left nil → bare-shell exec via execShellAndExit.
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "ord:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	body := sink.Body()
	signalIdx := strings.Index(body, "INFO signal timeout")
	execIdx := strings.Index(body, "INFO exec")
	if signalIdx < 0 {
		t.Fatalf("no INFO signal timeout line: %q", body)
	}
	if execIdx < 0 {
		t.Fatalf("no INFO exec line: %q", body)
	}
	if signalIdx >= execIdx {
		t.Errorf("signal timeout INFO must precede the exec INFO; signalIdx=%d execIdx=%d body=%q", signalIdx, execIdx, body)
	}
}

func TestHydrateTimeoutLog_PreservesWarnUnlinkAndMarkerUnset(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-pre__0.0.fifo")

	cmder := &recordingCommander{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "pre:0.0", io.Discard, cmder, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	body := sink.Body()

	// Existing WARN still fires exactly once.
	if n := strings.Count(body, "timeout waiting for hydrate signal"); n != 1 {
		t.Errorf("want exactly one existing timeout WARN, got %d: %q", n, body)
	}

	// FIFO unlinked.
	if _, err := os.Stat(fifo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FIFO not removed on timeout; stat err = %v", err)
	}

	// Marker-unset attempted via `set-option -su @portal-skeleton-pre__0.0`.
	wantUnset := "set-option -su @portal-skeleton-pre__0.0"
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

func TestHydrateTimeoutLog_NilHandleTimeout_NoSignalTimeoutNoExec(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-nil__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	exec := &stubExecShell{}
	cfg := hydrateConfig{
		FIFO:      fifo,
		File:      filepath.Join(dir, "sb"),
		HookKey:   "nil:0.0",
		Stdout:    new(bytes.Buffer),
		Client:    tmux.NewClient(&recordingCommander{}),
		Logger:    logger,
		ExecShell: exec.fn(),
		OpenFIFO:  instantTimeoutOpenFIFO,
		// HandleTimeout intentionally left nil — test-only fall-through.
	}

	if err := runHydrate(cfg); err == nil {
		t.Fatal("runHydrate must return the timeout error when HandleTimeout is nil")
	}

	if exec.called {
		t.Error("ExecShell must NOT be called on the nil-HandleTimeout fall-through")
	}
	if strings.Contains(sink.Body(), "signal timeout") {
		t.Errorf("nil-HandleTimeout fall-through must NOT emit signal timeout: %q", sink.Body())
	}
}
