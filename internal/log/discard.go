package log

import (
	"io"
	"log/slog"
)

// discardLogger is the single process-wide silent *slog.Logger. It is the one
// canonical io.Discard-backed sink in production code; the single-owner
// invariant documented in this package's doc.go forbids constructing a discard
// logger anywhere else. Consumers that tolerate a nil logger route through
// OrDiscard (or Discard for bare-sink construction) rather than re-declaring
// their own.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// OrDiscard returns l when it is non-nil, otherwise the shared discard logger.
// It is the canonical nil-tolerant guard: public functions and step cores that
// may be called with a nil *slog.Logger call OrDiscard once at entry so
// downstream logging is unconditionally safe (a nil *slog.Logger panics on use).
func OrDiscard(l *slog.Logger) *slog.Logger {
	if l == nil {
		return discardLogger
	}
	return l
}

// Discard returns the shared silent logger directly. Use it for default-sink
// construction where there is no candidate logger to fall back from (equivalent
// to OrDiscard(nil)).
func Discard() *slog.Logger {
	return discardLogger
}
