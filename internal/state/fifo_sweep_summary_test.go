// Tests in this file mutate package-level state (the process-wide log handler
// via log.SetTestHandler) and MUST NOT use t.Parallel.
//
// Phase 5 Task 5-6: the orphan-FIFO sweep cycle summary. SweepOrphanFIFOs emits
// exactly ONE INFO summary at completion under the clean-bound package logger
// (component "clean") carrying reaped=N + skipped=N + took. The per-removal INFO
// is demoted to a per-item DEBUG ("orphan fifo reaped", path attr) under clean,
// while the per-item lstat/remove WARNs stay on the injected bootstrap-bound
// logger seam with their wrapped-error attr.
//
// Spec reference: § Cycle-level summary cadence and shape (orphan-fifo-sweep row
// of the concrete cycle catalog); § Subsystem prefix taxonomy (clean component;
// closed cycle-summary attrs reaped/skipped/took).

package state_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// fifoSummarySink is a slog.Handler that records every emitted record with its
// level, message, and attrs (including the component attr bound via WithAttrs by
// log.For). The summary tests assert on the structured record (component=clean,
// msg, reaped/skipped int attrs, took rendered as a duration, path on the
// demoted DEBUG), so a substring sink would be too lossy.
type fifoSummarySink struct {
	mu      sync.Mutex
	records []fifoSummaryRecord
	shared  *fifoSummarySink
	bound   []slog.Attr
}

type fifoSummaryRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
}

func (s *fifoSummarySink) owner() *fifoSummarySink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *fifoSummarySink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *fifoSummarySink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &fifoSummarySink{shared: s.owner(), bound: next}
}

func (s *fifoSummarySink) WithGroup(_ string) slog.Handler {
	return &fifoSummarySink{shared: s.owner(), bound: s.bound}
}

func (s *fifoSummarySink) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())
	for _, a := range s.bound {
		attrs[a.Key] = a.Value
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	rec := fifoSummaryRecord{level: r.Level, msg: r.Message, attrs: attrs}
	owner := s.owner()
	owner.mu.Lock()
	owner.records = append(owner.records, rec)
	owner.mu.Unlock()
	return nil
}

func (s *fifoSummarySink) all() []fifoSummaryRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]fifoSummaryRecord, len(s.records))
	copy(out, s.records)
	return out
}

// summariesFor returns every record whose component matches comp and msg matches.
func (s *fifoSummarySink) summariesFor(comp, msg string) []fifoSummaryRecord {
	var out []fifoSummaryRecord
	for _, r := range s.all() {
		c, ok := r.attrs["component"]
		if !ok || c.String() != comp || r.msg != msg {
			continue
		}
		out = append(out, r)
	}
	return out
}

// onlySummary asserts exactly one record with the given component+msg was
// emitted and returns it.
func (s *fifoSummarySink) onlySummary(t *testing.T, comp, msg string) fifoSummaryRecord {
	t.Helper()
	sums := s.summariesFor(comp, msg)
	if len(sums) != 1 {
		t.Fatalf("expected exactly 1 %q %q summary, got %d: %+v", comp, msg, len(sums), s.all())
	}
	return sums[0]
}

// matching returns every record whose level+msg match and whose component
// equals comp.
func (s *fifoSummarySink) matching(level slog.Level, comp, msg string) []fifoSummaryRecord {
	var out []fifoSummaryRecord
	for _, r := range s.all() {
		if r.level != level || r.msg != msg {
			continue
		}
		c, ok := r.attrs["component"]
		if !ok || c.String() != comp {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (r fifoSummaryRecord) intAttr(t *testing.T, key string) int64 {
	t.Helper()
	v, ok := r.attrs[key]
	if !ok {
		t.Fatalf("summary missing attr %q: %+v", key, r.attrs)
	}
	if v.Kind() != slog.KindInt64 {
		t.Fatalf("attr %q kind = %v, want Int64: %+v", key, v.Kind(), v)
	}
	return v.Int64()
}

func (r fifoSummaryRecord) requireDuration(t *testing.T, key string) {
	t.Helper()
	v, ok := r.attrs[key]
	if !ok {
		t.Fatalf("summary missing attr %q: %+v", key, r.attrs)
	}
	if v.Kind() != slog.KindDuration {
		t.Errorf("attr %q kind = %v, want Duration", key, v.Kind())
	}
}

func installFIFOSummarySink(t *testing.T) *fifoSummarySink {
	t.Helper()
	sink := &fifoSummarySink{}
	log.SetTestHandler(t, sink)
	return sink
}

func TestSweepOrphanFIFOs_EmitsCleanSummaryCountingReapedAndSkipped(t *testing.T) {
	sink := installFIFOSummarySink(t)
	dir := t.TempDir()

	// Two orphans (reaped) + one live-marker-protected FIFO (skipped).
	reapedA := filepath.Join(dir, "hydrate-a__0.0.fifo")
	reapedB := filepath.Join(dir, "hydrate-b__0.0.fifo")
	protected := filepath.Join(dir, "hydrate-keep__0.0.fifo")
	for _, p := range []string{reapedA, reapedB, protected} {
		if err := state.CreateFIFO(p); err != nil {
			t.Fatalf("create FIFO %s: %v", p, err)
		}
	}

	live := map[string]struct{}{"keep__0.0": {}}

	if err := state.SweepOrphanFIFOs(dir, live, log.For("bootstrap")); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "orphan-fifo sweep complete")
	if rec.level != slog.LevelInfo {
		t.Errorf("summary level = %v, want INFO", rec.level)
	}
	if got := rec.intAttr(t, "reaped"); got != 2 {
		t.Errorf("reaped = %d, want 2", got)
	}
	if got := rec.intAttr(t, "skipped"); got != 1 {
		t.Errorf("skipped = %d, want 1 (live-marker-protected)", got)
	}
	rec.requireDuration(t, "took")
}

func TestSweepOrphanFIFOs_EmitsZeroReapedZeroSkippedForMissingStateDir(t *testing.T) {
	sink := installFIFOSummarySink(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	if err := state.SweepOrphanFIFOs(missing, map[string]struct{}{}, log.For("bootstrap")); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "orphan-fifo sweep complete")
	if got := rec.intAttr(t, "reaped"); got != 0 {
		t.Errorf("reaped = %d, want 0 (loop runs zero times)", got)
	}
	if got := rec.intAttr(t, "skipped"); got != 0 {
		t.Errorf("skipped = %d, want 0 (loop runs zero times)", got)
	}
	rec.requireDuration(t, "took")
}

func TestSweepOrphanFIFOs_PreservedNonFIFOCountsAsSkipped(t *testing.T) {
	sink := installFIFOSummarySink(t)
	dir := t.TempDir()

	regular := filepath.Join(dir, "hydrate-foo__0.0.fifo")
	if err := os.WriteFile(regular, []byte("not a fifo"), 0o600); err != nil {
		t.Fatalf("seed regular file: %v", err)
	}

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, log.For("bootstrap")); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	// The non-FIFO sibling is preserved.
	if info, err := os.Lstat(regular); err != nil {
		t.Fatalf("regular file removed by sweep: %v", err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("file mode changed: got %v", info.Mode())
	}

	rec := sink.onlySummary(t, "clean", "orphan-fifo sweep complete")
	if got := rec.intAttr(t, "reaped"); got != 0 {
		t.Errorf("reaped = %d, want 0", got)
	}
	if got := rec.intAttr(t, "skipped"); got != 1 {
		t.Errorf("skipped = %d, want 1 (non-FIFO sibling)", got)
	}
}

func TestSweepOrphanFIFOs_RemoveFailureWarnsOnLoggerAndCountsAsSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based EACCES setup is unix-specific")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0500 directory write protection")
	}

	sink := installFIFOSummarySink(t)
	dir := t.TempDir()

	a := filepath.Join(dir, "hydrate-a__0.0.fifo")
	b := filepath.Join(dir, "hydrate-b__0.0.fifo")
	if err := state.CreateFIFO(a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := state.CreateFIFO(b); err != nil {
		t.Fatalf("create b: %v", err)
	}

	// Strip write permission AFTER FIFOs are created so os.Remove fails for both.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, log.For("bootstrap")); err != nil {
		t.Errorf("SweepOrphanFIFOs returned error: %v", err)
	}

	// Restore permissions so the temp-dir cleanup can remove files.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}

	// Both remove failures WARN on the injected bootstrap logger, carrying a
	// wrapped error attr (the os.Remove error).
	warns := sink.matching(slog.LevelWarn, "bootstrap", "remove orphan fifo failed")
	if len(warns) != 2 {
		t.Fatalf("expected 2 remove-failure WARNs under bootstrap, got %d: %+v", len(warns), sink.all())
	}
	for _, w := range warns {
		if _, ok := w.attrs["error"]; !ok {
			t.Errorf("remove-failure WARN missing error attr: %+v", w.attrs)
		}
		if _, ok := w.attrs["path"]; !ok {
			t.Errorf("remove-failure WARN missing path attr: %+v", w.attrs)
		}
	}

	// Both failures counted as skipped; nothing reaped.
	rec := sink.onlySummary(t, "clean", "orphan-fifo sweep complete")
	if got := rec.intAttr(t, "reaped"); got != 0 {
		t.Errorf("reaped = %d, want 0", got)
	}
	if got := rec.intAttr(t, "skipped"); got != 2 {
		t.Errorf("skipped = %d, want 2 (both remove failures)", got)
	}
}

func TestSweepOrphanFIFOs_LiveMarkerProtectedCountsAsSkippedAndIsLeftInPlace(t *testing.T) {
	sink := installFIFOSummarySink(t)
	dir := t.TempDir()

	protected := filepath.Join(dir, "hydrate-keep__0.0.fifo")
	if err := state.CreateFIFO(protected); err != nil {
		t.Fatalf("create FIFO: %v", err)
	}

	live := map[string]struct{}{"keep__0.0": {}}

	if err := state.SweepOrphanFIFOs(dir, live, log.For("bootstrap")); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	if _, err := os.Lstat(protected); err != nil {
		t.Errorf("live-marker-protected FIFO removed: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "orphan-fifo sweep complete")
	if got := rec.intAttr(t, "reaped"); got != 0 {
		t.Errorf("reaped = %d, want 0", got)
	}
	if got := rec.intAttr(t, "skipped"); got != 1 {
		t.Errorf("skipped = %d, want 1 (live-marker-protected)", got)
	}
}

func TestSweepOrphanFIFOs_DemotesPerRemovalInfoToDebugUnderClean(t *testing.T) {
	sink := installFIFOSummarySink(t)
	dir := t.TempDir()

	orphan := filepath.Join(dir, "hydrate-gone__0.0.fifo")
	if err := state.CreateFIFO(orphan); err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, log.For("bootstrap")); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	// The old per-removal INFO message must be gone at any level/component.
	for _, r := range sink.all() {
		if r.msg == "removed orphan fifo" {
			t.Errorf("old per-removal INFO message must be gone: %+v", r)
		}
	}

	// Exactly one per-item DEBUG "orphan fifo reaped" under clean, carrying path.
	dbg := sink.matching(slog.LevelDebug, "clean", "orphan fifo reaped")
	if len(dbg) != 1 {
		t.Fatalf("expected 1 DEBUG 'orphan fifo reaped' under clean, got %d: %+v", len(dbg), sink.all())
	}
	if p, ok := dbg[0].attrs["path"]; !ok || p.String() != orphan {
		t.Errorf("DEBUG 'orphan fifo reaped' path = %v, want %s", dbg[0].attrs["path"], orphan)
	}
}

func TestSweepOrphanFIFOs_NoSummaryWhenGlobFails(t *testing.T) {
	sink := installFIFOSummarySink(t)
	// A malformed glob pattern is produced from a dir containing an unterminated
	// "[" character class, which filepath.Glob reports as ErrBadPattern.
	badDir := filepath.Join(t.TempDir(), "[")

	if err := state.SweepOrphanFIFOs(badDir, map[string]struct{}{}, log.For("bootstrap")); err == nil {
		t.Fatalf("expected non-nil error from filepath.Glob failure")
	}

	if got := sink.summariesFor("clean", "orphan-fifo sweep complete"); len(got) != 0 {
		t.Errorf("expected no summary on glob failure (returns before loop), got %d: %+v", len(got), got)
	}
}
