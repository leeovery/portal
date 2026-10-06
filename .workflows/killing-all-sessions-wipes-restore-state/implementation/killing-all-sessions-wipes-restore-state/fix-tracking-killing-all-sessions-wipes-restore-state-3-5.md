## Attempt 1

ISSUES:
- The hold is lost when the daemon's previous index predates the commit that filed the pane under its token. Sites:
  - `internal/state/capture.go:365-387` (`keepAnsweredTranscripts`) keeps the token path only when `prev`'s record for the token names it.
  - The daemon's `prev` is `deps.PrevIndex` (`cmd/state_daemon.go:271`), which changes only on the daemon's own successful commit (`:283`).
  - `commit-now` reads `sessions.json` (`cmd/state_commit_now.go:119`) and commits the waiting pane's re-file itself: `captureAndRefile` → `refilePendingScrollback`, then housekeeping removes the positional name. Nothing tells the daemon.

  Reachable sequence: X goes waiting → a `session-closed` `commit-now` files X under `pane-T.bin` and housekeeping removes the positional name → the user answers X → the daemon's next dumping cycle runs, either its tick (≤30s, sooner on notify) or the reboot's shutdown flush. All of this falls inside one daemon commit gap. In that cycle `prev` names X positional, so:
  - `keepAnsweredTranscripts` does not match;
  - `heldTranscripts` leaves X out;
  - `Write` judges an empty capture at the missing positional file, so it sends no confirmation and writes the empty bytes. If instead the capture is refused, nothing is written.

  Either way the commit names the positional path (empty or missing) and `gcOrphanScrollback` deletes `pane-T.bin`. That is AC1's and AC2's loss for "the daemon's tick and its shutdown flush alike". It also breaks AC5's "never names the pane's positional file while that file is missing". The waiting-side re-file is immune to this lag because it derives the token path from the live token. The answered side has no live marker, so it needs a signal that does not come from the daemon's in-memory index.

  FIX: Extend the hold with a rule that reads the disk. In `captureAndRefile` (`internal/state/scrollback.go`, after `refilePendingScrollback` at :369, where `dir` is in reach), point at its token-named file every pane that meets all of these:
  - the dump may write it (not in Skeleton, Pending or Carried);
  - its token is one `PendingScrollbackPath` accepts;
  - its record names a file that is not on disk;
  - its token-named file is on disk.

  This produces the same record state `heldTranscripts` already reads, so the writer's confirmation against the token file and `fileAtPositional` work unchanged. Keep `keepAnsweredTranscripts` for the case where a positional name still exists (a cycle that wrote it and ended uncommitted).

  Add a test in `internal/state/commit_cycle_answered_test.go`: seed disk with X under its token and no positional file, then run a tick whose `LoadPrev` returns an index naming X at its positional path (the daemon's view from before the `commit-now`). Run it once with an unconfirmed empty capture and once with `dumpsNothing`. Assert that `sessions.json` names the token file and that the file is present with its bytes. Add a daemon-level case in `cmd/state_daemon_empty_capture_test.go` with `deps.PrevIndex` left at the pre-wait index while disk names the token path.

  ALTERNATIVE: Have the daemon's `LoadPrev` (`cmd/state_daemon.go:271`) read `sessions.json` under the lock as `commit-now` does, falling back to `deps.PrevIndex` when the read skips or fails. That makes every committer's prev the last committed index, which matches §2.4 exactly. The cost: it changes the prev every daemon merge reads (skeleton, frozen, carry), adds a decode per commit, and moves the fix out of `internal/state`, where the task places the guarantee. I recommend the disk-based rule.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- internal/state/capture.go:359 — this comment carries the old rename model: the positional name is removed by the housekeeping of the first commit naming the token path, not when the pane goes waiting. It is also false for a pane `linkMovedSkeletonScrollback` filed under its token, which this function also holds once its skeleton marker clears.
  OLD: // keepAnsweredTranscripts points each pane neither skipped nor waiting whose
// previous record, matched on its token, names that token's re-filed
// transcript back at it: the pane's positional file was removed when it went
// waiting, so a record naming it would leave the pane's bytes for housekeeping
// to delete. The commit cycle moves the record back to its positional file once
// a dump has written the pane's new capture there.
  NEW: // keepAnsweredTranscripts points each pane neither skipped nor waiting whose
// previous record, matched on its token, names that token's re-filed
// transcript back at it: the positional file a fresh record names may hold
// none of the pane's bytes, and housekeeping would delete the file that does.
// The commit cycle moves the record back to its positional file once a dump
// has written the pane's new capture there.
- internal/state/commit_cycle.go:58 — "filed there while the pane waited" is false for a skeleton-moved pane that never waited, which `heldTranscripts` also admits.
  OLD: // held maps a pane key to the token-named transcript its record still
	// names, filed there while the pane waited.
  NEW: // held maps a pane key to the token-named transcript its record still
	// names.

NOTES:
- The hold also applies to a pane `linkMovedSkeletonScrollback` filed under its token, once its skeleton marker clears without the pane waiting (eager or no hook). Its empty capture is judged against the token file, and its record stays there until a dump writes it. This follows §2.4's "the file its last committed record names" and is safer than today's fresh positional record. AC6's "never waited" reads as ordinary panes, so I don't count this as drift.
- After the fix, the `RunCommitCycle` doc's claim (`internal/state/commit_cycle.go:97-99`) that "no commit leaves the pane's record off the file holding its bytes" holds. Today the ISSUE above falsifies it; the remedy is code, so it is not listed as a correction.
- I did not run the integration lane. I ran the unit lane for `internal/state` and `cmd/...` (all pass), `go vet`, gofmt, and `golangci-lint` on the changed packages (0 issues).
- The existing tests the task names (`cmd/state_daemon_resume_pending_test.go` answered-pane cases, `internal/state/empty_capture_test.go`, `cmd/state_daemon_empty_capture_test.go`) are unchanged apart from the appended daemon case, and all pass.
- Nothing arrived beyond the enumerated inputs.
