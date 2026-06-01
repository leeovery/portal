package state

import (
	"io"
	"log/slog"
)

// discardLogger is the canonical silent *slog.Logger used when a public
// state-package function is called with a nil logger. The legacy bespoke logger
// was nil-safe (a nil receiver short-circuited every write); the observability
// migration retyped these seams to *slog.Logger, which panics on a nil
// receiver. loggerOrDiscard preserves the old nil-tolerant contract so callers
// (and tests) may still pass nil to mean "do not log".
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// loggerOrDiscard returns logger when non-nil, else the shared discardLogger.
// Public state-package functions that may receive a nil logger call this once
// at entry so downstream logging is unconditionally safe.
func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return discardLogger
	}
	return logger
}
