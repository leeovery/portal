// Tests in this file mutate package-level cobra command state and MUST NOT use t.Parallel.
//
// Coverage for the Phase 6 hydrate-helper forensic trail (Task
// portal-observability-layer-6-4): the success exit-path INFO
// "hydrate: scrollback replayed bytes=N took=T" emitted on runHydrate's
// signal-arrived path — after the postamble write + 100ms settle sleep +
// marker-unset and before the terminal "hydrate: exec" INFO (Task 6-1).
//
// bytes is the exact io.Copy byte count (0 for an empty scrollback, the file
// size for a populated one); took is the measured replay (copy) duration,
// rendered as a time.Duration (NOT the settle sleep, NOT a quoted string).
//
// Spec reference: § Hook-firing observability limit (Mechanical rule 3 —
// success row: `Info("scrollback replayed", "bytes", n, "took", took)` then
// exec); § Subsystem prefix taxonomy (Hydrate attr group — `bytes`; text-mode
// time.Duration rendering of `took`).
package cmd

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/tmux"
)

// replayCfg builds a hydrateConfig wired for the production success path: the
// real blocking FIFO open (unblocked by signalFIFOAsync in the caller) and a
// populated scrollback File the caller controls. HookStore left nil → bare
// shell exec via execShellAndExit (emits the exec INFO).
func replayCfg(t *testing.T, fifo, scrollback, hookKey string, stdout io.Writer, exec func(string, []string), logger *slog.Logger) hydrateConfig {
	t.Helper()
	return hydrateConfig{
		FIFO:              fifo,
		File:              scrollback,
		HookKey:           hookKey,
		Stdout:            stdout,
		Client:            tmux.NewClient(&recordingCommander{}),
		Logger:            logger,
		ExecShell:         exec,
		OpenFIFO:          openFIFOWithTimeout,
		HandleFileMissing: handleHydrateFileMissing,
		HandleTimeout:     handleHydrateTimeout,
	}
}

func TestHydrateReplayedLog_EmitsScrollbackReplayedBytesTookOnSuccessPath(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-rep__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	payload := []byte("line1\nline2\nline3\n")
	if err := os.WriteFile(scrollback, payload, 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := replayCfg(t, fifo, scrollback, "rep:0.0", io.Discard, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	info := execLogLine(t, sink.Body(), "INFO", "scrollback replayed")
	if !strings.Contains(info, fmt.Sprintf("bytes=%d", len(payload))) {
		t.Errorf("scrollback replayed INFO missing bytes=%d: %q", len(payload), info)
	}
	if !strings.Contains(info, "took=") {
		t.Errorf("scrollback replayed INFO missing took=: %q", info)
	}
}

func TestHydrateReplayedLog_BytesEqualsCopyCountForPopulatedFile(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-pop__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	// Include NUL / non-UTF8 / escape bytes so the count is the verbatim byte
	// length, not a rune count.
	payload := []byte("line1\r\nline2\x00\xff\x1b[31mred\x1b[0m")
	if err := os.WriteFile(scrollback, payload, 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := replayCfg(t, fifo, scrollback, "pop:0.0", io.Discard, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	info := execLogLine(t, sink.Body(), "INFO", "scrollback replayed")
	if !strings.Contains(info, fmt.Sprintf("bytes=%d", len(payload))) {
		t.Errorf("bytes must equal io.Copy count (%d): %q", len(payload), info)
	}
}

func TestHydrateReplayedLog_ZeroByteScrollbackEmitsBytesZero(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-zero__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, []byte(""), 0o600); err != nil {
		t.Fatalf("seed empty scrollback: %v", err)
	}

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := replayCfg(t, fifo, scrollback, "zero:0.0", io.Discard, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	// An empty replay is still a successful rehydration — the INFO still fires.
	info := execLogLine(t, sink.Body(), "INFO", "scrollback replayed")
	if !strings.Contains(info, "bytes=0") {
		t.Errorf("zero-byte scrollback must emit bytes=0: %q", info)
	}
}

func TestHydrateReplayedLog_FiveMegabyteFileReportsExactByteCount(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-big__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	const size = 5 * 1024 * 1024
	payload := bytes.Repeat([]byte("A"), size)
	if err := os.WriteFile(scrollback, payload, 0o600); err != nil {
		t.Fatalf("seed 5MB scrollback: %v", err)
	}

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := replayCfg(t, fifo, scrollback, "big:0.0", io.Discard, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	info := execLogLine(t, sink.Body(), "INFO", "scrollback replayed")
	if !strings.Contains(info, fmt.Sprintf("bytes=%d", size)) {
		t.Errorf("5MB file must report exact byte count bytes=%d: %q", size, info)
	}
}

// scrollbackReplayedRecord returns the single captured record whose
// component=hydrate and msg="scrollback replayed", failing if not exactly one
// was emitted. It is a thin filter over the shared logtest.Sink so the test can
// assert the took attr's Kind (substring rendering cannot distinguish a
// slog.KindDuration took attr from a stringified one).
func scrollbackReplayedRecord(t *testing.T, sink *logtest.Sink) logtest.Record {
	t.Helper()
	var out []logtest.Record
	for _, r := range sink.Records() {
		comp, ok := r.Attrs["component"]
		if !ok || comp.String() != "hydrate" || r.Msg != "scrollback replayed" {
			continue
		}
		out = append(out, r)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 hydrate: scrollback replayed record, got %d: %+v", len(out), sink.Records())
	}
	return out[0]
}

func TestHydrateReplayedLog_TookIsDurationAcrossReplayNotSettleSleep(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-dur__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, []byte("CONTENT"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := replayCfg(t, fifo, scrollback, "dur:0.0", io.Discard, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	rec := scrollbackReplayedRecord(t, sink)
	took, ok := rec.Attrs["took"]
	if !ok {
		t.Fatalf("scrollback replayed record missing took attr: %+v", rec.Attrs)
	}
	if took.Kind() != slog.KindDuration {
		t.Errorf("took kind = %v, want Duration (must be the measured time.Duration, not stringified)", took.Kind())
	}
	// took is measured across the io.Copy only — it must NOT include the 100ms
	// settle sleep. A tiny in-memory copy completes far under 100ms.
	if took.Duration() >= hydrateSettleSleep {
		t.Errorf("took = %v, must be the copy duration (well under the %v settle sleep), not the settle sleep", took.Duration(), hydrateSettleSleep)
	}
}

func TestHydrateReplayedLog_PrecedesExecINFOAndFiresOnce(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-ord__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, []byte("DUMP"), 0o600); err != nil {
		t.Fatalf("seed scrollback: %v", err)
	}

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := replayCfg(t, fifo, scrollback, "ord:0.0", io.Discard, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	body := sink.Body()

	// Fires exactly once.
	if n := countLogLines(body, "INFO", "scrollback replayed"); n != 1 {
		t.Fatalf("want exactly one INFO scrollback replayed, got %d: %q", n, body)
	}

	// Ordering: scrollback replayed INFO precedes the exec INFO.
	replayedIdx := strings.Index(body, "INFO scrollback replayed")
	execIdx := strings.Index(body, "INFO exec")
	if replayedIdx < 0 {
		t.Fatalf("no INFO scrollback replayed line: %q", body)
	}
	if execIdx < 0 {
		t.Fatalf("no INFO exec line: %q", body)
	}
	if replayedIdx >= execIdx {
		t.Errorf("scrollback replayed INFO must precede the exec INFO; replayedIdx=%d execIdx=%d body=%q", replayedIdx, execIdx, body)
	}
}

func TestHydrateReplayedLog_NotEmittedOnTimeoutPath(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-not__0.0.fifo")

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := timeoutCfg(t, fifo, filepath.Join(dir, "sb"), "not:0.0", io.Discard, &recordingCommander{}, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if strings.Contains(sink.Body(), "scrollback replayed") {
		t.Errorf("timeout path must NOT emit scrollback replayed: %q", sink.Body())
	}
}

func TestHydrateReplayedLog_NotEmittedOnFileMissingPath(t *testing.T) {
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-fm__0.0.fifo")
	scrollback := filepath.Join(dir, "missing-sb")

	signalFIFOAsync(t, fifo)

	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := replayCfg(t, fifo, scrollback, "fm:0.0", io.Discard, (&stubExecShell{}).fn(), logger)

	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}

	if strings.Contains(sink.Body(), "scrollback replayed") {
		t.Errorf("file-missing path must NOT emit scrollback replayed: %q", sink.Body())
	}
}
