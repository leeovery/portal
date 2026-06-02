package state_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/statetest"
)

func TestSignalHydrateRetryDelays_MatchesSpecLadder(t *testing.T) {
	want := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
		80 * time.Millisecond,
		160 * time.Millisecond,
		190 * time.Millisecond,
	}
	if !reflect.DeepEqual([]time.Duration(state.SignalHydrateRetryDelays), want) {
		t.Errorf("SignalHydrateRetryDelays = %v, want %v", state.SignalHydrateRetryDelays, want)
	}
}

func TestSignalHydrateRetryDelays_CumulativeBudget500ms(t *testing.T) {
	var total time.Duration
	for _, d := range state.SignalHydrateRetryDelays {
		total += d
	}
	const want = 500 * time.Millisecond
	if total != want {
		t.Errorf("cumulative SignalHydrateRetryDelays = %v, want %v", total, want)
	}
}

func TestWriteFIFOSignal_WritesOneByteOnFirstTrySuccess(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		return w, nil
	}
	sleep := &statetest.RecordingSleep{}

	if err := state.WriteFIFOSignal("/tmp/example.fifo", open, sleep.Fn()); err != nil {
		t.Fatalf("WriteFIFOSignal: %v", err)
	}
	if openCalls != 1 {
		t.Errorf("OpenFIFO calls = %d, want 1", openCalls)
	}
	if len(sleep.Durations) != 0 {
		t.Errorf("Sleep called %v times on first-try success, want 0", len(sleep.Durations))
	}

	// Independent side-effect check: the byte must be observable on the read end.
	_ = w.Close()
	buf := make([]byte, 8)
	n, _ := r.Read(buf)
	if n != 1 {
		t.Errorf("read %d bytes, want 1", n)
	}
}

func TestWriteFIFOSignal_RetriesOnENXIOPerLadder(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		switch openCalls {
		case 1, 2:
			return nil, syscall.ENXIO
		default:
			return w, nil
		}
	}
	sleep := &statetest.RecordingSleep{}

	if err := state.WriteFIFOSignal("/tmp/example.fifo", open, sleep.Fn()); err != nil {
		t.Fatalf("WriteFIFOSignal: %v", err)
	}
	if openCalls != 3 {
		t.Errorf("OpenFIFO calls = %d, want 3", openCalls)
	}
	want := []time.Duration{
		state.SignalHydrateRetryDelays[0], // 10ms before retry 1
		state.SignalHydrateRetryDelays[1], // 20ms before retry 2
	}
	if !reflect.DeepEqual(sleep.Durations, want) {
		t.Errorf("Sleep durations = %v, want %v", sleep.Durations, want)
	}
}

// TestWriteFIFOSignal_EmitsRetryDebugUnderSignal pins the lower-level
// transition breadcrumb (Phase 5 Task 5-11, option a): on each retryable-error
// transition (ENXIO/EAGAIN) the retry ladder emits a DEBUG "fifo signal
// retrying" under component=signal carrying path + the wrapped error. The
// whole-operation WARN stays at the EagerSignalHydrate caller; this is the
// per-retry detail under signal. A retryable-then-success ladder fires the
// DEBUG once (one retry) and the operation still returns nil.
func TestWriteFIFOSignal_EmitsRetryDebugUnderSignal(t *testing.T) {
	sink := installFIFOSummarySink(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		if openCalls == 1 {
			return nil, syscall.ENXIO
		}
		return w, nil
	}
	sleep := &statetest.RecordingSleep{}

	const path = "/tmp/example.fifo"
	if err := state.WriteFIFOSignal(path, open, sleep.Fn()); err != nil {
		t.Fatalf("WriteFIFOSignal: %v", err)
	}

	dbg := sink.matching(slog.LevelDebug, "signal", "fifo signal retrying")
	if len(dbg) != 1 {
		t.Fatalf("expected 1 DEBUG 'fifo signal retrying' under component=signal (one retryable transition), got %d: %+v", len(dbg), sink.Records())
	}
	if p, ok := dbg[0].Attrs["path"]; !ok || p.String() != path {
		t.Errorf("retry DEBUG path attr = %v; want %q", dbg[0].Attrs["path"], path)
	}
	errAttr, ok := dbg[0].Attrs["error"]
	if !ok {
		t.Fatalf("retry DEBUG missing error attr: %+v", dbg[0].Attrs)
	}
	if errAttr.Kind() != slog.KindAny {
		t.Errorf("retry DEBUG error attr kind = %v; want Any (wrapped err passed directly)", errAttr.Kind())
	}
	if gotErr, ok := errAttr.Any().(error); !ok || !errors.Is(gotErr, syscall.ENXIO) {
		t.Errorf("retry DEBUG error attr = %v; want errors.Is(err, ENXIO)=true", errAttr.Any())
	}
}

// TestWriteFIFOSignal_RetryDebugOncePerRetryTransition pins the breadcrumb
// cardinality: a retry-exhaustion ladder (always ENXIO) fires the DEBUG once
// per actual sleep+retry transition — len(SignalHydrateRetryDelays) times,
// not once per open attempt.
func TestWriteFIFOSignal_RetryDebugOncePerRetryTransition(t *testing.T) {
	sink := installFIFOSummarySink(t)

	open := func(_ string) (*os.File, error) { return nil, syscall.ENXIO }
	sleep := &statetest.RecordingSleep{}

	const path = "/tmp/never-ready.fifo"
	if err := state.WriteFIFOSignal(path, open, sleep.Fn()); err == nil {
		t.Fatalf("expected retry-exhaustion error, got nil")
	}

	dbg := sink.matching(slog.LevelDebug, "signal", "fifo signal retrying")
	if len(dbg) != len(state.SignalHydrateRetryDelays) {
		t.Errorf("retry DEBUG count = %d; want %d (one per sleep+retry transition)", len(dbg), len(state.SignalHydrateRetryDelays))
	}
}

func TestWriteFIFOSignal_RetriesOnEAGAINPerLadder(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		if openCalls == 1 {
			return nil, syscall.EAGAIN
		}
		return w, nil
	}
	sleep := &statetest.RecordingSleep{}

	if err := state.WriteFIFOSignal("/tmp/example.fifo", open, sleep.Fn()); err != nil {
		t.Fatalf("WriteFIFOSignal: %v", err)
	}
	if openCalls != 2 {
		t.Errorf("OpenFIFO calls = %d, want 2", openCalls)
	}
	want := []time.Duration{state.SignalHydrateRetryDelays[0]}
	if !reflect.DeepEqual(sleep.Durations, want) {
		t.Errorf("Sleep durations = %v, want %v", sleep.Durations, want)
	}
}

func TestWriteFIFOSignal_ENOENTReturnsImmediatelyWithOpenFifoWrap(t *testing.T) {
	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		return nil, syscall.ENOENT
	}
	sleep := &statetest.RecordingSleep{}

	const path = "/tmp/missing.fifo"
	err := state.WriteFIFOSignal(path, open, sleep.Fn())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if openCalls != 1 {
		t.Errorf("OpenFIFO calls = %d, want 1 (no retry on ENOENT)", openCalls)
	}
	if len(sleep.Durations) != 0 {
		t.Errorf("Sleep called %d times on ENOENT, want 0", len(sleep.Durations))
	}
	if !errors.Is(err, syscall.ENOENT) {
		t.Errorf("err does not wrap syscall.ENOENT: %v", err)
	}
	wantPrefix := fmt.Sprintf("open fifo %s:", path)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("err %q does not start with %q", err.Error(), wantPrefix)
	}
}

func TestWriteFIFOSignal_NonRetryableErrorReturnsImmediately(t *testing.T) {
	// Any non-ENXIO/non-EAGAIN error must surface on the first iteration with
	// no Sleep call, wrapped with the "open fifo %s" prefix.
	sentinel := errors.New("permission denied (sentinel)")
	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		return nil, sentinel
	}
	sleep := &statetest.RecordingSleep{}

	const path = "/tmp/forbidden.fifo"
	err := state.WriteFIFOSignal(path, open, sleep.Fn())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if openCalls != 1 {
		t.Errorf("OpenFIFO calls = %d, want 1 (no retry on non-retryable err)", openCalls)
	}
	if len(sleep.Durations) != 0 {
		t.Errorf("Sleep called %d times on non-retryable err, want 0", len(sleep.Durations))
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("err does not wrap sentinel: %v", err)
	}
	wantPrefix := fmt.Sprintf("open fifo %s:", path)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("err %q does not start with %q", err.Error(), wantPrefix)
	}
}

func TestWriteFIFOSignal_RetryExhaustionWrapsLastErrWithRetriesExhausted(t *testing.T) {
	openCalls := 0
	open := func(_ string) (*os.File, error) {
		openCalls++
		return nil, syscall.ENXIO
	}
	sleep := &statetest.RecordingSleep{}

	const path = "/tmp/never-ready.fifo"
	err := state.WriteFIFOSignal(path, open, sleep.Fn())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	// 7 attempts: initial + len(delays) retries.
	wantOpens := 1 + len(state.SignalHydrateRetryDelays)
	if openCalls != wantOpens {
		t.Errorf("OpenFIFO calls = %d, want %d (initial + 6 retries)", openCalls, wantOpens)
	}
	// One sleep before each retry.
	if len(sleep.Durations) != len(state.SignalHydrateRetryDelays) {
		t.Errorf("Sleep called %d times, want %d", len(sleep.Durations), len(state.SignalHydrateRetryDelays))
	}
	if !reflect.DeepEqual(sleep.Durations, []time.Duration(state.SignalHydrateRetryDelays)) {
		t.Errorf("Sleep durations = %v, want %v", sleep.Durations, state.SignalHydrateRetryDelays)
	}

	if !errors.Is(err, syscall.ENXIO) {
		t.Errorf("retries-exhausted err does not wrap ENXIO: %v", err)
	}
	wantPrefix := fmt.Sprintf("retries exhausted opening fifo %s:", path)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("err %q does not start with %q", err.Error(), wantPrefix)
	}
}

func TestOpenFIFOForSignal_NonBlockingFlags(t *testing.T) {
	// Validate the production seam by inspecting its observable behavior:
	// open a real FIFO with no reader and verify OpenFIFOForSignal returns
	// ENXIO immediately rather than blocking. Only O_WRONLY|O_NONBLOCK
	// produces this result on POSIX.
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "no-reader.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	start := time.Now()
	f, err := state.OpenFIFOForSignal(path)
	elapsed := time.Since(start)

	if f != nil {
		_ = f.Close()
		t.Fatal("OpenFIFOForSignal returned non-nil file with no reader; expected ENXIO")
	}
	if !errors.Is(err, syscall.ENXIO) {
		t.Fatalf("OpenFIFOForSignal err = %v, want syscall.ENXIO", err)
	}
	// O_NONBLOCK guarantees the call returns immediately rather than blocking
	// for a reader. 100ms is a generous upper bound.
	if elapsed >= 100*time.Millisecond {
		t.Errorf("OpenFIFOForSignal blocked for %v; expected ~immediate return (O_NONBLOCK missing?)", elapsed)
	}
}

// TestSendHydrateSignal_WritesOneByteToReadyFIFO pins the production no-seam
// entry point: SendHydrateSignal opens the supplied FIFO via the production
// OpenFIFOForSignal seam, writes one byte, returns nil. This guards the
// caller-facing contract that production sites (cmd/state_signal_hydrate and
// cmd/bootstrap_production) rely on — neither passes a custom seam.
func TestSendHydrateSignal_WritesOneByteToReadyFIFO(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ready.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	// Stand up a blocking O_RDONLY reader in a goroutine so the FIFO has a
	// reader present by the time SendHydrateSignal calls its O_WRONLY|O_NONBLOCK
	// open. The reader's blocking Read returns once SendHydrateSignal writes
	// the byte; the goroutine forwards (n, err) on a buffered channel that
	// the main goroutine drains with a 1s timeout. FIFOs do not support
	// SetReadDeadline (file type does not support deadline) so a goroutine +
	// channel timeout is the portable shape.
	type readResult struct {
		n   int
		err error
	}
	readDone := make(chan readResult, 1)
	go func() {
		reader, openErr := os.OpenFile(path, os.O_RDONLY, 0)
		if openErr != nil {
			readDone <- readResult{err: openErr}
			return
		}
		defer func() { _ = reader.Close() }()
		buf := make([]byte, 8)
		n, err := reader.Read(buf)
		readDone <- readResult{n: n, err: err}
	}()

	if err := state.SendHydrateSignal(path); err != nil {
		t.Fatalf("SendHydrateSignal: %v", err)
	}

	select {
	case r := <-readDone:
		if r.err != nil {
			t.Fatalf("reader goroutine: %v", r.err)
		}
		if r.n != 1 {
			t.Errorf("read %d bytes, want 1", r.n)
		}
	case <-time.After(time.Second):
		t.Fatal("reader goroutine did not receive byte within 1s")
	}
}

// TestSendHydrateSignal_PropagatesNonRetryableError pins the error contract
// against a missing FIFO: ENOENT must surface immediately wrapped with the
// production "open fifo" prefix because SendHydrateSignal delegates to
// WriteFIFOSignal, which surfaces non-ENXIO/non-EAGAIN errors on the first
// iteration. A regression that altered the wrapping (or accidentally swapped
// the production seams) would surface here.
func TestSendHydrateSignal_PropagatesNonRetryableError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.fifo")
	// NOTE: deliberately do NOT mkfifo — the open MUST surface ENOENT.

	err := state.SendHydrateSignal(path)
	if err == nil {
		t.Fatal("SendHydrateSignal returned nil; want ENOENT-wrapped error")
	}
	if !errors.Is(err, syscall.ENOENT) {
		t.Errorf("SendHydrateSignal err = %v; want errors.Is(err, syscall.ENOENT)=true", err)
	}
	wantPrefix := fmt.Sprintf("open fifo %s:", path)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("err %q does not start with %q", err.Error(), wantPrefix)
	}
}

// TestDefaultFIFOSignaler_SendSignalDelegatesToSendHydrateSignal pins the
// adapter contract: DefaultFIFOSignaler{}.SendSignal(path) returns whatever
// state.SendHydrateSignal(path) returns. Since DefaultFIFOSignaler is the
// production wiring at cmd/bootstrap_production.go, a regression that broke
// this delegation would silently drop the FIFO byte for every restored pane.
func TestDefaultFIFOSignaler_SendSignalDelegatesToSendHydrateSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.fifo")
	// No mkfifo — both SendSignal and SendHydrateSignal must surface ENOENT
	// wrapped with the same "open fifo" prefix.

	directErr := state.SendHydrateSignal(path)
	adapterErr := state.DefaultFIFOSignaler{}.SendSignal(path)

	if directErr == nil || adapterErr == nil {
		t.Fatalf("expected non-nil errors; directErr=%v adapterErr=%v", directErr, adapterErr)
	}
	if !errors.Is(adapterErr, syscall.ENOENT) {
		t.Errorf("adapterErr = %v; want errors.Is(adapterErr, syscall.ENOENT)=true", adapterErr)
	}
	if directErr.Error() != adapterErr.Error() {
		t.Errorf("DefaultFIFOSignaler.SendSignal err %q diverges from state.SendHydrateSignal err %q",
			adapterErr.Error(), directErr.Error())
	}
}
