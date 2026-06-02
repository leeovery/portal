package state

import (
	"log/slog"

	"github.com/leeovery/portal/internal/log"
)

// loggerOrDiscard returns logger when non-nil, else the shared discard logger
// owned by internal/log. The legacy bespoke logger was nil-safe (a nil receiver
// short-circuited every write); the observability migration retyped these seams
// to *slog.Logger, which panics on a nil receiver. This forwarder preserves the
// old nil-tolerant contract so callers (and tests) may still pass nil to mean
// "do not log". Public state-package functions that may receive a nil logger
// call this once at entry so downstream logging is unconditionally safe.
func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	return log.OrDiscard(logger)
}
