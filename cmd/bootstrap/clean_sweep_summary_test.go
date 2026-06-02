// Tests in this file mutate package-level state (the process-wide log handler
// via log.SetTestHandler) and MUST NOT use t.Parallel.
//
// Phase 5 Task 5-5: the two cmd/bootstrap clean-sweep cycle summaries. Both
// SweepOrphanDaemons and CleanStaleMarkers emit exactly ONE INFO summary at
// completion under the clean-bound package logger (component "clean"), while
// per-item Debug/Warn breadcrumbs stay on the injected bootstrap-bound logger
// seam. The orphan-daemon summary carries killed=N + took; the marker summary
// carries unset=N + took. The per-kill INFO is demoted to a per-item DEBUG
// (under clean, grouping the sweep's own detail with its summary).
//
// Spec reference: § Cycle-level summary cadence and shape (orphan-daemon-sweep
// and marker-cleanup rows of the concrete cycle catalog); § Subsystem prefix
// taxonomy (clean component; closed cycle-summary attrs killed/unset/took).

package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// cleanSummarySink is a slog.Handler that records every emitted record with its
// level, message, and attrs (including the component attr bound via WithAttrs by
// log.For). The clean-sweep summary tests assert on the structured record
// (component=clean, msg, killed/unset int attrs, took rendered as a duration),
// so a substring sink would be too lossy.
type cleanSummarySink struct {
	mu      sync.Mutex
	records []cleanSummaryRecord
	shared  *cleanSummarySink
	bound   []slog.Attr
}

type cleanSummaryRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
}

func (s *cleanSummarySink) owner() *cleanSummarySink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *cleanSummarySink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *cleanSummarySink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &cleanSummarySink{shared: s.owner(), bound: next}
}

func (s *cleanSummarySink) WithGroup(_ string) slog.Handler {
	return &cleanSummarySink{shared: s.owner(), bound: s.bound}
}

func (s *cleanSummarySink) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())
	for _, a := range s.bound {
		attrs[a.Key] = a.Value
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	rec := cleanSummaryRecord{level: r.Level, msg: r.Message, attrs: attrs}
	owner := s.owner()
	owner.mu.Lock()
	owner.records = append(owner.records, rec)
	owner.mu.Unlock()
	return nil
}

func (s *cleanSummarySink) all() []cleanSummaryRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]cleanSummaryRecord, len(s.records))
	copy(out, s.records)
	return out
}

// summariesFor returns every record whose component matches comp and msg matches.
func (s *cleanSummarySink) summariesFor(comp, msg string) []cleanSummaryRecord {
	var out []cleanSummaryRecord
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
func (s *cleanSummarySink) onlySummary(t *testing.T, comp, msg string) cleanSummaryRecord {
	t.Helper()
	sums := s.summariesFor(comp, msg)
	if len(sums) != 1 {
		t.Fatalf("expected exactly 1 %q %q summary, got %d: %+v", comp, msg, len(sums), s.all())
	}
	return sums[0]
}

func (r cleanSummaryRecord) intAttr(t *testing.T, key string) int64 {
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

func (r cleanSummaryRecord) requireDuration(t *testing.T, key string) {
	t.Helper()
	v, ok := r.attrs[key]
	if !ok {
		t.Fatalf("summary missing attr %q: %+v", key, r.attrs)
	}
	if v.Kind() != slog.KindDuration {
		t.Errorf("attr %q kind = %v, want Duration", key, v.Kind())
	}
}

func installCleanSummarySink(t *testing.T) *cleanSummarySink {
	t.Helper()
	sink := &cleanSummarySink{}
	log.SetTestHandler(t, sink)
	return sink
}

// matching returns every record whose level+msg match and whose component
// equals comp.
func (s *cleanSummarySink) matching(level slog.Level, comp, msg string) []cleanSummaryRecord {
	var out []cleanSummaryRecord
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

// ---- Orphan-daemon sweep summary ----

func TestSweepOrphanDaemons_EmitsCleanSummaryCountingSuccessfulKills(t *testing.T) {
	sink := installCleanSummarySink(t)
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{2001, 2002}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "orphan-daemon sweep complete")
	if rec.level != slog.LevelInfo {
		t.Errorf("summary level = %v, want INFO", rec.level)
	}
	if got := rec.intAttr(t, "killed"); got != 2 {
		t.Errorf("killed = %d, want 2", got)
	}
	rec.requireDuration(t, "took")
}

func TestSweepOrphanDaemons_DemotesPerKillInfoToDebug(t *testing.T) {
	sink := installCleanSummarySink(t)
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{2001}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	// No INFO per-kill line under any component.
	for _, r := range sink.all() {
		if r.level == slog.LevelInfo && r.msg == "orphan killed" {
			t.Errorf("per-kill line must not be INFO: %+v", r)
		}
		if r.level == slog.LevelInfo && r.msg == "sweep: killed orphan daemon" {
			t.Errorf("old per-kill INFO message must be gone: %+v", r)
		}
	}
	// Exactly one per-item DEBUG "orphan killed" under clean, carrying target_pid.
	dbg := sink.matching(slog.LevelDebug, "clean", "orphan killed")
	if len(dbg) != 1 {
		t.Fatalf("expected 1 DEBUG 'orphan killed' under clean, got %d: %+v", len(dbg), sink.all())
	}
	if pid, ok := dbg[0].attrs["target_pid"]; !ok || pid.Int64() != 2001 {
		t.Errorf("DEBUG 'orphan killed' target_pid = %v, want 2001", dbg[0].attrs["target_pid"])
	}
}

func TestSweepOrphanDaemons_ExcludesSkippedAndFailedFromKilled(t *testing.T) {
	sink := installCleanSummarySink(t)
	// 3001 identifies-not-portal-daemon (DEBUG skip), 3002 kill fails (WARN),
	// 3003 succeeds. killed must be exactly 1.
	identify := &recordingIdentify{
		results: map[int]identifyOutcome{
			3001: {res: state.IdentifyNotPortalDaemon},
			3002: {res: state.IdentifyIsPortalDaemon},
			3003: {res: state.IdentifyIsPortalDaemon},
		},
	}
	kill := &recordingKill{errs: map[int]error{3002: errors.New("kill: no such process")}}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{3001, 3002, 3003}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "orphan-daemon sweep complete")
	if got := rec.intAttr(t, "killed"); got != 1 {
		t.Errorf("killed = %d, want 1 (excludes skip + failed kill)", got)
	}

	// Identity-skip stays DEBUG on the bootstrap logger.
	skips := sink.matching(slog.LevelDebug, "bootstrap", "sweep: pid not identity-checked as portal daemon, skipping")
	if len(skips) != 1 {
		t.Errorf("expected 1 identity-skip DEBUG under bootstrap, got %d: %+v", len(skips), sink.all())
	}
	// Kill-failure stays WARN on the bootstrap logger.
	warns := sink.matching(slog.LevelWarn, "bootstrap", "sweep: kill failed")
	if len(warns) != 1 {
		t.Errorf("expected 1 kill-failure WARN under bootstrap, got %d: %+v", len(warns), sink.all())
	}
}

func TestSweepOrphanDaemons_NoSummaryWhenPgrepFails(t *testing.T) {
	sink := installCleanSummarySink(t)
	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return nil, errors.New("pgrep boom") },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     func(pid int) (state.IdentifyResult, error) { return state.IdentifyIsPortalDaemon, nil },
		Kill:         func(pid int) error { return nil },
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	if got := sink.summariesFor("clean", "orphan-daemon sweep complete"); len(got) != 0 {
		t.Errorf("expected no summary on pgrep failure (returns before loop), got %d: %+v", len(got), got)
	}
}

func TestSweepOrphanDaemons_SummaryWithZeroKilledWhenSaverPanePIDErrors(t *testing.T) {
	sink := installCleanSummarySink(t)
	// SaverPanePID errors → empty legitimate set → sweep proceeds. No candidates
	// kill (all identify dead), so killed=0 but the summary is still emitted.
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyDead}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{4001, 4002}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, errors.New("list-panes boom") },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "orphan-daemon sweep complete")
	if got := rec.intAttr(t, "killed"); got != 0 {
		t.Errorf("killed = %d, want 0", got)
	}
	rec.requireDuration(t, "took")
}

// ---- Marker sweep summary ----

func TestCleanStaleMarkers_EmitsCleanSummaryCountingSuccessfulUnsets(t *testing.T) {
	sink := installCleanSummarySink(t)
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"stale1__0.0": {},
		"stale2__1.2": {},
		"live__0.0":   {},
	}}
	live := &fakeLivePaneLister{output: "live:0.0\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &MarkerCleanupCore{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
		Logger:   log.For("bootstrap"),
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "marker sweep complete")
	if rec.level != slog.LevelInfo {
		t.Errorf("summary level = %v, want INFO", rec.level)
	}
	if got := rec.intAttr(t, "unset"); got != 2 {
		t.Errorf("unset = %d, want 2", got)
	}
	rec.requireDuration(t, "took")
}

func TestCleanStaleMarkers_SummaryUnsetCountsOnlySuccessfulUnsets(t *testing.T) {
	sink := installCleanSummarySink(t)
	// Three stale markers; the middle unset fails. unset must be 2, and the
	// aggregate error still returns.
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"a__0.0": {},
		"b__0.0": {},
		"c__0.0": {},
	}}
	live := &fakeLivePaneLister{output: "alive:9.9\n"}
	sentinel := errors.New("tmux: option boom")
	unsetter := &fakeMarkerUnsetter{errs: map[int]error{2: sentinel}}

	c := &MarkerCleanupCore{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
		Logger:   log.For("bootstrap"),
	}
	err := c.CleanStaleMarkers()
	if err == nil {
		t.Fatalf("expected aggregate error when one unset fails; got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected returned error to wrap sentinel %v, got %v", sentinel, err)
	}

	rec := sink.onlySummary(t, "clean", "marker sweep complete")
	if got := rec.intAttr(t, "unset"); got != 2 {
		t.Errorf("unset = %d, want 2 (counts successful unsets only)", got)
	}
}

func TestCleanStaleMarkers_SummaryUnsetZeroOnMassUnsetHazardDeferral(t *testing.T) {
	sink := installCleanSummarySink(t)
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"protected__0.0": {},
		"another__1.2":   {},
	}}
	// No error, but zero panes parsed → mass-unset-hazard deferral.
	live := &fakeLivePaneLister{output: ""}
	unsetter := &fakeMarkerUnsetter{}

	c := &MarkerCleanupCore{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
		Logger:   log.For("bootstrap"),
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers must return nil for mass-unset-hazard deferral; got %v", err)
	}
	if len(unsetter.calls) != 0 {
		t.Errorf("expected zero unset calls under deferral, got %v", unsetter.calls)
	}

	rec := sink.onlySummary(t, "clean", "marker sweep complete")
	if got := rec.intAttr(t, "unset"); got != 0 {
		t.Errorf("unset = %d, want 0 (never a false unset on deferral)", got)
	}
	// The deferral WARN still fires on the bootstrap logger.
	warns := sink.matching(slog.LevelWarn, "bootstrap", "stale-marker cleanup: zero live panes parsed with markers present; skipping to avoid mass-unset hazard (next bootstrap retries)")
	if len(warns) != 1 {
		t.Errorf("expected 1 deferral WARN under bootstrap, got %d: %+v", len(warns), sink.all())
	}
}

func TestCleanStaleMarkers_NoSummaryWhenListErrorReturns(t *testing.T) {
	t.Run("ListSkeletonMarkers error", func(t *testing.T) {
		sink := installCleanSummarySink(t)
		lister := &fakeMarkerLister{err: errors.New("show-options: tmux dead")}
		live := &fakeLivePaneLister{output: "live:0.0\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
			Logger:   log.For("bootstrap"),
		}
		if err := c.CleanStaleMarkers(); err == nil {
			t.Fatalf("expected non-nil error from ListSkeletonMarkers failure")
		}
		if got := sink.summariesFor("clean", "marker sweep complete"); len(got) != 0 {
			t.Errorf("expected no summary on ListSkeletonMarkers error, got %d: %+v", len(got), got)
		}
	})

	t.Run("ListAllPanesWithFormat error", func(t *testing.T) {
		sink := installCleanSummarySink(t)
		lister := &fakeMarkerLister{markers: map[string]struct{}{"m__0.0": {}}}
		live := &fakeLivePaneLister{err: errors.New("list-panes: socket gone")}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
			Logger:   log.For("bootstrap"),
		}
		if err := c.CleanStaleMarkers(); err == nil {
			t.Fatalf("expected non-nil error from ListAllPanesWithFormat failure")
		}
		if got := sink.summariesFor("clean", "marker sweep complete"); len(got) != 0 {
			t.Errorf("expected no summary on ListAllPanesWithFormat error, got %d: %+v", len(got), got)
		}
	})
}

func TestCleanStaleMarkers_SummaryUnsetZeroOnEmptyMarkersNoOp(t *testing.T) {
	sink := installCleanSummarySink(t)
	lister := &fakeMarkerLister{markers: map[string]struct{}{}}
	// Empty live + empty markers → clean no-op return.
	live := &fakeLivePaneLister{output: ""}
	unsetter := &fakeMarkerUnsetter{}

	c := &MarkerCleanupCore{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
		Logger:   log.For("bootstrap"),
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	rec := sink.onlySummary(t, "clean", "marker sweep complete")
	if got := rec.intAttr(t, "unset"); got != 0 {
		t.Errorf("unset = %d, want 0 on empty-markers no-op", got)
	}
	rec.requireDuration(t, "took")
}

// Compile-time guard: tmux import is exercised so goimports does not drop it if
// a future edit removes the only reference; the StructuralKeyFormat constant is
// the canonical live-pane format the marker sweep requests.
var _ = tmux.StructuralKeyFormat
