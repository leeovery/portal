package cmd

import (
	"fmt"

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
}

// resolveCommitNowDeps returns the production-or-overridden function values
// for one commit-now invocation. Unset fields in commitNowDeps fall through to
// the production implementation independently.
func resolveCommitNowDeps() (
	readIndex func(string) (state.Index, bool, error),
	captureStructure func(state.CaptureClient, map[string]struct{}, *state.Index) (state.Index, error),
	commit func(string, state.Index, bool, *state.Logger) error,
	newClient func() state.CaptureClient,
) {
	readIndex = state.ReadIndex
	captureStructure = state.CaptureStructure
	commit = state.Commit
	newClient = func() state.CaptureClient { return tmux.DefaultClient() }

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
	return
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

		readIndex, captureStructure, commit, newClient := resolveCommitNowDeps()

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
