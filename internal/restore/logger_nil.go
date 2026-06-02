package restore

import (
	"log/slog"

	"github.com/leeovery/portal/internal/log"
)

// The logger() accessors return the shared discard logger owned by internal/log
// when no Logger was injected. The legacy bespoke logger was nil-safe (a nil
// receiver short-circuited every write); the observability migration retyped
// the Logger fields to *slog.Logger, which panics on a nil receiver. These
// forwarders preserve the old nil-tolerant contract so callers (and tests) may
// leave Logger unset.

func (o *Orchestrator) logger() *slog.Logger {
	return log.OrDiscard(o.Logger)
}

func (r *SessionRestorer) logger() *slog.Logger {
	return log.OrDiscard(r.Logger)
}
