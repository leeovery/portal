package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// FlagSweepCore lowers the session flags in a state directory that nothing
// else would ever lower: a session's hydrating or resume-pending flag once it
// isn't live, whose helper or panel went with it, and a hydrating flag on a
// live session that isn't running restore's helper, as a bootstrap that died
// before its Create leaves for whoever makes the session next. Left up, either
// keeps the saver off that session for good. Every field must be supplied; a
// sweep missing one lowers nothing.
type FlagSweepCore struct {
	Dir string
	// List lists the live sessions, answering or not.
	List func(ctx context.Context) ([]backend.Session, error)
	// RunsHelper reports whether a live session runs restore's hydrate helper
	// for its own name, which lowers its hydrating flag itself.
	RunsHelper func(backend.Session) bool
	Logger     *slog.Logger
}

var _ FlagSweeper = (*FlagSweepCore)(nil)

// SweepStaleFlags lowers the stale flags. A listing that fails lowers nothing,
// as nothing can be told live; a session that didn't answer keeps its flags,
// as its command can't be read. It fails only when it can't tell what to
// lower, and counts each flag it couldn't, with a WARN.
func (c *FlagSweepCore) SweepStaleFlags(ctx context.Context) error {
	logger := log.OrDiscard(c.Logger)
	if c.Dir == "" || c.List == nil || c.RunsHelper == nil {
		return errors.New("flag sweep: not wired, nothing lowered")
	}
	start := time.Now()
	sessions, err := c.List(ctx)
	if err != nil {
		return fmt.Errorf("flag sweep: list the sessions, nothing lowered: %w", err)
	}
	live := make(map[string]backend.Session, len(sessions))
	for _, s := range sessions {
		live[s.Name] = s
	}
	lowered := 0
	for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
		names, err := flag.List(c.Dir)
		if err != nil {
			logger.Warn("flag sweep: flags unreadable, skipping", "error", err)
			continue
		}
		for _, name := range names {
			if why := c.stale(flag, live, name); why != "" && c.lower(logger, flag, name, why) {
				lowered++
			}
		}
	}
	cleanLogger.Info("stale-flag sweep complete", "unset", lowered, log.Took(start))
	return nil
}

// stale says why the named session's flag is stale, and is empty when it
// isn't.
func (c *FlagSweepCore) stale(flag state.SessionFlag, live map[string]backend.Session, name string) string {
	s, ok := live[name]
	switch {
	case !ok:
		return "its session isn't live"
	case flag == state.Hydrating && s.Answered && !c.RunsHelper(s):
		return "its session isn't running restore's helper"
	}
	return ""
}

// lower lowers the named session's flag, and reports whether it did.
func (c *FlagSweepCore) lower(logger *slog.Logger, flag state.SessionFlag, name, why string) bool {
	if _, err := flag.Clear(c.Dir, name); err != nil {
		logger.Warn("flag sweep: lower failed", "session", name, "detail", string(flag), "error", err)
		return false
	}
	cleanLogger.Debug("stale flag lowered", "session", name, "detail", string(flag)+": "+why)
	return true
}
