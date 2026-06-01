package log

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// errWriter is an io.Writer that always fails its Write, modelling a sink whose
// open/reopen or per-record write(2) failed (disk-full / EACCES / ENOSPC). The
// textHandler must swallow this and never propagate it to the slog caller.
type errWriter struct {
	err   error
	calls int
}

func (w *errWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, w.err
}

// captureStderrFallback redirects the package stderrFallback seam to buf for the
// duration of the test, restoring it via t.Cleanup. Tests assert the serialized
// record lands here when the primary sink write fails.
func captureStderrFallback(t *testing.T, buf io.Writer) {
	t.Helper()
	prev := stderrFallback
	stderrFallback = buf
	t.Cleanup(func() { stderrFallback = prev })
}

func TestTextHandler_DoesNotPropagateWriteFailureAndWritesStderrFallback(t *testing.T) {
	var fallback bytes.Buffer
	captureStderrFallback(t, &fallback)

	w := &errWriter{err: errors.New("disk full")}
	h := newTextHandler(w, slog.LevelInfo, 12345, "0.5.0", "daemon")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "daemon")})

	// Handle MUST return nil despite the underlying write failing.
	if err := h.Handle(context.Background(), newRecord(slog.LevelInfo, "open-failed")); err != nil {
		t.Fatalf("Handle propagated a write failure: %v (want nil)", err)
	}

	// The serialized record must have been attempted on the stderr fallback.
	out := fallback.String()
	if !strings.Contains(out, " daemon: open-failed ") {
		t.Errorf("expected serialized record on stderr fallback, got: %q", out)
	}
}

func TestTextHandler_DropsRecordOnWriteFailureAndContinues(t *testing.T) {
	var fallback bytes.Buffer
	captureStderrFallback(t, &fallback)

	w := &errWriter{err: errors.New("write failed")}
	h := newTextHandler(w, slog.LevelInfo, 1, "v", "daemon")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "daemon")})

	// First record: the write fails and is dropped — Handle returns nil.
	if err := h.Handle(context.Background(), newRecord(slog.LevelInfo, "dropped")); err != nil {
		t.Fatalf("Handle propagated a write failure: %v (want nil)", err)
	}

	// Now the writer recovers (modelling a successful reopen). The next record
	// must write normally.
	var recovered bytes.Buffer
	th, ok := h.(*textHandler)
	if !ok {
		t.Fatalf("handler is %T, want *textHandler", h)
	}
	th.w = &recovered

	if err := th.Handle(context.Background(), newRecord(slog.LevelInfo, "after")); err != nil {
		t.Fatalf("Handle returned error after recovery: %v", err)
	}
	if !strings.Contains(recovered.String(), " daemon: after ") {
		t.Errorf("post-recovery record did not write normally, got: %q", recovered.String())
	}
}

func TestTextHandler_NeverPanicsOrReturnsErrorOnDiskFullOrEACCES(t *testing.T) {
	var fallback bytes.Buffer
	captureStderrFallback(t, &fallback)

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"ENOSPC disk full", errors.New("write /x: no space left on device")},
		{"EACCES permission denied", os.ErrPermission},
		{"closed file", os.ErrClosed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &errWriter{err: tc.err}
			h := newTextHandler(w, slog.LevelInfo, 1, "v", "daemon")
			h = h.WithAttrs([]slog.Attr{slog.String("component", "daemon")})

			// No panic, no propagated error.
			if err := h.Handle(context.Background(), newRecord(slog.LevelInfo, "msg")); err != nil {
				t.Fatalf("Handle propagated %v error (want nil)", tc.err)
			}
		})
	}
}

func TestRotatingSink_OpenFailureDoesNotPropagateThroughHandle(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	var fallback bytes.Buffer
	captureStderrFallback(t, &fallback)

	// A 0500 (read+execute, no write) stateDir makes the first-of-day
	// O_CREAT|O_EXCL open fail with EACCES, exercising the open-failure arm
	// through the real sink wired into the configured handler.
	dir := t.TempDir()
	unwritable := filepath.Join(dir, "ro")
	if err := os.Mkdir(unwritable, 0o500); err != nil {
		t.Fatalf("mkdir 0500: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unwritable, 0o700) })

	s := newRotatingSink(unwritable, defaultRotateSize)
	t.Cleanup(func() { _ = s.close() })
	h := newTextHandler(s, slog.LevelInfo, 1, "v", "daemon")
	h = h.WithAttrs([]slog.Attr{slog.String("component", "daemon")})

	if err := h.Handle(context.Background(), newRecord(slog.LevelInfo, "open-eacces")); err != nil {
		t.Fatalf("Handle propagated an open failure: %v (want nil)", err)
	}

	// The record was attempted on the stderr fallback.
	if !strings.Contains(fallback.String(), " daemon: open-eacces ") {
		t.Errorf("expected serialized record on stderr fallback after open EACCES, got: %q", fallback.String())
	}
}

func TestInit_WriterIsUnbufferedMarkerReadableBeforeInfoReturns(t *testing.T) {
	snapshotInitState(t)

	day := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	dir := t.TempDir()
	if err := Init(dir, "0.5.0", "daemon"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// Info returns here; the bytes MUST already be in the kernel with NO explicit
	// flush/Sync — the unbuffered-writer constraint.
	For("daemon").Info("marker")

	// Read the day file with NO Sync call anywhere.
	b, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29"))
	if err != nil {
		t.Fatalf("reading day file: %v", err)
	}
	if !strings.Contains(string(b), " daemon: marker ") {
		t.Errorf("marker not readable from file immediately after Info returned (writer buffered?): %q", string(b))
	}
}

func TestRotatingSink_KeepsWritingToOpenFdWhenSymlinkSwingFails(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	// Force every symlink swing to fail via the symlinkFunc seam (Task 2-3).
	prev := symlinkFunc
	symlinkFunc = func(oldname, newname string) error {
		return errors.New("symlink swing failed")
	}
	t.Cleanup(func() { symlinkFunc = prev })

	dir := t.TempDir()
	s := newRotatingSink(dir, defaultRotateSize)
	t.Cleanup(func() { _ = s.close() })

	// Write must still succeed: the swing failure leaves the prior (absent)
	// symlink in place but the record lands in the freshly-opened day fd.
	if _, err := s.Write([]byte("swing-failed-line\n")); err != nil {
		t.Fatalf("Write returned error on a failed symlink swing: %v (want nil)", err)
	}

	// The record landed in the open day file.
	b, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29"))
	if err != nil {
		t.Fatalf("read day file: %v", err)
	}
	if string(b) != "swing-failed-line\n" {
		t.Errorf("day file = %q, want %q (write must continue to the open fd despite swing failure)", string(b), "swing-failed-line\n")
	}
}
