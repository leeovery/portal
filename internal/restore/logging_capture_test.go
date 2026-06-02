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
	mu      sync.Mutex
	lines   []string
	records []capturedRecord
}

// capturedRecord retains a record's level, message, and ordered attr keys so
// tests can assert on the exact attr-key set of a summary line (e.g. that the
// geometry summary carries only panes/took/anomalous and no scrollback key).
type capturedRecord struct {
	level slog.Level
	msg   string
	keys  []string
}

func (s *captureSink) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (s *captureSink) WithAttrs(_ []slog.Attr) slog.Handler         { return s }
func (s *captureSink) WithGroup(_ string) slog.Handler              { return s }

func (s *captureSink) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Level.String())
	b.WriteString(" ")
	b.WriteString(r.Message)
	var keys []string
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		keys = append(keys, a.Key)
		return true
	})
	s.mu.Lock()
	s.lines = append(s.lines, b.String())
	s.records = append(s.records, capturedRecord{level: r.Level, msg: r.Message, keys: keys})
	s.mu.Unlock()
	return nil
}

func (s *captureSink) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

// recordsWithMessage returns every captured record whose message equals msg, in
// emission order. Used by precise-attr assertions on summary lines.
func (s *captureSink) recordsWithMessage(msg string) []capturedRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []capturedRecord
	for _, r := range s.records {
		if r.msg == msg {
			out = append(out, r)
		}
	}
	return out
}

func newCaptureLogger(t *testing.T) (*slog.Logger, *captureSink) {
	t.Helper()
	sink := &captureSink{}
	return slog.New(sink), sink
}
