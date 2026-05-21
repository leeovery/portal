package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

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
	// Defaults to defaultTouchSaveRequested.
	TouchSaveRequested func(dir string) error
}

// resolveCommitNowDeps returns the production-or-overridden function values
// for one commit-now invocation. Unset fields in commitNowDeps fall through to
// the production implementation independently.
func resolveCommitNowDeps() (
	readIndex func(string) (state.Index, bool, error),
	captureStructure func(state.CaptureClient, map[string]struct{}, *state.Index) (state.Index, error),
	commit func(string, state.Index, bool, *state.Logger) error,
	newClient func() state.CaptureClient,
	isRestoring func() (bool, error),
	touchSaveRequested func(string) error,
) {
	readIndex = state.ReadIndex
	captureStructure = state.CaptureStructure
	commit = state.Commit
	newClient = func() state.CaptureClient { return tmux.DefaultClient() }
	isRestoring = func() (bool, error) { return state.IsRestoringSet(tmux.DefaultClient()) }
	touchSaveRequested = defaultTouchSaveRequested

	if commitNowDeps == nil {
		return
	}
	if commitNowDeps.ReadIndex != nil {
		readIndex = commitNowDeps.ReadIndex
	}
	if commitNowDeps.CaptureStructure != nil {
		captureStructure = commitNowDeps.CaptureStructure
	}
	if commitNowDeps.Commit != nil {
		commit = commitNowDeps.Commit
	}
	if commitNowDeps.NewClient != nil {
		newClient = commitNowDeps.NewClient
	}
	if commitNowDeps.IsRestoring != nil {
		isRestoring = commitNowDeps.IsRestoring
	}
	if commitNowDeps.TouchSaveRequested != nil {
		touchSaveRequested = commitNowDeps.TouchSaveRequested
	}
	return
}

// defaultTouchSaveRequested creates-or-truncates save.requested under dir and
// bumps its mtime. Mirrors the byte-for-byte sequence state notify performs
// in-line; kept package-local so commit-now's short-circuit can reuse it
// without depending on cobra wiring.
//
// The Chtimes call is best-effort: a save.requested that exists but failed an
// mtime bump still satisfies the daemon's dirty-flag check on the next tick.
func defaultTouchSaveRequested(dir string) error {
	path := state.SaveRequested(dir)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("touch save.requested: %w", err)
	}
	_ = f.Close()
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return nil
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

		readIndex, captureStructure, commit, newClient, isRestoring, touchSaveRequested := resolveCommitNowDeps()

		// @portal-restoring short-circuit. Mirrors the daemon's tick() entry
		// guard: when bootstrap step 5 Restore (or a step-4 saver
		// version-upgrade firing session-closed mid-restore) is in progress,
		// any structural commit would write a partial skeleton view. Skip
		// every primitive, touch save.requested so the daemon's first
		// post-restoration tick commits, and exit 0 — the skip is a
		// deliberate completion, not an error.
		//
		// A query failure on @portal-restoring is out of scope here; per
		// task 1-2 the production primitive deterministically returns either
		// (false, nil) or (true, nil), with task 1-3 owning the
		// tmux-unreachable branch.
		restoring, err := isRestoring()
		if err == nil && restoring {
			logger.Info(state.ComponentDaemon, "commit-now skipped: @portal-restoring set")
			if terr := touchSaveRequested(dir); terr != nil {
				logger.Warn(state.ComponentDaemon, "touch save.requested during short-circuit: %v", terr)
			}
			return nil
		}

		prev := loadPrevIndex(dir, readIndex, logger)

		client := newClient()
		idx, err := captureStructure(client, nil, &prev)
		if err != nil {
			logger.Warn(state.ComponentDaemon, "capture structure: %v", err)
			return fmt.Errorf("capture structure: %w", err)
		}

		if err := commit(dir, idx, false, logger); err != nil {
			logger.Warn(state.ComponentDaemon, "commit sessions.json: %v", err)
			return fmt.Errorf("commit sessions.json: %w", err)
		}

		return nil
	},
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
