package log

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fixedClock returns a nowFunc that yields a controllable instant. The returned
// setter advances the clock so date-change behaviour is deterministic.
func fixedClock(t *testing.T, initial time.Time) (set func(time.Time)) {
	t.Helper()
	cur := initial
	var mu sync.Mutex
	prev := nowFunc
	nowFunc = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}
	t.Cleanup(func() { nowFunc = prev })
	return func(at time.Time) {
		mu.Lock()
		cur = at
		mu.Unlock()
	}
}

// statDevIno returns the (Dev, Ino) of path, following symlinks. Fails the test
// on a stat error so the assertion is unambiguous.
func statDevIno(t *testing.T, path string) (uint64, uint64) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat %s: Sys() is %T, want *syscall.Stat_t", path, info.Sys())
	}
	return uint64(st.Dev), st.Ino
}

func TestRotatingSink_FirstHandleCreatesDayFileViaExclusive(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	dir := t.TempDir()
	s := newRotatingSink(dir)

	if _, err := s.Write([]byte("line one\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	t.Cleanup(func() { _ = s.close() })

	dayPath := filepath.Join(dir, "portal.log.2026-05-29")
	b, err := os.ReadFile(dayPath)
	if err != nil {
		t.Fatalf("reading day file: %v", err)
	}
	if string(b) != "line one\n" {
		t.Errorf("day file contents = %q, want %q", string(b), "line one\n")
	}

	// The portal.log symlink must point at today's file.
	target, err := os.Readlink(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("readlink portal.log: %v", err)
	}
	if filepath.Base(target) != "portal.log.2026-05-29" {
		t.Errorf("symlink target = %q, want portal.log.2026-05-29", target)
	}
}

func TestRotatingSink_ReusesFdAcrossSameDayWritesWithMatchingInode(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	dir := t.TempDir()
	s := newRotatingSink(dir)
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("first\n")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	fd1 := s.file.Fd()

	if _, err := s.Write([]byte("second\n")); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	fd2 := s.file.Fd()

	if fd1 != fd2 {
		t.Errorf("fd changed across same-day writes: %d -> %d (expected reuse)", fd1, fd2)
	}

	b, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29"))
	if err != nil {
		t.Fatalf("read day file: %v", err)
	}
	if string(b) != "first\nsecond\n" {
		t.Errorf("day file = %q, want %q", string(b), "first\nsecond\n")
	}
}

func TestRotatingSink_ReopensOnSameDayInodeMismatchWithoutSweeps(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	dir := t.TempDir()
	s := newRotatingSink(dir)
	t.Cleanup(func() { _ = s.close() })

	sweeps := 0
	s.dayRoll = func() { sweeps++ }

	if _, err := s.Write([]byte("before\n")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	origIno := s.ino

	// Replace today's file out from under the open fd with a brand-new inode,
	// and swing the symlink to it (mimics a peer's size-cap rotation onto a new
	// same-day file). The open fd now points at an orphaned inode.
	dayPath := filepath.Join(dir, "portal.log.2026-05-29")
	if err := os.Remove(dayPath); err != nil {
		t.Fatalf("remove day file: %v", err)
	}
	nf, err := os.OpenFile(dayPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("recreate day file: %v", err)
	}
	_ = nf.Close()
	newIno := mustIno(t, dayPath)
	if newIno == origIno {
		t.Fatalf("test setup: recreated file reused inode %d", newIno)
	}

	if _, err := s.Write([]byte("after\n")); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	if sweeps != 0 {
		t.Errorf("day-roll sweeps ran %d times on a same-day inode mismatch; want 0", sweeps)
	}
	// The post-reopen fd's inode must match the live file.
	if s.ino != newIno {
		t.Errorf("sink inode = %d after reopen, want live inode %d", s.ino, newIno)
	}
	// "after" must have landed in the live (reopened) file, not the orphan.
	b, err := os.ReadFile(dayPath)
	if err != nil {
		t.Fatalf("read live day file: %v", err)
	}
	if string(b) != "after\n" {
		t.Errorf("live day file = %q, want %q (reopened onto live target)", string(b), "after\n")
	}
}

func TestRotatingSink_RecreatesDayFileWhenSymlinkTargetENOENT(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	dir := t.TempDir()
	s := newRotatingSink(dir)
	t.Cleanup(func() { _ = s.close() })

	sweeps := 0
	s.dayRoll = func() { sweeps++ }

	if _, err := s.Write([]byte("before\n")); err != nil {
		t.Fatalf("first Write: %v", err)
	}

	// Unlink today's file entirely: stat(symlink) now yields ENOENT.
	dayPath := filepath.Join(dir, "portal.log.2026-05-29")
	if err := os.Remove(dayPath); err != nil {
		t.Fatalf("remove day file: %v", err)
	}

	if _, err := s.Write([]byte("after\n")); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	if sweeps != 0 {
		t.Errorf("day-roll sweeps ran %d times on ENOENT mid-day; want 0", sweeps)
	}
	// Day file recreated and "after" landed in it.
	b, err := os.ReadFile(dayPath)
	if err != nil {
		t.Fatalf("read recreated day file: %v", err)
	}
	if string(b) != "after\n" {
		t.Errorf("recreated day file = %q, want %q", string(b), "after\n")
	}
}

func TestRotatingSink_OpensNewDayFileAndFlagsSweepsOnDateChange(t *testing.T) {
	day1 := time.Date(2026, 5, 29, 23, 59, 0, 0, time.UTC)
	set := fixedClock(t, day1)

	dir := t.TempDir()
	s := newRotatingSink(dir)
	t.Cleanup(func() { _ = s.close() })

	sweeps := 0
	s.dayRoll = func() { sweeps++ }

	if _, err := s.Write([]byte("day-one\n")); err != nil {
		t.Fatalf("day-one Write: %v", err)
	}
	if sweeps != 0 {
		t.Fatalf("sweeps ran %d times on first-ever write; want 0", sweeps)
	}

	// Advance past local midnight.
	set(time.Date(2026, 5, 30, 0, 0, 1, 0, time.UTC))

	if _, err := s.Write([]byte("day-two\n")); err != nil {
		t.Fatalf("day-two Write: %v", err)
	}

	if sweeps != 1 {
		t.Errorf("day-roll sweeps ran %d times on a date change; want 1", sweeps)
	}

	// New day's file created and written.
	b2, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-30"))
	if err != nil {
		t.Fatalf("read day-two file: %v", err)
	}
	if string(b2) != "day-two\n" {
		t.Errorf("day-two file = %q, want %q", string(b2), "day-two\n")
	}
	// Day-one's file is unchanged (not appended to after the roll).
	b1, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29"))
	if err != nil {
		t.Fatalf("read day-one file: %v", err)
	}
	if string(b1) != "day-one\n" {
		t.Errorf("day-one file = %q, want %q (untouched after roll)", string(b1), "day-one\n")
	}
	// Symlink follows today's file.
	target, err := os.Readlink(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if filepath.Base(target) != "portal.log.2026-05-30" {
		t.Errorf("symlink target = %q, want portal.log.2026-05-30", target)
	}
}

func TestRotatingSink_FallsBackToAppendOnEEXISTWhenLosingCreateRace(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	dir := t.TempDir()

	// A peer process already created today's file (won the create race) and
	// wrote a line. Our sink's first-of-day O_CREAT|O_EXCL must fail EEXIST and
	// fall back to O_APPEND, preserving the peer's line.
	dayPath := filepath.Join(dir, "portal.log.2026-05-29")
	if err := os.WriteFile(dayPath, []byte("peer-line\n"), 0o600); err != nil {
		t.Fatalf("seed peer file: %v", err)
	}

	s := newRotatingSink(dir)
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("our-line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	b, err := os.ReadFile(dayPath)
	if err != nil {
		t.Fatalf("read day file: %v", err)
	}
	if string(b) != "peer-line\nour-line\n" {
		t.Errorf("day file = %q, want %q (append fallback must preserve peer line)", string(b), "peer-line\nour-line\n")
	}
}

func TestRotatingSink_RaceFreeUnderConcurrentWrite(t *testing.T) {
	day := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	fixedClock(t, day)

	dir := t.TempDir()
	s := newRotatingSink(dir)
	t.Cleanup(func() { _ = s.close() })

	const goroutines = 8
	const perGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				line := "g" + strconv.Itoa(id) + "-" + strconv.Itoa(i) + "\n"
				if _, err := s.Write([]byte(line)); err != nil {
					t.Errorf("concurrent Write: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	// All lines accounted for (locked critical section serialises writes).
	b, err := os.ReadFile(filepath.Join(dir, "portal.log.2026-05-29"))
	if err != nil {
		t.Fatalf("read day file: %v", err)
	}
	lines := 0
	for _, c := range b {
		if c == '\n' {
			lines++
		}
	}
	if want := goroutines * perGoroutine; lines != want {
		t.Errorf("wrote %d lines, want %d", lines, want)
	}
}

// mustIno returns the inode of path, following symlinks.
func mustIno(t *testing.T, path string) uint64 {
	t.Helper()
	_, ino := statDevIno(t, path)
	return ino
}
