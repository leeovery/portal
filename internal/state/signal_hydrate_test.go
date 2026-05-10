package state_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// fakeSleep records every duration WriteFIFOSignal hands to its sleep seam so
// tests can assert the retry-ladder shape without timing-dependent waits.
type fakeSleep struct {
	Durations []time.Duration
}

func (s *fakeSleep) fn() func(time.Duration) {
	return func(d time.Duration) { s.Durations = append(s.Durations, d) }
}

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
	sleep := &fakeSleep{}

	if err := state.WriteFIFOSignal("/tmp/example.fifo", open, sleep.fn()); err != nil {
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
	sleep := &fakeSleep{}

	if err := state.WriteFIFOSignal("/tmp/example.fifo", open, sleep.fn()); err != nil {
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
	sleep := &fakeSleep{}

	if err := state.WriteFIFOSignal("/tmp/example.fifo", open, sleep.fn()); err != nil {
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
	sleep := &fakeSleep{}

	const path = "/tmp/missing.fifo"
	err := state.WriteFIFOSignal(path, open, sleep.fn())
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
	sleep := &fakeSleep{}

	const path = "/tmp/forbidden.fifo"
	err := state.WriteFIFOSignal(path, open, sleep.fn())
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
	sleep := &fakeSleep{}

	const path = "/tmp/never-ready.fifo"
	err := state.WriteFIFOSignal(path, open, sleep.fn())
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
