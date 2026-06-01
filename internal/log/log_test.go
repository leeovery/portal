package log

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"testing"
)

// initLogger is bound at package init — before any test body runs — to assert
// that internal/log's own package init constructs root before any For call.
// If For returned nil (or panicked) during init, this var would be nil or the
// package would fail to load.
var initLogger = For("init-probe")

func TestFor_ReturnsNonNilBeforeInit(t *testing.T) {
	logger := For("daemon")
	if logger == nil {
		t.Fatal("For returned a nil logger before Init")
	}
}

func TestFor_BoundAtPackageInit_IsNonNil(t *testing.T) {
	if initLogger == nil {
		t.Fatal("For called at package init returned nil; package init did not construct root first")
	}
}

func TestFor_EmptyComponentReturnsValidLogger(t *testing.T) {
	logger := For("")
	if logger == nil {
		t.Fatal("For(\"\") returned a nil logger")
	}
	// A valid logger must accept records without panicking.
	logger.Info("probe")
}

func TestFor_CachedLoggerRoutesToHandlerInstalledAfterSwap(t *testing.T) {
	restore := snapshotHandler()
	t.Cleanup(restore)

	// Cache a logger BEFORE the swap, mirroring package-init binding.
	cached := For("daemon")

	rec := &recordingHandler{}
	setHandler(rec)

	cached.Info("after swap")

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record routed to swapped-in handler, got %d", len(rec.records))
	}
	if got := rec.records[0].Message; got != "after swap" {
		t.Errorf("routed record message = %q, want %q", got, "after swap")
	}
}

func TestFor_RaceFreeUnderConcurrentForAndSwap(t *testing.T) {
	restore := snapshotHandler()
	t.Cleanup(restore)

	var wg sync.WaitGroup
	const goroutines = 16

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				logger := For("daemon")
				logger.Info("concurrent")
			}
		}()
	}
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				setHandler(&recordingHandler{})
			}
		}()
	}

	wg.Wait()
}

func TestDefaultHandler_DropsDebugEmitsInfoToStderr(t *testing.T) {
	// The default-handler behaviour (writing INFO+ as text to os.Stderr) is
	// observed in a subprocess so we can capture real stderr without mutating
	// process-global state in this test process.
	if os.Getenv("PORTAL_LOG_DEFAULT_HANDLER_PROBE") == "1" {
		logger := For("daemon")
		logger.Debug("debug-line")
		logger.Info("info-line")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run", "TestDefaultHandler_DropsDebugEmitsInfoToStderr")
	cmd.Env = append(os.Environ(), "PORTAL_LOG_DEFAULT_HANDLER_PROBE=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("subprocess failed: %v\nstderr:\n%s", err, stderr.String())
	}

	out := stderr.String()
	if bytes.Contains(stderr.Bytes(), []byte("debug-line")) {
		t.Errorf("DEBUG line should be dropped by the pre-Init default handler; stderr:\n%s", out)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("info-line")) {
		t.Errorf("INFO line should be emitted to stderr by the pre-Init default handler; stderr:\n%s", out)
	}
}

// snapshotHandler captures the current inner handler and returns a restore func
// so tests that call setHandler do not leak a handler into sibling tests.
func snapshotHandler() func() {
	prev := swap.load()
	return func() { setHandler(prev) }
}

// recordingHandler is an in-memory slog.Handler that records every Handle call.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordingHandler) WithGroup(string) slog.Handler { return h }
