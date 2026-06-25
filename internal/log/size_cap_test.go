package log

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// forceSegmentEEXIST overrides openSegmentFunc so that opening any segment whose
// N is in the given set fails with os.ErrExist, modelling a peer process / stale
// gap that already claimed that N. Restored via t.Cleanup. All other N values
// fall through to the real open.
func forceSegmentEEXIST(t *testing.T, taken map[int]bool) {
	t.Helper()
	prev := openSegmentFunc
	openSegmentFunc = func(path string, flag int, perm os.FileMode) (*os.File, error) {
		base := filepath.Base(path)
		// path is .../portal.log.<date>.<N>; pull the trailing .N.
		idx := lastDot(base)
		if idx >= 0 {
			if n, err := atoiSafe(base[idx+1:]); err == nil && taken[n] {
				return nil, os.ErrExist
			}
		}
		return os.OpenFile(path, flag, perm)
	}
	t.Cleanup(func() { openSegmentFunc = prev })
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func atoiSafe(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return 0, os.ErrInvalid
	}
	return n, nil
}

// sizeCapDay is the fixed calendar instant used across the size-cap tests so
// "today" is deterministic via the nowFunc clock seam.
func sizeCapDay(t *testing.T) {
	t.Helper()
	fixedClock(t, time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC))
}

// segmentTarget reads the portal.log symlink and returns its bare basename.
func segmentTarget(t *testing.T, dir string) string {
	t.Helper()
	target, err := os.Readlink(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("readlink portal.log: %v", err)
	}
	return filepath.Base(target)
}

func TestRotatingSink_RotatesToSegment1WhenNextRecordReachesCap(t *testing.T) {
	sizeCapDay(t)
	dir := t.TempDir()

	// Cap small enough that the second record would reach it. First record is 5
	// bytes ("aaaa\n"); cap of 6 means current_size(5) + len("bbbb\n")(5) = 10 >= 6.
	s := newRotatingSink(dir, 6)
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("aaaa\n")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	// First write goes to the base file; no rotation yet (current_size was 0).
	if got := segmentTarget(t, dir); got != "portal.log.2026-05-29" {
		t.Fatalf("after first write symlink = %q, want portal.log.2026-05-29", got)
	}

	if _, err := s.Write([]byte("bbbb\n")); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	// The overflow record must land in portal.log.2026-05-29.1 and the symlink
	// must point at it.
	if got := segmentTarget(t, dir); got != "portal.log.2026-05-29.1" {
		t.Errorf("after overflow symlink = %q, want portal.log.2026-05-29.1", got)
	}
	seg1 := filepath.Join(dir, "portal.log.2026-05-29.1")
	b, err := os.ReadFile(seg1)
	if err != nil {
		t.Fatalf("read segment .1: %v", err)
	}
	if string(b) != "bbbb\n" {
		t.Errorf("segment .1 = %q, want %q", string(b), "bbbb\n")
	}
	// The base file still holds only the first record (not sealed, untouched).
	base, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29"))
	if err != nil {
		t.Fatalf("read base file: %v", err)
	}
	if string(base) != "aaaa\n" {
		t.Errorf("base file = %q, want %q", string(base), "aaaa\n")
	}
}

func TestRotatingSink_DiscoversNextNAsMaxPlusOneAcrossGaps(t *testing.T) {
	sizeCapDay(t)
	dir := t.TempDir()

	// Pre-seed today's segments with a gap: .1 and .3 present, .2 missing. The
	// next overflow must open .4 (max+1), NOT fill the .2 gap.
	for _, n := range []string{"1", "3"} {
		if err := os.WriteFile(filepath.Join(dir, "portal.log.2026-05-29."+n), []byte("seed\n"), 0o600); err != nil {
			t.Fatalf("seed segment .%s: %v", n, err)
		}
	}

	s := newRotatingSink(dir, 1) // cap of 1 byte: any record overflows immediately.
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("overflow\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := segmentTarget(t, dir); got != "portal.log.2026-05-29.4" {
		t.Errorf("symlink = %q, want portal.log.2026-05-29.4 (max+1 across the gap)", got)
	}
	b, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29.4"))
	if err != nil {
		t.Fatalf("read segment .4: %v", err)
	}
	if string(b) != "overflow\n" {
		t.Errorf("segment .4 = %q, want %q", string(b), "overflow\n")
	}
	// The .2 gap was NOT filled.
	if _, err := os.Stat(filepath.Join(dir, "portal.log.2026-05-29.2")); !os.IsNotExist(err) {
		t.Errorf("segment .2 exists (stat err = %v); the gap must NOT be filled", err)
	}
}

func TestRotatingSink_OpensSegment1WhenNoExistingSegments(t *testing.T) {
	sizeCapDay(t)
	dir := t.TempDir()

	// Cap of 1 byte: the very first write overflows the (zero-size) base file with
	// no pre-existing .N segments present, so next N discovery must yield 1.
	s := newRotatingSink(dir, 1)
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("first\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := segmentTarget(t, dir); got != "portal.log.2026-05-29.1" {
		t.Errorf("symlink = %q, want portal.log.2026-05-29.1 (no existing .N -> 1)", got)
	}
	b, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29.1"))
	if err != nil {
		t.Fatalf("read segment .1: %v", err)
	}
	if string(b) != "first\n" {
		t.Errorf("segment .1 = %q, want %q", string(b), "first\n")
	}
}

func TestRotatingSink_RetriesNextNOnEEXISTUntilFreeSegmentClaimed(t *testing.T) {
	sizeCapDay(t)
	dir := t.TempDir()

	// Discovery yields next=1 (no existing segments). Force .1 and .2 to fail
	// EEXIST (a racing writer / stale gap claimed them) so the open must retry
	// 1 -> 2 -> 3 and land on .3.
	forceSegmentEEXIST(t, map[int]bool{1: true, 2: true})

	s := newRotatingSink(dir, 1) // cap of 1 byte: first write overflows.
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("retry\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := segmentTarget(t, dir); got != "portal.log.2026-05-29.3" {
		t.Errorf("symlink = %q, want portal.log.2026-05-29.3 (EEXIST retry 1->2->3)", got)
	}
	b, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29.3"))
	if err != nil {
		t.Fatalf("read segment .3: %v", err)
	}
	if string(b) != "retry\n" {
		t.Errorf("segment .3 = %q, want %q", string(b), "retry\n")
	}
}

func TestRotatingSink_DoesNotChmodPriorSegmentAfterSizeCapRotation(t *testing.T) {
	sizeCapDay(t)
	dir := t.TempDir()

	// Cap of 6: first record (5 bytes) goes to the base file, second overflows.
	s := newRotatingSink(dir, 6)
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("aaaa\n")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if _, err := s.Write([]byte("bbbb\n")); err != nil {
		t.Fatalf("second Write (overflow): %v", err)
	}

	// The prior same-day segment (the base file) must remain mode 0600 — it is NOT
	// sealed on a same-day rotation (a peer may hold an open O_APPEND fd; same-day
	// files are sealed only on the day roll).
	basePath := filepath.Join(dir, "portal.log.2026-05-29")
	info, err := os.Stat(basePath)
	if err != nil {
		t.Fatalf("stat base file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("prior segment perm = %o after size-cap rotation, want 0600 (must NOT be chmod'd)", got)
	}
}

func TestRotatingSink_NeverRotatesInSteadyStateBelowCap(t *testing.T) {
	sizeCapDay(t)
	dir := t.TempDir()

	// A realistic cap relative to the record sizes: many small writes stay far
	// below it, so no overflow segment is ever created.
	s := newRotatingSink(dir, defaultRotateSize)
	t.Cleanup(func() { _ = s.close() })

	for i := range 100 {
		if _, err := s.Write([]byte("steady-state line\n")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	// Exactly one log file (the base day file); no .N overflow segments.
	matches, err := filepath.Glob(filepath.Join(dir, "portal.log.2026-05-29*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d portal.log.2026-05-29* files, want 1 (no overflow): %v", len(matches), matches)
	}
	if filepath.Base(matches[0]) != "portal.log.2026-05-29" {
		t.Errorf("sole file = %q, want portal.log.2026-05-29", filepath.Base(matches[0]))
	}
	// Symlink still points at the base file (never swung to a .N).
	if got := segmentTarget(t, dir); got != "portal.log.2026-05-29" {
		t.Errorf("symlink = %q, want portal.log.2026-05-29 (no rotation in steady state)", got)
	}
}
