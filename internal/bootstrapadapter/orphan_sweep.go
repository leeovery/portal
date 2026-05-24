package bootstrapadapter

// Production adapter for the bootstrap.OrphanSweeper interface — step 4 of
// the eleven-step bootstrap sequence. The adapter is a thin wrapper that
// pins the canonical Pgrep form (`pgrep -fx '^portal state daemon( |$)'`,
// via state.PgrepPortalDaemons) and wires tmux.SaverPanePID into the
// (saverPID int, err error) seam shape consumed by *bootstrap.OrphanSweepCore.
//
// Lives in its own file (sibling to adapters.go) because the orphan-sweep
// adapter wires tmux-specific helpers — keeping that import surface scoped
// to a single file lets the rest of internal/bootstrapadapter stay focused.

import (
	"errors"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// NewOrphanSweeper builds a fully-wired *bootstrap.OrphanSweepCore — the
// production OrphanSweeper for orchestrator step 4. Pgrep is the canonical
// `pgrep -fx '^portal state daemon( |$)'` enumeration (state.PgrepPortalDaemons,
// the single source of truth shared with the portaltest helper);
// SaverPanePID reads the first pane's PID from the `_portal-saver` session
// via the *tmux.Client; Identify and Kill fall through to the package-internal
// defaults (state.IdentifyDaemon and syscall.Kill(pid, SIGKILL)).
//
// client must be non-nil; behaviour with a nil client is undefined and
// will panic at the first SaverPanePID invocation (matching the codebase's
// "explicit fields, fail loud" adapter convention).
//
// logger is forwarded to the underlying *OrphanSweepCore so DEBUG / INFO /
// WARN diagnostics under state.ComponentBootstrap land in portal.log. nil
// is tolerated — *state.Logger is itself nil-safe, and the core
// substitutes its no-op default at entry.
func NewOrphanSweeper(client *tmux.Client, logger *state.Logger) bootstrap.OrphanSweeper {
	return &bootstrap.OrphanSweepCore{
		Pgrep:        state.PgrepPortalDaemons,
		SaverPanePID: func() (int, error) { return saverPanePID(client) },
		Logger:       logger,
	}
}

// saverPanePID reads the first pane PID of `_portal-saver` via
// tmux.SaverPanePID.
//
// Three observable shapes:
//
//   - (pid, nil) when `_portal-saver` is present with at least one pane.
//   - (0, nil) when `_portal-saver` is absent on the live tmux server
//     (tmux.ErrNoSuchSession) OR present but reports zero panes
//     (tmux.ErrEmptyPaneList). The orphan-sweep core treats both as
//     "legitimate set empty" and sweeps the full pgrep result.
//   - (0, err) on any other failure path (generic exec failure,
//     tmux.ErrPanePIDParse). The core logs the wrapped error via
//     Logger.Warn and proceeds against the full pgrep result.
//
// The HasSession pre-check is intentionally omitted: tmux.SaverPanePID
// classifies absent-session errors as tmux.ErrNoSuchSession via the
// wrapNoSuchSession helper, which we collapse to (0, nil) here. Skipping
// the pre-check also closes the small HasSession→list-panes race window
// in which the saver could be destroyed between the two calls.
func saverPanePID(client *tmux.Client) (int, error) {
	pid, err := tmux.SaverPanePID(client, tmux.PortalSaverName)
	if err != nil {
		if errors.Is(err, tmux.ErrNoSuchSession) || errors.Is(err, tmux.ErrEmptyPaneList) {
			return 0, nil
		}
		return 0, err
	}
	return pid, nil
}
