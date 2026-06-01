package restore_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// captureSink is a slog.Handler used by the restore package's logging tests.
// It records every record and renders a text body in the shape
// "<LEVEL> <msg> key=value..." so the existing substring assertions (level
// label, message phrase, session/error attrs) keep working after the
// observability migration retyped Orchestrator.Logger / SessionRestorer.Logger
// to *slog.Logger.
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

func (s *captureSink) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

func newCaptureLogger(t *testing.T) (*slog.Logger, *captureSink) {
	t.Helper()
	sink := &captureSink{}
	return slog.New(sink), sink
}
