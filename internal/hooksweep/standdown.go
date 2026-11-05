package hooksweep

import (
	"context"
	"log/slog"

	"github.com/leeovery/portal/internal/hooks"
)

// standDownMsg and standDownAttrs give every stand-down one line shape, so a
// single grep answers whether the prune declined and why.
const standDownMsg = "clean-stale-skipped"

func standDownAttrs(reason Reason, extra ...any) []any {
	return append([]any{"op", standDownMsg, "via", hooks.ViaInternal.String(), "reason", string(reason)}, extra...)
}

// StandDown is a declined cycle: the reason its caller reports alongside the
// attrs its log line carries. The zero value means the cycle ran. Emission is
// the sweep's — a diagnosis that declines has pruned nothing and must not
// claim a stand-down in the reaper's own vocabulary.
type StandDown struct {
	reason Reason
	attrs  []any
}

// Reason names the condition that declined the cycle, empty for a cycle that
// ran.
func (s StandDown) Reason() Reason { return s.reason }

// Declined reports whether the cycle stood down at all.
func (s StandDown) Declined() bool { return s.reason != "" }

// emit logs the stand-down at DEBUG: a deferred prune loses nothing, and the
// next cycle retries it. One that waits on the tmux version's hooks to be
// migrated warns, since no cycle clears it by itself. A doctor --fix that
// stood down prints its reason.
func (s StandDown) emit() {
	level := slog.LevelDebug
	if s.reason == ReasonTmuxKeys {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, standDownMsg, standDownAttrs(s.reason, s.attrs...)...)
}

func decline(reason Reason, attrs ...any) StandDown {
	return StandDown{reason: reason, attrs: attrs}
}

// declinedError is a stand-down in transit — the reason its guard decided on,
// carried by the very error that aborts the clean, so a decline names its
// reason at the site that returns it rather than in a variable beside it.
type declinedError struct {
	StandDown
}

func (e declinedError) Error() string {
	return "hook staleness cycle declined: " + string(e.reason)
}
