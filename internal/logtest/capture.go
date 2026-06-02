// Package logtest provides a shared in-process log-capturing slog.Handler for
// portal's logging unit tests. It is a leaf, test-only helper: it imports
// nothing portal-internal and production (non-test) code must not import it
// (mirroring internal/portaltest, internal/restoretest, etc.).
//
// The capturing Sink records every record and renders a text body in the
// canonical shape
//
//	<LEVEL> <msg> key=value...
//
// (a component bound via .With("component", …) is rendered on every line so
// component-scoped loggers read back the way production text output does).
// This rendering is the contract every consumer's substring assertions key on
// — cmd, internal/state, and internal/restore all share this one declaration
// so the shape changes in exactly one place. Sink also retains a structured
// view of each record (level, message, ordered attr keys, and the attr-value
// map) for callers that assert on the exact attr-key set or the typed attr
// values of a line rather than its rendered text.
package logtest

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// TestingT is the subset of *testing.T the failing-path accessors depend on.
// Accepting an interface (rather than *testing.T) lets the accessors' own
// failure paths be unit-tested without aborting the harness; production callers
// pass their *testing.T.
type TestingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Record is a flattened, structured view of one captured slog.Record: its
// level, message, the ordered keys of its attrs (including any bound via
// WithAttrs), and the attr-value map keyed by attr name. Callers use Keys to
// assert on the exact attr-key set of a line — e.g. that a geometry summary
// carries only panes/took/anomalous and no scrollback key — and Attrs to assert
// on the typed attr values (e.g. that took is a slog.KindDuration, or that a
// component=capture line carries the expected int counts).
type Record struct {
	Level slog.Level
	Msg   string
	Keys  []string
	// Attrs maps each attr key to its slog.Value (bound attrs first, then
	// per-call attrs, last-write-wins on a duplicate key). It carries the same
	// key set as Keys.
	Attrs map[string]slog.Value
}

// AttrString returns the string rendering (slog.Value.String) of the attr named
// key, failing the test if the record carries no such attr.
func (r Record) AttrString(t TestingT, key string) string {
	t.Helper()
	v, ok := r.Attrs[key]
	if !ok {
		t.Fatalf("record missing attr %q: %+v", key, r.Attrs)
	}
	return v.String()
}

// IntAttr returns the int64 value of the attr named key, failing the test if it
// is absent or is not a slog.KindInt64 value.
func (r Record) IntAttr(t TestingT, key string) int64 {
	t.Helper()
	v, ok := r.Attrs[key]
	if !ok {
		t.Fatalf("record missing attr %q: %+v", key, r.Attrs)
	}
	if v.Kind() != slog.KindInt64 {
		t.Fatalf("attr %q kind = %v, want Int64: %+v", key, v.Kind(), v)
	}
	return v.Int64()
}

// RequireDuration fails the test unless the attr named key is present and is a
// slog.KindDuration value (the text-mode rendering of a time.Duration took
// attr is indistinguishable from a stringified one, so kind must be asserted).
func (r Record) RequireDuration(t TestingT, key string) {
	t.Helper()
	v, ok := r.Attrs[key]
	if !ok {
		t.Fatalf("record missing attr %q: %+v", key, r.Attrs)
	}
	if v.Kind() != slog.KindDuration {
		t.Fatalf("attr %q kind = %v, want Duration", key, v.Kind())
	}
}

// HasAttr reports whether the record carries an attr named key.
func (r Record) HasAttr(key string) bool {
	_, ok := r.Attrs[key]
	return ok
}

// Sink is a slog.Handler that captures every record into an in-memory buffer
// and exposes both the rendered body and the structured records for
// assertion. The zero value is ready to use as a root sink.
type Sink struct {
	mu      sync.Mutex
	lines   []string
	records []Record
	// shared points at the lines-owning sink so handlers derived via
	// WithAttrs/WithGroup (notably the .With("component", …) binding) record
	// into the same buffer; nil on the root sink.
	shared *Sink
	// bound holds attrs accumulated via WithAttrs so a component bound at the
	// logger (not at each call site) is rendered on every record.
	bound []slog.Attr
}

func (s *Sink) owner() *Sink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

// Enabled records every level unconditionally — PORTAL_LOG_LEVEL filtering is
// a handler concern owned by internal/log in production, not by these unit
// tests, which assert that a given line was emitted at a given level.
func (s *Sink) Enabled(_ context.Context, _ slog.Level) bool { return true }

// WithAttrs returns a derived handler that records into the same owning buffer
// with the supplied attrs appended to its bound set.
func (s *Sink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &Sink{shared: s.owner(), bound: next}
}

// WithGroup is a passthrough that preserves the bound attrs and owning buffer.
func (s *Sink) WithGroup(_ string) slog.Handler {
	return &Sink{shared: s.owner(), bound: s.bound}
}

// Handle renders the record into the canonical "<LEVEL> <msg> key=value..."
// shape (bound attrs first, then per-call attrs) and appends both the rendered
// line and a structured Record to the owning buffer. The same single traversal
// that builds the rendered line and the ordered Keys also populates the
// attr-value map (last-write-wins on a duplicate key, matching the
// bound-then-call iteration order).
func (s *Sink) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Level.String())
	b.WriteString(" ")
	b.WriteString(r.Message)
	keys := make([]string, 0, len(s.bound)+r.NumAttrs())
	attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())
	for _, a := range s.bound {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		keys = append(keys, a.Key)
		attrs[a.Key] = a.Value
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		keys = append(keys, a.Key)
		attrs[a.Key] = a.Value
		return true
	})
	owner := s.owner()
	owner.mu.Lock()
	owner.lines = append(owner.lines, b.String())
	owner.records = append(owner.records, Record{Level: r.Level, Msg: r.Message, Keys: keys, Attrs: attrs})
	owner.mu.Unlock()
	return nil
}

// Body returns every captured line joined by newlines.
func (s *Sink) Body() string {
	owner := s.owner()
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return strings.Join(owner.lines, "\n")
}

// Lines returns a snapshot copy of the captured lines in emission order. The
// returned slice is safe to retain — later writes do not mutate it.
func (s *Sink) Lines() []string {
	owner := s.owner()
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return append([]string(nil), owner.lines...)
}

// Records returns a snapshot copy of the captured structured records in
// emission order.
func (s *Sink) Records() []Record {
	owner := s.owner()
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return append([]Record(nil), owner.records...)
}

// OnlyRecord returns the single captured record, failing the test if there is
// not exactly one. It is the shared form of the per-file onlyRecord helper the
// store/migrate logging tests assert single-emission with.
func (s *Sink) OnlyRecord(t TestingT) Record {
	t.Helper()
	recs := s.Records()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 log record, got %d: %+v", len(recs), recs)
	}
	return recs[0]
}

// NewCaptureLogger returns a *slog.Logger routed into a fresh Sink and the
// sink itself so tests can inspect the rendered body and structured records.
func NewCaptureLogger(t *testing.T) (*slog.Logger, *Sink) {
	t.Helper()
	sink := &Sink{}
	return slog.New(sink), sink
}
