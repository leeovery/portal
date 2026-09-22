package cmd

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// errCommitNowFailed drives a non-zero exit while keeping stderr silent: the
// tmux hook subprocess has nowhere meaningful to surface it, so diagnostics go
// to portal.log instead.
var errCommitNowFailed = errors.New("commit-now failed")

// IsSilentExitError reports whether the top-level handler must suppress err's
// stderr emission.
func IsSilentExitError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errCommitNowFailed) ||
		errors.Is(err, ErrDoctorUnhealthy)
}

// A non-nil commitNowDeps need not populate every field: each unset field falls
// back to its production implementation.
var commitNowDeps *CommitNowDeps

// CaptureAndRefile is always called with a nil skipSet and Commit with
// anyScrollbackChanged=false - commit-now writes no scrollback bytes.
type CommitNowDeps struct {
	ReadIndex        func(dir string) (state.Index, bool, error)
	CaptureAndRefile func(c state.CaptureClient, dir string, skipSet map[string]struct{}, prev *state.Index, hm state.HashMap, logger *slog.Logger) (state.Index, map[string]struct{}, error)
	Commit           func(dir string, idx state.Index, anyScrollbackChanged bool, logger *slog.Logger) error
	NewClient        func() state.CaptureClient

	// When @portal-restoring is set, commit-now short-circuits as a no-op: the
	// daemon owns sessions.json during restoration.
	IsRestoring func() (bool, error)

	// TouchSaveRequested is called on the short-circuit so the daemon's first
	// post-restoration tick commits without waiting out the 30s gap rule.
	TouchSaveRequested func(dir string) error
}

// resolveCommitNowDeps returns the seams commit-now runs against: whatever a
// test injected, with the production default filled in for each seam it left
// unset. A new seam costs one fill line here; leaving it out is a nil
// dereference at first use rather than an injection the command silently
// ignores.
func resolveCommitNowDeps() *CommitNowDeps {
	deps := &CommitNowDeps{}
	if commitNowDeps != nil {
		*deps = *commitNowDeps
	}

	if deps.ReadIndex == nil {
		deps.ReadIndex = state.ReadIndex
	}
	if deps.CaptureAndRefile == nil {
		deps.CaptureAndRefile = state.CaptureAndRefile
	}
	if deps.Commit == nil {
		deps.Commit = state.Commit
	}
	if deps.NewClient == nil {
		deps.NewClient = func() state.CaptureClient { return tmux.DefaultClient() }
	}
	if deps.IsRestoring == nil {
		deps.IsRestoring = func() (bool, error) { return state.IsRestoringSet(tmux.DefaultClient()) }
	}
	if deps.TouchSaveRequested == nil {
		deps.TouchSaveRequested = state.TouchSaveRequested
	}

	return deps
}

// Invoked by the tmux session-closed hook, so externally-killed sessions leave
// sessions.json before the next bootstrap can resurrect them.
var stateCommitNowCmd = &cobra.Command{
	Use:    "commit-now",
	Short:  "Synchronously commit sessions.json from live tmux state (internal, invoked by tmux hooks)",
	Args:   cobra.NoArgs,
	Hidden: true,
	// Defensive restatement of the rootCmd settings: a tmux hook subprocess has
	// nowhere meaningful to send stderr, so reparenting must not lose this.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := state.EnsureDir()
		if err != nil {
			// Pre-logger: without a state dir there is nowhere to open portal.log.
			return fmt.Errorf("ensure state dir: %w", err)
		}

		logger := daemonLogger

		deps := resolveCommitNowDeps()

		// Standing down costs a longer resurrection window, recovered on the
		// next tick by the touch below, so a failed read is reported as the
		// anomaly it is while the skip itself stays routine.
		restoring, err := state.RestoreWindowActive(deps.IsRestoring())
		if restoring {
			if err != nil {
				logger.Warn("isRestoring query failed; presuming @portal-restoring set to protect in-flight restore", "error", err)
			} else {
				logger.Info("commit-now skipped: @portal-restoring set")
			}
			touchAfterShortCircuit(logger, dir, deps.TouchSaveRequested)
			return nil
		}

		prev := loadPrevIndex(dir, deps.ReadIndex, logger)

		client := deps.NewClient()
		idx, _, err := deps.CaptureAndRefile(client, dir, nil, &prev, nil, logger)
		if err != nil {
			return failCommitNow(logger, dir, deps.TouchSaveRequested, "capture structure", err)
		}

		if err := deps.Commit(dir, idx, false, logger); err != nil {
			return failCommitNow(logger, dir, deps.TouchSaveRequested, "commit sessions.json", err)
		}

		return nil
	},
}

// A touch failure is logged and swallowed; the exit-0 status dominates.
func touchAfterShortCircuit(logger *slog.Logger, dir string, touch func(string) error) {
	if terr := touch(dir); terr != nil {
		logger.Warn("touch save.requested during short-circuit failed", "error", terr)
	}
}

// The touch is what makes the daemon's next tick retry. Only the sentinel is
// wrapped: the cause survives as interpolated text, since portal.log is the
// authoritative diagnostic sink and stderr stays silent.
func failCommitNow(logger *slog.Logger, dir string, touch func(string) error, stage string, cause error) error {
	logger.Error(stage+" failed", "error", cause)
	if terr := touch(dir); terr != nil {
		logger.Warn("touch save.requested after commit-now failure failed", "error", terr)
	}
	return fmt.Errorf("%w: %s: %v", errCommitNowFailed, stage, cause)
}

// An absent or unusable sessions.json yields a zero-value Index with a WARN:
// dropping killed sessions does not depend on PrevIndex, so a read failure must
// never abort the synchronous commit.
func loadPrevIndex(dir string, readIndex func(string) (state.Index, bool, error), logger *slog.Logger) state.Index {
	idx, skip, err := readIndex(dir)
	if err != nil {
		logger.Warn("read sessions.json failed; proceeding with zero-value PrevIndex", "error", err)
		return state.Index{}
	}
	if skip {
		logger.Warn("sessions.json absent; proceeding with zero-value PrevIndex")
		return state.Index{}
	}
	return idx
}

func init() {
	stateCmd.AddCommand(stateCommitNowCmd)
}
