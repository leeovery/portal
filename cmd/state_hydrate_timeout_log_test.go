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
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// durationCaptureSink records the slog.Value of every emitted record's attrs
// (including the component bound via WithAttrs) so a test can assert an attr's
// Kind — substring rendering is too lossy to distinguish a slog.KindDuration
// took attr from a stringified one. Mirrors captureSummarySink (Task 5-1) but
// scoped to the timeout-log tests.
type durationCaptureSink struct {
	mu      sync.Mutex
	records []durationCaptureRecord
	shared  *durationCaptureSink
	bound   []slog.Attr
}

type durationCaptureRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
}

func (s *durationCaptureSink) owner() *durationCaptureSink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *durationCaptureSink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *durationCaptureSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &durationCaptureSink{shared: s.owner(), bound: next}
}

func (s *durationCaptureSink) WithGroup(_ string) slog.Handler {
	return &durationCaptureSink{shared: s.owner(), bound: s.bound}
}

func (s *durationCaptureSink) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())
	for _, a := range s.bound {
		attrs[a.Key] = a.Value
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	rec := durationCaptureRecord{level: r.Level, msg: r.Message, attrs: attrs}
	owner := s.owner()
	owner.mu.Lock()
	owner.records = append(owner.records, rec)
	owner.mu.Unlock()
	return nil
}

func (s *durationCaptureSink) all() []durationCaptureRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]durationCaptureRecord, len(s.records))
	copy(out, s.records)
	return out
}

// signalTimeoutRecord returns the single record whose component=hydrate and
// msg="signal timeout". Fails if not exactly one was emitted.
func (s *durationCaptureSink) signalTimeoutRecord(t *testing.T) durationCaptureRecord {
	t.Helper()
	var out []durationCaptureRecord
	for _, r := range s.all() {
		comp, ok := r.attrs["component"]
		if !ok || comp.String() != "hydrate" || r.msg != "signal timeout" {
			continue
		}
		out = append(out, r)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 hydrate: signal timeout record, got %d: %+v", len(out), s.all())
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

	sink := &durationCaptureSink{}
	logger := slog.New(sink).With("component", "hydrate")
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "dur:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	rec := sink.signalTimeoutRecord(t)
	took, ok := rec.attrs["took"]
	if !ok {
		t.Fatalf("signal timeout record missing took attr: %+v", rec.attrs)
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
