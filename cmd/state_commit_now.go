package cmd

import (
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// errCommitNowFailed is the named sentinel returned from commit-now's RunE on
// any failure exit. The tmux hook subprocess has nowhere meaningful to
// surface stderr, so all diagnostics route through state.Logger / portal.log
// under state.ComponentDaemon. Cobra (with SilenceErrors/SilenceUsage
// inherited from rootCmd) prints nothing; main.go detects this sentinel via
// IsSilentExitError so the stderr-suppression contract is compile-time-linked
// across the cmd and main packages rather than relying on the prior
// empty-message string-compare convention. The sentinel exists solely to
// drive a non-zero process exit while preserving the underlying cause via
// errors.Unwrap on the wrapped failure returned from failCommitNow.
var errCommitNowFailed = errors.New("commit-now failed")

// IsSilentExitError reports whether err is one of the cmd-package sentinels
// whose stderr emission must be suppressed at the top-level error handler.
// Both errCommitNowFailed (state commit-now, hook subprocess context) and
// ErrStatusUnhealthy (state status, rendered output already on stdout) drive
// non-zero process exits without printing anything to stderr. main.go calls
// this in place of the legacy err.Error() == "" guard.
func IsSilentExitError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errCommitNowFailed) || errors.Is(err, ErrStatusUnhealthy)
}

// commitNowDeps is the DI seam for the commit-now subcommand. When nil, the
// production implementations (state.ReadIndex / state.CaptureStructure /
// state.Commit / tmux.DefaultClient) are used. Tests assign a fully-populated
// *CommitNowDeps in t.Cleanup to inject mocks for each function and the tmux
// client constructor.
//
// Per the cmd-package DI idiom (mirroring bootstrapDeps / openDeps /
// hooksDeps), a non-nil deps struct does NOT have to populate every field;
// commit-now falls back to the production implementation for each unset
// field independently. This keeps integration-style tests that only need to
// swap one seam (e.g. NewClient) free of boilerplate.
var commitNowDeps *CommitNowDeps

// CommitNowDeps exposes the four collaborators commit-now needs as function
// fields so tests can swap them.
//
//   - ReadIndex defaults to state.ReadIndex — loads the prior sessions.json
//     from disk. A skip=true or err!=nil result triggers the zero-value
//     PrevIndex fallback and a WARN log entry.
//   - CaptureStructure defaults to state.CaptureStructure — queries the live
//     tmux server and produces the structural Index that will be committed.
//     commit-now never passes a non-nil skipSet: this path has no skeleton
//     markers to merge from.
//   - Commit defaults to state.Commit — atomic temp+rename write of
//     sessions.json with anyScrollbackChanged=false (commit-now never writes
//     scrollback bytes; that remains the daemon's exclusive responsibility).
//   - NewClient defaults to a *tmux.Client adapter — production code calls
//     tmux.DefaultClient.
type CommitNowDeps struct {
	ReadIndex        func(dir string) (state.Index, bool, error)
	CaptureStructure func(c state.CaptureClient, skipSet map[string]struct{}, prev *state.Index) (state.Index, error)
	Commit           func(dir string, idx state.Index, anyScrollbackChanged bool, logger *state.Logger) error
	NewClient        func() state.CaptureClient

	// IsRestoring queries the @portal-restoring server option. When the
	// marker is set, commit-now short-circuits as a no-op (the daemon's
	// existing restoration discipline owns sessions.json during bootstrap
	// step 5 / step 4 version-upgrade kills). Defaults to a closure that
	// calls state.IsRestoringSet against a fresh production tmux client.
	IsRestoring func() (bool, error)

	// TouchSaveRequested creates-or-truncates save.requested under dir and
	// bumps its mtime, mirroring the in-line touch state notify performs.
	// Used on the @portal-restoring short-circuit so the daemon's first
	// post-restoration tick commits without waiting for the 30s gap rule.
	// Defaults to state.TouchSaveRequested.
	TouchSaveRequested func(dir string) error
}

// resolveCommitNowDeps returns a fully-populated *CommitNowDeps for one
// commit-now invocation. Unset fields in the package-level commitNowDeps fall
// through to the production implementation independently — same per-field
// nil-check idiom as bootstrapDeps / openDeps / hooksDeps. The returned value
// is never nil and every field is guaranteed non-nil, so RunE can dereference
// fields directly without further nil checks.
func resolveCommitNowDeps() *CommitNowDeps {
	deps := &CommitNowDeps{
		ReadIndex:          state.ReadIndex,
		CaptureStructure:   state.CaptureStructure,
		Commit:             state.Commit,
		NewClient:          func() state.CaptureClient { return tmux.DefaultClient() },
		IsRestoring:        func() (bool, error) { return state.IsRestoringSet(tmux.DefaultClient()) },
		TouchSaveRequested: state.TouchSaveRequested,
	}

	if commitNowDeps == nil {
		return deps
	}
	if commitNowDeps.ReadIndex != nil {
		deps.ReadIndex = commitNowDeps.ReadIndex
	}
	if commitNowDeps.CaptureStructure != nil {
		deps.CaptureStructure = commitNowDeps.CaptureStructure
	}
	if commitNowDeps.Commit != nil {
		deps.Commit = commitNowDeps.Commit
	}
	if commitNowDeps.NewClient != nil {
		deps.NewClient = commitNowDeps.NewClient
	}
	if commitNowDeps.IsRestoring != nil {
		deps.IsRestoring = commitNowDeps.IsRestoring
	}
	if commitNowDeps.TouchSaveRequested != nil {
		deps.TouchSaveRequested = commitNowDeps.TouchSaveRequested
	}
	return deps
}

// stateCommitNowCmd performs a synchronous structural capture-and-commit on
// the live tmux server, rewriting sessions.json atomically. It is invoked
// from the tmux session-closed hook so externally-killed sessions are
// removed from sessions.json before the next bootstrap can resurrect them.
//
// This file implements the bare happy path plus the PrevIndex fallback:
//
//   - PrevIndex is sourced from disk via state.ReadIndex. A missing or
//     corrupt sessions.json falls back to a zero-value PrevIndex and logs
//     WARN under state.ComponentDaemon — a ReadIndex failure is never a
//     commit-now failure exit per the spec.
//   - state.CaptureStructure is invoked with a nil skipSet (commit-now never
//     coordinates with skeleton markers).
//   - state.Commit is invoked with anyScrollbackChanged=false so no .bin
//     files are written and gcOrphanScrollback only runs when the structural
//     delta actually changed.
//   - save.requested is NOT touched on this success path. Successful sync
//     commits are silent to the daemon (see spec § save.requested
//     Discipline).
//
// The @portal-restoring short-circuit and the failure-path save.requested
// touch land in tasks 1-2 and 1-3 respectively and are out of scope here.
//
// Hidden from --help: this command is invoked by tmux hooks, not directly by
// users.
var stateCommitNowCmd = &cobra.Command{
	Use:    "commit-now",
	Short:  "Synchronously commit sessions.json from live tmux state (internal, invoked by tmux hooks)",
	Args:   cobra.NoArgs,
	Hidden: true,
	// Defensive: rootCmd already sets these, but a tmux hook subprocess has
	// nowhere meaningful to send stderr, so we restate here so any future
	// reparenting of the command keeps the silent-failure invariant.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := state.EnsureDir()
		if err != nil {
			// Fatal pre-logger: without a state dir we have nowhere to open
			// portal.log. cobra prints the wrapped error to stderr.
			return fmt.Errorf("ensure state dir: %w", err)
		}

		// Non-rotating logger — only the daemon rotates portal.log. A nil
		// logger here is acceptable; *state.Logger's nil-receiver no-ops every
		// call so a diagnostics open failure never fails commit-now.
		logger, _ := openNoRotateLogger()
		defer func() { _ = logger.Close() }()

		deps := resolveCommitNowDeps()

		// @portal-restoring short-circuit. Mirrors the daemon's tick() entry
		// guard: when bootstrap step 5 Restore (or a step-4 saver
		// version-upgrade firing session-closed mid-restore) is in progress,
		// any structural commit would write a partial skeleton view. Skip
		// every primitive, touch save.requested so the daemon's first
		// post-restoration tick commits, and exit 0 — the skip is a
		// deliberate completion, not an error.
		//
		// A query failure on @portal-restoring is treated symmetrically to
		// (true, nil): if we cannot prove the marker is clear, presume it
		// set and protect the in-flight restore. The cost is a marginally
		// extended resurrection window on rare transient-tmux-query
		// failures, recovered on the daemon's next tick via the
		// save.requested touch. The risk priority — protect in-flight
		// restore over "kill removes the session promptly" — matches the
		// spec's @portal-restoring Defence section.
		restoring, err := deps.IsRestoring()
		switch {
		case err != nil:
			logger.Warn(state.ComponentDaemon, "isRestoring query failed; presuming @portal-restoring marker set to protect in-flight restore: %v", err)
			touchAfterShortCircuit(logger, dir, deps.TouchSaveRequested)
			return nil
		case restoring:
			logger.Info(state.ComponentDaemon, "commit-now skipped: @portal-restoring set")
			touchAfterShortCircuit(logger, dir, deps.TouchSaveRequested)
			return nil
		}

		prev := loadPrevIndex(dir, deps.ReadIndex, logger)

		client := deps.NewClient()
		idx, err := deps.CaptureStructure(client, nil, &prev)
		if err != nil {
			return failCommitNow(logger, dir, deps.TouchSaveRequested, "capture structure", err)
		}

		if err := deps.Commit(dir, idx, false, logger); err != nil {
			return failCommitNow(logger, dir, deps.TouchSaveRequested, "commit sessions.json", err)
		}

		return nil
	},
}

// touchAfterShortCircuit performs the best-effort save.requested touch shared
// by both @portal-restoring short-circuit branches — (true, nil) and the
// query-error "presume set" branch. A touch failure is logged at WARN under
// state.ComponentDaemon and swallowed; the short-circuit's exit-0 status
// dominates per spec § save.requested Touch Failure Handling.
func touchAfterShortCircuit(logger *state.Logger, dir string, touch func(string) error) {
	if terr := touch(dir); terr != nil {
		logger.Warn(state.ComponentDaemon, "touch save.requested during short-circuit: %v", terr)
	}
}

// failCommitNow is the shared failure-exit path for commit-now's structural
// primitives. Per spec § commit-now Failure Behaviour and § save.requested
// Touch Failure Handling:
//
//  1. Log the primary failure at ERROR under state.ComponentDaemon so the
//     daemon-driven and hook-driven capture log streams remain unified.
//  2. Best-effort touch save.requested so the daemon's next scheduled tick
//     (within 1s when it is alive) commits — bounded fallback recovery for the
//     resurrection window. Touch errors are logged at WARN and never
//     propagated; the original failure dominates.
//  3. Return an error that wraps errCommitNowFailed via fmt.Errorf("%w: %s:
//     %v", ...). errors.Is(err, errCommitNowFailed) drives main.go's silent-
//     exit suppression (see IsSilentExitError). The cause is preserved as
//     interpolated text in the error message only — errors.Unwrap surfaces
//     errCommitNowFailed (the sole %w arg), not the cause. portal.log is
//     the authoritative diagnostic sink. Cobra (SilenceErrors=true) is
//     responsible for not printing the error; main.go's IsSilentExitError
//     guard prevents the top-level handler from duplicating it. The hook
//     subprocess has nowhere meaningful to send stderr; user-facing
//     diagnostics route exclusively through portal.log.
func failCommitNow(logger *state.Logger, dir string, touch func(string) error, stage string, cause error) error {
	logger.Error(state.ComponentDaemon, "%s: %v", stage, cause)
	if terr := touch(dir); terr != nil {
		logger.Warn(state.ComponentDaemon, "touch save.requested after %s failure: %v", stage, terr)
	}
	return fmt.Errorf("%w: %s: %v", errCommitNowFailed, stage, cause)
}

// loadPrevIndex returns the prior on-disk Index for use as CaptureStructure's
// prev argument. Both "missing sessions.json" (the clean ENOENT skip) and
// "exists-but-unusable" (read or decode failure) map to a zero-value Index;
// either case is logged at WARN under state.ComponentDaemon so the first-ever
// invocation on a fresh install and a partial-write corruption are both
// visible in portal.log without aborting the synchronous commit.
//
// commit-now never treats a ReadIndex failure as a fatal exit — the primary
// goal of the synchronous path is to drop killed sessions from sessions.json,
// and that goal is satisfied independent of PrevIndex availability. The
// daemon's next tick will repopulate any per-pane content fields the fresh
// capture cannot regenerate on its own.
func loadPrevIndex(dir string, readIndex func(string) (state.Index, bool, error), logger *state.Logger) state.Index {
	idx, skip, err := readIndex(dir)
	if err != nil {
		logger.Warn(state.ComponentDaemon, "read sessions.json: %v; proceeding with zero-value PrevIndex", err)
		return state.Index{}
	}
	if skip {
		logger.Warn(state.ComponentDaemon, "sessions.json absent; proceeding with zero-value PrevIndex")
		return state.Index{}
	}
	return idx
}

func init() {
	stateCmd.AddCommand(stateCommitNowCmd)
}
