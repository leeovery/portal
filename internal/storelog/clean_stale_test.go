package storelog_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/storelog"
)

// captureSink records every emitted record together with the attrs bound via
// WithAttrs (notably the component attr that log.For binds at the logger). It
// mirrors the hooks/project store test sinks so the helper's emission can be
// asserted faithfully.
type captureSink struct {
	mu      sync.Mutex
	records []captureRecord
	shared  *captureSink
	bound   []slog.Attr
}

type captureRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
}

func (s *captureSink) owner() *captureSink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *captureSink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *captureSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &captureSink{shared: s.owner(), bound: next}
}

func (s *captureSink) WithGroup(_ string) slog.Handler {
	return &captureSink{shared: s.owner(), bound: s.bound}
}

func (s *captureSink) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())
	for _, a := range s.bound {
		attrs[a.Key] = a.Value
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	rec := captureRecord{level: r.Level, msg: r.Message, attrs: attrs}
	owner := s.owner()
	owner.mu.Lock()
	owner.records = append(owner.records, rec)
	owner.mu.Unlock()
	return nil
}

func (s *captureSink) all() []captureRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]captureRecord, len(s.records))
	copy(out, s.records)
	return out
}

func installCapture(t *testing.T) *captureSink {
	t.Helper()
	sink := &captureSink{}
	log.SetTestHandler(t, sink)
	return sink
}

func (s *captureSink) onlyRecord(t *testing.T) captureRecord {
	t.Helper()
	recs := s.all()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 log record, got %d: %+v", len(recs), recs)
	}
	return recs[0]
}

func (r captureRecord) attrString(t *testing.T, key string) string {
	t.Helper()
	v, ok := r.attrs[key]
	if !ok {
		t.Fatalf("record missing attr %q: %+v", key, r.attrs)
	}
	return v.String()
}

func TestEmitCleanStaleSummary_SuccessInfo(t *testing.T) {
	logger := log.For("hooks")
	sink := installCapture(t)

	storelog.EmitCleanStaleSummary(logger, 2, time.Now().Add(-5*time.Millisecond), nil)

	rec := sink.onlyRecord(t)
	if rec.level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.level)
	}
	if rec.msg != "clean-stale" {
		t.Errorf("msg = %q, want %q", rec.msg, "clean-stale")
	}
	if got := rec.attrString(t, "op"); got != "clean-stale" {
		t.Errorf("op = %q, want %q", got, "clean-stale")
	}
	if got := rec.attrString(t, "component"); got != "hooks" {
		t.Errorf("component = %q, want %q", got, "hooks")
	}
	if got := rec.attrString(t, "entries"); got != "2" {
		t.Errorf("entries = %q, want %q", got, "2")
	}
	if got := rec.attrString(t, "via"); got != "internal" {
		t.Errorf("via = %q, want %q", got, "internal")
	}
	tookVal, ok := rec.attrs["took"]
	if !ok {
		t.Fatalf("summary missing took attr: %+v", rec.attrs)
	}
	if tookVal.Kind() != slog.KindDuration {
		t.Errorf("took attr kind = %v, want Duration", tookVal.Kind())
	}
	// A successful summary must NOT carry error / error_class.
	if _, ok := rec.attrs["error"]; ok {
		t.Errorf("success summary must omit error attr: %+v", rec.attrs)
	}
	if _, ok := rec.attrs["error_class"]; ok {
		t.Errorf("success summary must omit error_class attr: %+v", rec.attrs)
	}
}

func TestEmitCleanStaleSummary_FailureWarn(t *testing.T) {
	logger := log.For("projects")
	sink := installCapture(t)

	saveErr := fmt.Errorf("%w: boom", fileutil.ErrWriteTempCreate)
	storelog.EmitCleanStaleSummary(logger, 3, time.Now().Add(-5*time.Millisecond), saveErr)

	rec := sink.onlyRecord(t)
	if rec.level != slog.LevelWarn {
		t.Errorf("level = %v, want WARN", rec.level)
	}
	if rec.msg != "clean-stale" {
		t.Errorf("msg = %q, want %q", rec.msg, "clean-stale")
	}
	if got := rec.attrString(t, "op"); got != "clean-stale" {
		t.Errorf("op = %q, want %q", got, "clean-stale")
	}
	if got := rec.attrString(t, "component"); got != "projects" {
		t.Errorf("component = %q, want %q", got, "projects")
	}
	if got := rec.attrString(t, "entries"); got != "3" {
		t.Errorf("entries = %q, want %q", got, "3")
	}
	if got := rec.attrString(t, "via"); got != "internal" {
		t.Errorf("via = %q, want %q", got, "internal")
	}
	// error_class is derived from fileutil.ClassifyWriteError inside the helper.
	if got := rec.attrString(t, "error_class"); got != "write-failed-temp-create" {
		t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
	}
	tookVal, ok := rec.attrs["took"]
	if !ok {
		t.Fatalf("WARN missing took attr: %+v", rec.attrs)
	}
	if tookVal.Kind() != slog.KindDuration {
		t.Errorf("took attr kind = %v, want Duration", tookVal.Kind())
	}
	errVal, ok := rec.attrs["error"]
	if !ok {
		t.Fatalf("WARN record missing error attr: %+v", rec.attrs)
	}
	loggedErr, ok := errVal.Any().(error)
	if !ok {
		t.Fatalf("error attr is not an error value: %T", errVal.Any())
	}
	if loggedErr != saveErr {
		t.Errorf("logged error attr = %v, want the raw saveErr %v", loggedErr, saveErr)
	}
}
