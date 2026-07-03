package bootstrapadapter

// Production adapter for the bootstrap.OrphanSweeper interface — step 4 of
// the ten-step bootstrap sequence. The adapter is a thin wrapper that
// pins the canonical Pgrep form (`pgrep -fx '^portal state daemon( |$)'`,
// via state.PgrepPortalDaemons) and forwards tmux.SaverPanePIDOrAbsent
// verbatim into the (pid int, present bool, err error) seam consumed by
// *bootstrap.OrphanSweepCore — preserving the tri-state contract at the
// type level so absent ((0, false, nil)) and "present with pid 0"
// ((0, true, nil)) cannot collapse into one another.
//
// Lives in its own file (sibling to adapters.go) because the orphan-sweep
// adapter wires tmux-specific helpers — keeping that import surface scoped
// to a single file lets the rest of internal/bootstrapadapter stay focused.

import (
	"log/slog"

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
// WARN diagnostics under the bootstrap component land in portal.log. nil
// is tolerated — the core substitutes its io.Discard-backed default at
// entry.
func NewOrphanSweeper(client *tmux.Client, logger *slog.Logger) bootstrap.OrphanSweeper {
	return &bootstrap.OrphanSweepCore{
		Pgrep: state.PgrepPortalDaemons,
		SaverPanePID: func() (pid int, present bool, err error) {
			return tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)
		},
		Logger: logger,
	}
}
