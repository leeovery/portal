package log

import (
	"log/slog"
	"time"
)

// Took is the single source of truth for the reserved "took" cycle-summary
// attr key and its time.Duration type. Every cycle/sweep/tick summary line
// (spec § Cycle-level summary cadence and shape) ends its terminal Info call
// with log.Took(start) so the attr key and Duration type are pinned in one
// place rather than re-typed by hand at each call site. The emitted output is
// byte-identical to the hand-written "took", time.Since(start) pair
// (took=<duration>).
func Took(start time.Time) slog.Attr {
	return slog.Duration("took", time.Since(start))
}
