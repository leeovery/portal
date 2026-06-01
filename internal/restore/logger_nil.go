package restore

import (
	"io"
	"log/slog"
)

// discardLogger is the canonical silent *slog.Logger used when an Orchestrator
// or SessionRestorer is constructed without a Logger. The legacy bespoke logger
// was nil-safe (a nil receiver short-circuited every write); the observability
// migration retyped the Logger fields to *slog.Logger, which panics on a nil
// receiver. The logger() accessors preserve the old nil-tolerant contract so
// callers (and tests) may leave Logger unset.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func (o *Orchestrator) logger() *slog.Logger {
	if o.Logger == nil {
		return discardLogger
	}
	return o.Logger
}

func (r *SessionRestorer) logger() *slog.Logger {
	if r.Logger == nil {
		return discardLogger
	}
	return r.Logger
}
