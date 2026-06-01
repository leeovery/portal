package state_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// captureSink is a slog.Handler used by the state package's logging tests. It
// records every record and renders a text body in the shape
// "<LEVEL> <msg> key=value..." so the existing substring assertions (level
// label, paneKey, error text, path) keep working after the observability
// migration retyped every logging seam to *slog.Logger.
//
// It records every level unconditionally — PORTAL_LOG_LEVEL filtering is a
// handler concern owned by internal/log in production, not by these unit
// tests, which assert that a given line was emitted at a given level.
type captureSink struct {
	mu    sync.Mutex
	lines []string
}

func (s *captureSink) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (s *captureSink) WithAttrs(_ []slog.Attr) slog.Handler         { return s }
func (s *captureSink) WithGroup(_ string) slog.Handler              { return s }

func (s *captureSink) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Level.String())
	b.WriteString(" ")
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		return true
	})
	s.mu.Lock()
	s.lines = append(s.lines, b.String())
	s.mu.Unlock()
	return nil
}

// body returns every captured line joined by newlines.
func (s *captureSink) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

// newCaptureLogger returns a *slog.Logger routed into a fresh captureSink and
// the sink itself so tests can inspect the rendered body.
func newCaptureLogger(t *testing.T) (*slog.Logger, *captureSink) {
	t.Helper()
	sink := &captureSink{}
	return slog.New(sink), sink
}
