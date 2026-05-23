package bootstrapadapter

// Production adapter for the bootstrap.OrphanSweeper interface — step 4 of
// the eleven-step bootstrap sequence. The adapter is a thin wrapper that
// pins the canonical Pgrep form (`pgrep -fx '^portal state daemon( |$)'`)
// and wires the *tmux.Client's HasSession + FirstPanePIDInSession surface
// into the (saverPID int, err error) seam shape consumed by
// *bootstrap.OrphanSweepCore.
//
// Lives in its own file (sibling to adapters.go) because the orphan-sweep
// adapter pulls in os/exec (for the pgrep shell-out) — keeping that import
// surface scoped to a single file lets the rest of internal/bootstrapadapter
// stay free of process-management dependencies.

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// NewOrphanSweeper builds a fully-wired *bootstrap.OrphanSweepCore — the
// production OrphanSweeper for orchestrator step 4. Pgrep is the canonical
// `pgrep -fx '^portal state daemon( |$)'` enumeration; SaverPanePID reads
// the first pane's PID from the `_portal-saver` session via the
// *tmux.Client; Identify and Kill fall through to the package-internal
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
		Pgrep:        pgrepPortalDaemons,
		SaverPanePID: func() (int, error) { return saverPanePID(client) },
		Logger:       logger,
	}
}

// pgrepDaemonPattern is the canonical `pgrep -fx` regex that matches a live
// `portal state daemon` process. The `-f` flag matches against the full
// argv string; `-x` requires an exact match (the regex must consume the
// whole argv); the anchored regex prevents false positives from e.g.
// `portal state daemon-foo` or `portal state daemon --some-flag` (the
// trailing ` |$` clause allows trailing argv tokens separated by a space,
// while still anchoring the prefix).
//
// Spec § Component B Behaviour #1 — pgrep -fx is the single canonical
// form used by both the implementation and the acceptance criteria.
const pgrepDaemonPattern = "^portal state daemon( |$)"

// pgrepPortalDaemons enumerates candidate PIDs via
// `pgrep -fx '^portal state daemon( |$)'`. Returns:
//
//   - ([]int{...}, nil) on a successful match (one or more candidates).
//   - (nil, nil) when pgrep exits with status 1 AND empty stdout — pgrep's
//     documented "no matches" signal. This is the steady-state form on a
//     clean install where no orphan daemons exist; treating it as an error
//     would force the core to log a WARN on every bootstrap.
//   - (nil, err) on any other non-zero exit or OS-layer failure. The core
//     surfaces the error via Logger.Warn and continues — orphan-sweep is
//     best-effort.
//
// PIDs that cannot be parsed as integers are skipped silently — best-
// effort posture.
func pgrepPortalDaemons() ([]int, error) {
	out, err := exec.Command("pgrep", "-fx", pgrepDaemonPattern).Output()
	if err != nil {
		// pgrep status 1 + empty stdout = no matches (canonical pgrep
		// "nothing found" signal). Treat as a normal empty result.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(strings.TrimSpace(string(out))) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("pgrep %q: %w", pgrepDaemonPattern, err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		// Exit 0 with empty output: defensive guard. pgrep's documented
		// contract is exit 1 on no matches, but the empty-output / exit-0
		// shape is handled here for robustness.
		return nil, nil
	}

	var pids []int
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			// Skip malformed lines silently — best-effort posture.
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// saverPanePID reads the first pane PID of `_portal-saver` via the
// *tmux.Client.
//
// Three observable shapes:
//
//   - (pid, nil) when `_portal-saver` is present with at least one pane.
//   - (0, nil) when `_portal-saver` is absent on the live tmux server
//     (HasSession returns false). The orphan-sweep core treats this as
//     "legitimate set empty" and sweeps the full pgrep result.
//   - (0, err) on any other failure path (FirstPanePIDInSession surface
//     error, pane PID parse failure). The core logs the wrapped error via
//     Logger.Warn and proceeds against the full pgrep result.
func saverPanePID(client *tmux.Client) (int, error) {
	if !client.HasSession(tmux.PortalSaverName) {
		return 0, nil
	}
	return client.FirstPanePIDInSession(tmux.PortalSaverName)
}
