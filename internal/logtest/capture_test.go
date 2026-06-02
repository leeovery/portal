package logtest_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
)

// TestNewCaptureLogger_RendersLevelMessageKeyValue locks the canonical
// "<LEVEL> <msg> key=value" rendering contract that every consumer's
// substring assertions key on. This is the single place the contract is
// now pinned; if it changes here, all consumers (cmd, state, restore)
// must be re-evaluated.
func TestNewCaptureLogger_RendersLevelMessageKeyValue(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	logger.Info("hello world", "session", "demo", "count", 3)

	if got, want := sink.Body(), "INFO hello world session=demo count=3"; got != want {
		t.Errorf("Body() = %q, want %q", got, want)
	}
}

// TestSink_RecordsLevelMessageAndOrderedKeys verifies the structured-record
// view restore relies on for exact attr-key-set assertions.
func TestSink_RecordsLevelMessageAndOrderedKeys(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	logger.Warn("geometry complete", "panes", 4, "took", "5ms", "anomalous", false)

	recs := sink.Records()
	if len(recs) != 1 {
		t.Fatalf("Records() len = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.Level != slog.LevelWarn {
		t.Errorf("Level = %v, want WARN", r.Level)
	}
	if r.Msg != "geometry complete" {
		t.Errorf("Msg = %q, want %q", r.Msg, "geometry complete")
	}
	wantKeys := []string{"panes", "took", "anomalous"}
	if len(r.Keys) != len(wantKeys) {
		t.Fatalf("Keys = %v, want %v", r.Keys, wantKeys)
	}
	for i, k := range wantKeys {
		if r.Keys[i] != k {
			t.Errorf("Keys[%d] = %q, want %q", i, r.Keys[i], k)
		}
	}
}

// TestSink_RendersBoundComponentOnEveryLine confirms a component bound via
// .With("component", …) is rendered on every record — the behaviour cmd's
// component-binding wrapper depends on. State/restore never bind attrs, so
// this path stays dormant for them and their output is unaffected.
func TestSink_RendersBoundComponentOnEveryLine(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	bound := logger.With("component", "daemon")

	bound.Info("lock acquired", "tmux_pane", "%42")

	if got, want := sink.Body(), "INFO lock acquired component=daemon tmux_pane=%42"; got != want {
		t.Errorf("Body() = %q, want %q", got, want)
	}
}

// TestSink_Lines_ReturnsCopy verifies the Lines accessor returns a snapshot
// the caller may retain and inspect (used by cmd's at-exit snapshot tests).
func TestSink_Lines_ReturnsCopy(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	logger.Info("one")
	snapshot := sink.Lines()
	logger.Info("two")

	if len(snapshot) != 1 {
		t.Fatalf("snapshot len = %d, want 1 (must not see later writes)", len(snapshot))
	}
	if snapshot[0] != "INFO one" {
		t.Errorf("snapshot[0] = %q, want %q", snapshot[0], "INFO one")
	}
}

// TestSink_RecordsAttrValues verifies the structured-attr-map view (Record.Attrs)
// that the eleven structured-attr-map consumers assert on: a component bound via
// WithAttrs plus per-call attrs both land in the map with their slog.Value
// (preserving Kind), and the Attrs key set agrees with Keys.
func TestSink_RecordsAttrValues(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	bound := logger.With("component", "daemon")

	bound.Info("tick complete", "panes", 4, "took", 3*time.Second)

	recs := sink.Records()
	if len(recs) != 1 {
		t.Fatalf("Records() len = %d, want 1", len(recs))
	}
	r := recs[0]

	comp, ok := r.Attrs["component"]
	if !ok {
		t.Fatalf("Attrs missing bound component: %+v", r.Attrs)
	}
	if comp.String() != "daemon" {
		t.Errorf("component = %q, want %q", comp.String(), "daemon")
	}
	panes, ok := r.Attrs["panes"]
	if !ok || panes.Kind() != slog.KindInt64 || panes.Int64() != 4 {
		t.Errorf("panes attr = %+v, want Int64 4", panes)
	}
	took, ok := r.Attrs["took"]
	if !ok || took.Kind() != slog.KindDuration || took.Duration() != 3*time.Second {
		t.Errorf("took attr = %+v, want Duration 3s", took)
	}

	// Attrs key set agrees with Keys (same bound+call attrs).
	if len(r.Attrs) != len(r.Keys) {
		t.Fatalf("Attrs key count %d != Keys count %d (%v / %v)", len(r.Attrs), len(r.Keys), r.Attrs, r.Keys)
	}
	for _, k := range r.Keys {
		if _, ok := r.Attrs[k]; !ok {
			t.Errorf("key %q present in Keys but missing from Attrs", k)
		}
	}
}

// TestSink_RecordsAttrValues_LastWriteWins pins the bound-then-call ordering: a
// per-call attr re-using a bound key overwrites the bound value (matching the
// eleven copies, which ranged bound first, then the call attrs).
func TestSink_RecordsAttrValues_LastWriteWins(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	bound := logger.With("via", "internal")

	bound.Info("set", "via", "cli")

	recs := sink.Records()
	if len(recs) != 1 {
		t.Fatalf("Records() len = %d, want 1", len(recs))
	}
	via, ok := recs[0].Attrs["via"]
	if !ok {
		t.Fatalf("Attrs missing via: %+v", recs[0].Attrs)
	}
	if via.String() != "cli" {
		t.Errorf("via = %q, want %q (per-call attr must win over bound)", via.String(), "cli")
	}
}

// TestRecord_AttrString covers the value-by-string accessor and its
// missing-key *testing.T-failing path.
func TestRecord_AttrString(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("set", "op", "set")

	r := sink.OnlyRecord(t)
	if got := r.AttrString(t, "op"); got != "set" {
		t.Errorf("AttrString(op) = %q, want %q", got, "set")
	}

	// Missing key fails the supplied TestingT.
	if !expectFail(func(sub logtest.TestingT) { r.AttrString(sub, "nope") }) {
		t.Errorf("AttrString must fail the test for a missing key")
	}
}

// TestRecord_IntAttr covers the Int64 accessor: extraction by kind and the
// failing path for a non-Int64 attr.
func TestRecord_IntAttr(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("tick complete", "panes", 7, "msg", "text")

	r := sink.OnlyRecord(t)
	if got := r.IntAttr(t, "panes"); got != 7 {
		t.Errorf("IntAttr(panes) = %d, want 7", got)
	}

	// A non-Int64 attr fails.
	if !expectFail(func(sub logtest.TestingT) { r.IntAttr(sub, "msg") }) {
		t.Errorf("IntAttr must fail for a non-Int64 attr")
	}
}

// TestRecord_RequireDuration covers the duration-kind assertion: a Duration attr
// passes; a non-Duration attr fails.
func TestRecord_RequireDuration(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("tick complete", "took", 5*time.Millisecond, "entries", "2")

	r := sink.OnlyRecord(t)
	r.RequireDuration(t, "took") // must not fail

	if !expectFail(func(sub logtest.TestingT) { r.RequireDuration(sub, "entries") }) {
		t.Errorf("RequireDuration must fail for a non-Duration attr")
	}
}

// TestRecord_HasAttr covers the presence predicate.
func TestRecord_HasAttr(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("rm", "op", "rm")

	r := sink.OnlyRecord(t)
	if !r.HasAttr("op") {
		t.Errorf("HasAttr(op) = false, want true")
	}
	if r.HasAttr("value") {
		t.Errorf("HasAttr(value) = true, want false")
	}
}

// TestSink_OnlyRecord covers the single-record accessor and its
// not-exactly-one failing path.
func TestSink_OnlyRecord(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	logger.Info("first")

	r := sink.OnlyRecord(t)
	if r.Msg != "first" {
		t.Errorf("OnlyRecord().Msg = %q, want %q", r.Msg, "first")
	}

	logger.Info("second")
	if !expectFail(func(sub logtest.TestingT) { sink.OnlyRecord(sub) }) {
		t.Errorf("OnlyRecord must fail when more than one record was captured")
	}
}

// fakeT is a minimal stand-in for the logtest.TestingT interface so the
// *testing.T-failing accessor paths can be exercised without aborting the outer
// test. Fatalf records the failure and aborts via panic, which expectFail
// recovers (mirroring how a real t.Fatalf unwinds via runtime.Goexit).
type fakeT struct {
	failed bool
}

func (f *fakeT) Helper() {}

func (f *fakeT) Fatalf(string, ...any) {
	f.failed = true
	panic(fakeFatal{})
}

type fakeFatal struct{}

// expectFail runs fn with a fakeT, recovering the fakeFatal panic that a
// failing accessor raises, and reports whether the accessor failed the
// supplied TestingT.
func expectFail(fn func(logtest.TestingT)) (failed bool) {
	t := &fakeT{}
	defer func() {
		_ = recover()
		failed = t.failed
	}()
	fn(t)
	return t.failed
}
