package bootstrap

import (
	"log/slog"
	"os"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// defaultKill is the production default for OrphanSweepCore.Kill: sends
// SIGKILL to the supplied PID. SIGKILL (not SIGTERM) is the spec-mandated
// signal per Component B — the orphan view is untrusted, so no final
// flush is permitted (same reasoning as Component A's escalation).
func defaultKill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}

// OrphanSweeper is the orchestrator step seam for Component B: it enumerates
// every live `portal state daemon` process, builds the legitimate set from
// the `_portal-saver` pane's PID, and SIGKILLs every non-legitimate identity-
// checked candidate. Best-effort: SweepOrphanDaemons never returns a non-nil
// error — every failure path is logged via Logger.Warn under
// the bootstrap component and swallowed.
//
// The concrete *OrphanSweepCore in this file satisfies this interface and is
// the production implementation. Adapter wiring (production Pgrep /
// SaverPanePID / Identify / Kill seam closures) lives in
// internal/bootstrapadapter — out of scope for this file.
type OrphanSweeper interface {
	SweepOrphanDaemons() error
}

// OrphanSweepCore is the dependency-injected sweeper that implements
// Component B's bootstrap-time orphan daemon enumeration and kill loop.
//
// Each seam is a function field (rather than an interface) because each
// dependency is a single-arity primitive — wrapping each in an interface
// would add naming overhead without separating concerns the way
// MarkerCleanupCore's multi-call dependencies (ListSkeletonMarkers,
// ListAllPanesWithFormat, UnsetServerOption) do.
//
// Production wiring (Task 4-4) supplies non-nil values for Pgrep and
// SaverPanePID. Identify and Kill default to state.IdentifyDaemon and a
// syscall.Kill(pid, SIGKILL) closure respectively when nil — sensible
// production defaults that test code overrides via struct fields.
//
// Logger is optional; SweepOrphanDaemons substitutes a no-op default at
// entry so call sites can dispatch unconditionally (mirrors the contract
// of MarkerCleanupCore and EagerSignalCore).
type OrphanSweepCore struct {
	// Pgrep enumerates candidate PIDs that match the canonical form
	// `pgrep -fx '^portal state daemon( |$)'`. Production adapter (Task
	// 4-4) wraps os/exec; a missing seam at runtime causes
	// SweepOrphanDaemons to short-circuit at entry — no defaulting here
	// because the production behaviour requires shelling out and the
	// bootstrap package must not import os/exec to avoid pulling
	// process-management surface into a pure-orchestration package.
	Pgrep func() ([]int, error)

	// SaverPanePID returns the pane PID of `_portal-saver` with explicit
	// tri-state semantics:
	//
	//   - (pid, true,  nil)  — `_portal-saver` is present; pid is the
	//     pane PID (typically > 0).
	//   - (0,   false, nil)  — `_portal-saver` is absent. The sweep
	//     proceeds with an empty legitimate set against the full pgrep
	//     result, with no warning emitted.
	//   - (0,   false, err)  — any other failure (generic exec failure,
	//     parse failure). The sweep WARN-logs and proceeds with an empty
	//     legitimate set.
	//
	// "Absent" is encoded at the type level (the `present` bool) rather
	// than overloaded onto pid == 0, so a future implementer returning
	// pid 0 defensively on a non-error path cannot silently flip "absent"
	// into "legitimate empty PID". No default here; production wiring
	// must inject the tmux adapter.
	SaverPanePID func() (pid int, present bool, err error)

	// Identify defaults to state.IdentifyDaemon when nil — the same
	// primitive Component A uses for its kill-barrier identity check.
	// Tests override to stub canned identity outcomes per PID.
	Identify func(pid int) (state.IdentifyResult, error)

	// Kill defaults to a syscall.Kill(pid, SIGKILL) closure when nil.
	// SIGKILL (not SIGTERM) is the spec-mandated signal — orphan view is
	// untrusted; no final flush; same reasoning as Component A.
	Kill func(pid int) error

	// Logger is optional; nil tolerated. SweepOrphanDaemons routes it through
	// the shared internal/log discard sink via log.OrDiscard at entry so call
	// sites can dispatch unconditionally.
	Logger *slog.Logger
}

var _ OrphanSweeper = (*OrphanSweepCore)(nil)

// SweepOrphanDaemons enumerates candidate `portal state daemon` PIDs via the
// Pgrep seam, builds the legitimate set from the SaverPanePID seam, then for
// each non-legitimate candidate identity-checks via Identify and SIGKILLs via
// Kill. Best-effort: every failure path emits Logger.Warn under the
// bootstrap component and is swallowed. The method returns nil
// unconditionally.
//
// Algorithm:
//  1. Route the Logger through log.OrDiscard so a nil sink discards
//     so call sites can dispatch logger.Debug / .Info / .Warn unconditionally.
//  2. Apply production defaults for Identify and Kill when those seams are
//     nil. (Pgrep and SaverPanePID have no defaults — production wiring
//     must supply them; absence indicates a programmer error.)
//  3. Invoke Pgrep. On error: WARN ("sweep: pgrep failed") and return
//     nil — best-effort posture forbids escalation.
//  4. Invoke SaverPanePID. Three observable shapes:
//     - err != nil: WARN ("sweep: list-panes _portal-saver failed,
//     legitimate set empty") and treat the legitimate set as
//     empty; sweep proceeds against ALL pgrep results.
//     - !present (absent): legitimate set stays empty; no warning.
//     - present: insert pid into the legitimate set.
//  5. For each candidate PID that is NOT in the legitimate set AND NOT
//     equal to os.Getpid() (defensive self-skip):
//     a. Invoke Identify. On error: WARN ("sweep: identity-check failed,
//     skipping" with target_pid) and continue to the next PID.
//     b. On result != IdentifyIsPortalDaemon: DEBUG ("sweep: pid not
//     identity-checked as portal daemon, skipping") and continue.
//     c. Invoke Kill. On error: WARN ("sweep: kill failed") on the
//     bootstrap logger and continue. On success: DEBUG ("orphan killed"
//     with target_pid) on cleanLogger and increment the killed counter.
//  6. Emit one INFO cycle summary ("orphan-daemon sweep complete" with
//     killed + took) on cleanLogger (component clean), then return nil. The
//     summary is reached only on the Pgrep-succeeded path; the Pgrep-fail
//     early return at step 3 emits no summary. The SaverPanePID-error path
//     proceeds (empty legitimate set) and reaches the summary normally.
//
// SweepOrphanDaemons never returns a non-nil error — Component B is a best-
// effort step and the orchestrator's Warn-and-swallow site at step 4 would
// surface a returned error as a redundant Warn line. The nil return is the
// canonical "step completed" signal regardless of internal failures.
func (c *OrphanSweepCore) SweepOrphanDaemons() error {
	logger := log.OrDiscard(c.Logger)
	identify := c.Identify
	if identify == nil {
		identify = state.IdentifyDaemon
	}
	kill := c.Kill
	if kill == nil {
		kill = defaultKill
	}

	start := time.Now()
	var killed int

	candidates, err := c.Pgrep()
	if err != nil {
		logger.Warn("sweep: pgrep failed", "error", err)
		return nil
	}

	legitimate := map[int]struct{}{}
	saverPID, saverPresent, saverErr := c.SaverPanePID()
	switch {
	case saverErr != nil:
		logger.Warn("sweep: list-panes _portal-saver failed, legitimate set empty", "error", saverErr)
	case !saverPresent:
		// `_portal-saver` absent — legitimate set stays empty; no warn.
	default:
		legitimate[saverPID] = struct{}{}
	}

	ownPID := os.Getpid()
	for _, pid := range candidates {
		if pid == ownPID {
			// Defensive self-skip — should never appear in pgrep output
			// for `portal state daemon` because the orchestrator is
			// running under a different argv, but the guard is cheap and
			// the consequences of a false positive (killing ourselves) are
			// fatal.
			continue
		}
		if _, isLegit := legitimate[pid]; isLegit {
			continue
		}
		res, err := identify(pid)
		if err != nil {
			logger.Warn("sweep: identity-check failed, skipping", "target_pid", pid, "error", err)
			continue
		}
		if res != state.IdentifyIsPortalDaemon {
			logger.Debug("sweep: pid not identity-checked as portal daemon, skipping", "target_pid", pid)
			continue
		}
		if err := kill(pid); err != nil {
			logger.Warn("sweep: kill failed", "target_pid", pid, "error", err)
			continue
		}
		// Per-kill detail demoted from INFO to DEBUG (per the Cycle-level
		// summary cadence: per-item events at DEBUG, one INFO summary at
		// completion). Emitted on cleanLogger so the clean: prefix groups the
		// sweep's own detail with its clean-component summary below.
		cleanLogger.Debug("orphan killed", "target_pid", pid)
		killed++
	}
	// One INFO cycle summary at completion (component clean). Reached only on
	// the Pgrep-succeeded path — the Pgrep-fail early return above emits no
	// summary. killed counts successful SIGKILLs only (identity-skips and
	// kill-failures are excluded).
	cleanLogger.Info("orphan-daemon sweep complete", "killed", killed, log.Took(start))
	return nil
}
