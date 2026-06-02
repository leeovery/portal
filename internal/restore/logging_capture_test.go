package restore_test

import (
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

// captureSink is a thin wrapper over the shared logtest.Sink that adds
// restore-specific record filtering. The capture-handler base (rendering,
// body, structured records) lives in internal/logtest; only restore's
// recordsWithMessage extension is layered on here.
type captureSink struct {
	*logtest.Sink
}

// capturedRecord retains a record's level, message, and ordered attr keys so
// tests can assert on the exact attr-key set of a summary line (e.g. that the
// geometry summary carries only panes/took/anomalous and no scrollback key).
type capturedRecord struct {
	level slog.Level
	msg   string
	keys  []string
}

// recordsWithMessage returns every captured record whose message equals msg, in
// emission order. Used by precise-attr assertions on summary lines.
func (s *captureSink) recordsWithMessage(msg string) []capturedRecord {
	var out []capturedRecord
	for _, r := range s.Records() {
		if r.Msg == msg {
			out = append(out, capturedRecord{level: r.Level, msg: r.Msg, keys: r.Keys})
		}
	}
	return out
}

// newCaptureLogger returns a *slog.Logger routed into a fresh captureSink and
// the sink itself so tests can inspect the rendered body and filtered records.
func newCaptureLogger(t *testing.T) (*slog.Logger, *captureSink) {
	t.Helper()
	sink := &captureSink{Sink: &logtest.Sink{}}
	return slog.New(sink), sink
}
