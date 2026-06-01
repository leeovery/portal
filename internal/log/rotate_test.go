package log

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// componentCapture is a test slog.Handler that records every Handle call AND
// preserves the WithAttrs-accumulated attrs (notably the component attr that For
// delivers via root.With("component", ...)). The package-level recordingHandler
// discards WithAttrs, so it cannot see the component prefix; this handler keeps
// the accumulated chain so a test can assert component=log-rotate on a captured
// record.
type componentCapture struct {
	mu       *sync.Mutex // shared across derived handlers so vet sees no copy.
	sticky   []slog.Attr // accumulated via WithAttrs (carries the component attr).
	captured *[]capturedRecord
}

// capturedRecord is a flattened view of one Handle call: its message plus a
// merged key->string map of the sticky (WithAttrs) and per-record attrs.
type capturedRecord struct {
	message string
	attrs   map[string]string
}

func newComponentCapture() (*componentCapture, *[]capturedRecord) {
	recs := &[]capturedRecord{}
	return &componentCapture{mu: &sync.Mutex{}, captured: recs}, recs
}

func (h *componentCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *componentCapture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	attrs := make(map[string]string, len(h.sticky)+r.NumAttrs())
	for _, a := range h.sticky {
		attrs[a.Key] = a.Value.Resolve().String()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Resolve().String()
		return true
	})
	*h.captured = append(*h.captured, capturedRecord{message: r.Message, attrs: attrs})
	return nil
}

func (h *componentCapture) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.sticky = append(append([]slog.Attr(nil), h.sticky...), attrs...)
	return &clone
}

func (h *componentCapture) WithGroup(string) slog.Handler { return h }

// touchFile creates an empty file at ${dir}/${name} with mode 0600 and fails the
// test on error.
func touchFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// permOf returns the permission bits of path, failing the test on a stat error.
func permOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func TestRotatingSink_SealsPastDayFilesOnRealDayRoll(t *testing.T) {
	day1 := mustDate(2026, 5, 29)
	set := fixedClock(t, day1)

	dir := t.TempDir()
	s := newRotatingSink(dir, defaultRotateSize)
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("day-one\n")); err != nil {
		t.Fatalf("day-one Write: %v", err)
	}

	// Seed a size-cap overflow segment for day one so the roll seals it too.
	seg := touchFile(t, dir, "portal.log.2026-05-29.1")

	day1Path := filepath.Join(dir, "portal.log.2026-05-29")
	if got := permOf(t, day1Path); got == 0o400 {
		t.Fatalf("day-one base sealed before the roll; want still writable")
	}

	// Roll past local midnight and write again — the day-roll seam must seal all
	// of yesterday's files.
	set(mustDate(2026, 5, 30))
	if _, err := s.Write([]byte("day-two\n")); err != nil {
		t.Fatalf("day-two Write: %v", err)
	}

	if got := permOf(t, day1Path); got != 0o400 {
		t.Errorf("day-one base perm = %o after roll, want 0400 (sink must wire sealPastDayFiles into dayRoll)", got)
	}
	if got := permOf(t, seg); got != 0o400 {
		t.Errorf("day-one segment perm = %o after roll, want 0400", got)
	}
	// Today's file is NOT sealed.
	if got := permOf(t, filepath.Join(dir, "portal.log.2026-05-30")); got == 0o400 {
		t.Errorf("today's file was sealed by the roll; want still writable")
	}
}

// mustDate is a terse local-time date constructor for clock-injection tests.
func mustDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
}

func TestSealPastDayFiles_SealsAllYesterdaySegmentsOnDayRoll(t *testing.T) {
	dir := t.TempDir()

	base := touchFile(t, dir, "portal.log.2026-05-29")
	seg1 := touchFile(t, dir, "portal.log.2026-05-29.1")
	seg2 := touchFile(t, dir, "portal.log.2026-05-29.2")

	sealPastDayFiles(dir, "2026-05-30")

	for _, p := range []string{base, seg1, seg2} {
		if got := permOf(t, p); got != 0o400 {
			t.Errorf("%s perm = %o, want 0400", filepath.Base(p), got)
		}
	}
}

func TestSealPastDayFiles_SealsEveryPastDayInOneSweep(t *testing.T) {
	dir := t.TempDir()

	d1 := touchFile(t, dir, "portal.log.2026-05-25")
	d2 := touchFile(t, dir, "portal.log.2026-05-26.1")
	d3 := touchFile(t, dir, "portal.log.2026-05-28")

	sealPastDayFiles(dir, "2026-05-30")

	for _, p := range []string{d1, d2, d3} {
		if got := permOf(t, p); got != 0o400 {
			t.Errorf("%s perm = %o, want 0400 (multi-day downtime must seal all past days)", filepath.Base(p), got)
		}
	}
}

func TestSealPastDayFiles_SkipsSymlinkTempSweptSentinelAndNonLogSiblings(t *testing.T) {
	dir := t.TempDir()

	tmp := touchFile(t, dir, "portal.log."+strconv.Itoa(os.Getpid())+".symlink.tmp")
	swept := touchFile(t, dir, "portal.log.swept.2026-05-29")
	other := touchFile(t, dir, "portal.log.notes")

	sealPastDayFiles(dir, "2026-05-30")

	for _, p := range []string{tmp, swept, other} {
		if got := permOf(t, p); got == 0o400 {
			t.Errorf("%s was chmod'd to 0400; strict date-parse must skip non-log siblings", filepath.Base(p))
		}
	}
}

func TestSealPastDayFiles_SkipsFileAlreadyAt0400(t *testing.T) {
	dir := t.TempDir()

	path := touchFile(t, dir, "portal.log.2026-05-29")
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("pre-seal chmod: %v", err)
	}

	// Force chmod to fail if it is invoked at all; an already-0400 file must be
	// skipped before reaching chmodFunc.
	prev := chmodFunc
	chmodFunc = func(string, os.FileMode) error {
		return errors.New("chmod must not be called for an already-sealed file")
	}
	t.Cleanup(func() { chmodFunc = prev })

	rec, captured := newComponentCapture()
	SetTestHandler(t, rec)

	sealPastDayFiles(dir, "2026-05-30")

	if len(*captured) != 0 {
		t.Errorf("an already-0400 file produced %d log records; want 0 (no redundant chmod, no WARN)", len(*captured))
	}
	if got := permOf(t, path); got != 0o400 {
		t.Errorf("perm = %o, want 0400 (unchanged)", got)
	}
}

func TestSealPastDayFiles_DoesNotSealTodayFileOrTodaySameDaySegments(t *testing.T) {
	dir := t.TempDir()

	today := touchFile(t, dir, "portal.log.2026-05-30")
	seg := touchFile(t, dir, "portal.log.2026-05-30.1")

	sealPastDayFiles(dir, "2026-05-30")

	for _, p := range []string{today, seg} {
		if got := permOf(t, p); got == 0o400 {
			t.Errorf("%s was sealed; today's file and same-day segments must NOT be chmod'd (peer may hold an open fd)", filepath.Base(p))
		}
	}
}

func TestSealPastDayFiles_WarnsAndContinuesWhenChmodFails(t *testing.T) {
	dir := t.TempDir()

	// Two past-day candidates. Force chmod to fail for exactly one of them and
	// assert the OTHER is still sealed (continue-not-abort).
	failPath := touchFile(t, dir, "portal.log.2026-05-28")
	okPath := touchFile(t, dir, "portal.log.2026-05-29")

	prev := chmodFunc
	chmodFunc = func(path string, mode os.FileMode) error {
		if path == failPath {
			return errors.New("synthetic chmod failure")
		}
		return os.Chmod(path, mode)
	}
	t.Cleanup(func() { chmodFunc = prev })

	rec, captured := newComponentCapture()
	SetTestHandler(t, rec)

	sealPastDayFiles(dir, "2026-05-30")

	// Exactly one WARN under log-rotate naming the failed path.
	var warns []capturedRecord
	for _, r := range *captured {
		if r.message == "chmod failed" {
			warns = append(warns, r)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("got %d 'chmod failed' records, want 1", len(warns))
	}
	w := warns[0]
	if got := w.attrs["component"]; got != "log-rotate" {
		t.Errorf("WARN component = %q, want log-rotate", got)
	}
	if got := w.attrs["path"]; got != failPath {
		t.Errorf("WARN path = %q, want %q", got, failPath)
	}
	if _, ok := w.attrs["error"]; !ok {
		t.Errorf("WARN missing error attr")
	}

	// The sweep continued: the other candidate is sealed despite the earlier failure.
	if got := permOf(t, okPath); got != 0o400 {
		t.Errorf("%s perm = %o, want 0400 (sweep must continue past a chmod failure)", filepath.Base(okPath), got)
	}
}
