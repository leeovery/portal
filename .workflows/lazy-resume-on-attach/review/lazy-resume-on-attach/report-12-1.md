TASK: A Waiting Pane's Transcript Survives Two Commits That Overlap (lazy-resume-on-attach-12-1, tick-31beed)

ACCEPTANCE CRITERIA:
(Pane X has a lazy registration and a token, was saved at scrollback/K.bin while not waiting, and is restored at its saved address.)
1. A tick captures while X is skeleton-marked; during its dump X is marked pending and a commit-now starts. The commit-now takes no capture until the tick has committed and finished housekeeping, then re-files K.bin onto pane-<token>.bin and commits: the file holds X's transcript and sessions.json names it.
2. The daemon's next tick, whose in-memory previous index still names K.bin, commits X on pane-<token>.bin and the file survives that tick's housekeeping.
3. Mirror order: a tick re-filing X holds the cycle when a commit-now starts; the commit-now captures only after the tick's housekeeping, sees X pending, and leaves pane-<token>.bin on disk and named.
4. Two back-to-back commit-nows across X's pending mark run one after the other; the second takes the first's committed sessions.json as its previous index; pane-<token>.bin survives both passes.
5. Lock held past the bound: a tick logs its existing `tick failed` WARN having captured, moved, written and deleted nothing; save.requested stays; a later tick runs the cycle. A shutdown flush logs `final flush failed` and `shutdown ... flush_completed=false`.
6. Lock held past the bound: commit-now exits non-zero, stderr silent, ERROR in portal.log, save.requested touched, nothing captured, moved, written or deleted.
7. A committer killed while holding the lock does not hold up the next one.
8. With no other committer, the tick, the shutdown flush and commit-now commit what they commit today; the no-clobber re-file and the skeleton-stage link are unchanged and their tests stay green.

STATUS: issues_found

SPEC CONTEXT: The spec says a waiting (frozen) pane keeps its original content through every reboot it waits through. Its bytes are re-filed under a token-derived name (`scrollback/pane-<token>.bin`) for the length of the wait. The housekeeping pass reclaims whatever the committed index does not name. The spec covers the whole restore-to-wait hand-over (skeleton, then pending). Both committers (the saver's tick and `portal state commit-now`) read the markers themselves. This task closes a deletion route between two committers whose captures and collections interleave.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/commit_cycle.go:14-28: the `commit.lock` sidecar, `ErrCommitLockHeld`, the 5s bound and the 5ms poll.
  - internal/state/commit_cycle.go:51-72: `RunCommitCycle`. The lock is taken before `LoadPrev` and held through `captureAndRefile`, the caller's `Dump` and `Commit` (and so `gcOrphanScrollback`). It is released by the deferred Close.
  - internal/state/commit_cycle.go:76-98: the bounded LOCK_EX|LOCK_NB poll.
  - cmd/state_daemon.go:245-282: `captureAndCommit` enters through `RunCommitCycle`, with `scrollbackDump.run` as the dump. It is reached from `tick` (:200) and `defaultShutdownFlush` (:384).
  - cmd/state_commit_now.go:112-125: commit-now enters with no dump. Its `LoadPrev` closure reads sessions.json under the lock. A lock timeout leaves through `failCommitNow` (:141-147), which touches save.requested.
  - internal/restore/lazy_resume_panel_integration_test.go:412 and internal/restore/lazy_resume_renumbered_restore_integration_test.go:170: both fixtures now enter through the entry point.
- Notes:
  - I enumerated the production call sites. The only callers of `RunCommitCycle` are the daemon (cmd/state_daemon.go:257) and commit-now (cmd/state_commit_now.go:61, :114). No production code outside internal/state calls `Commit` or the unexported `captureAndRefile`.
  - The tick's timeout route now removes save.requested before the cycle and re-touches it on failure (cmd/state_daemon.go:196-205). A later task (12-3) made this change deliberately. It meets the criterion that the flag survives for the next tick.
  - The shutdown path is unchanged apart from the entry point.
  - The no-clobber re-file (`refilePendingPane` / `placeStoredScrollback` / `moveNoClobber`) and the skeleton-stage link (`linkMovedSkeletonScrollback`) are untouched by this task's commit.
  - The adopt-on-missing-source path (internal/state/scrollback.go:143-152) is what makes the daemon's lagging in-memory PrevIndex safe (criterion 2).
  - `SetCommitLockTimeoutForTest` (internal/state/commitlocktest.go) follows the established `internal/hooks/locktest.go` pattern.

TESTS:
- Status: Adequate (two test-quality defects below)
- Coverage:
  - `TestRunCommitCycleSerialisesOverlappingCommitters` (internal/state/commit_cycle_test.go:238-355) covers criteria 1–4 against a shared fake world that flips X from skeleton to pending. It asserts that there are zero tmux reads while the other committer holds the cycle, and that the transcript is filed under the token and named in sessions.json. The criterion-2 subtest drives the next tick with the older in-memory index.
  - `TestRunCommitCycleHoldsTheLockThroughTheHousekeepingPass` (:659-685) probes the lock at the moment of the gc WARN, which pins the hold's span to the end of the pass.
  - `TestRunCommitCycleLockBound` (:395-462) covers the timeout and the release inside the bound.
  - `TestRunCommitCycleAfterAKilledHolder` (:488-530) SIGKILLs a re-exec'd holder process and asserts the next acquire is granted in well under the bound (criterion 7).
  - cmd/state_commit_cycle_lock_test.go:116-219 covers criteria 5 and 6 through the real `tick`, `defaultShutdownFlush` and commit-now RunE: WARN/ERROR lines, flush_completed=false, save.requested, no capture reads, sessions.json and scrollback set unchanged, and a post-release tick that commits.
  - The existing commit-now and deps-convention suites were converted to the new seam.
- Notes:
  - Criterion 4 is exercised against the test's own `commitNowCycle` `LoadPrev`, not commit-now's closure. That is safe: a lagging previous index is harmless for the reason given under IMPLEMENTATION, so no failure follows from it.
  - Two defects are listed under FINDINGS. One test names a behaviour it cannot observe. One synchronisation point is one call early.

CODE QUALITY:
- Project conventions: Followed. The seam is merged via `resolveCommitNowDeps`. The test-only setter follows the locktest.go precedent. There is no `t.Parallel`.
- SOLID principles: Good. One entry point with a single variable part (the dump).
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. Comments hold true against the code, including that `LoadPrev` is called only under the lock and the error-wrapping claims in `RunCommitCycle`'s doc.
- Issues: None beyond the test findings.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/state/commit_cycle_test.go:533 — the subtest "it commits the capture and reports the dump's change so an unchanged structure still writes" never presents an unchanged structure.
  - How it breaks: line 537 lists `closed` as live, but `worldClient.ListAllPanesWithFormat` (:90-98) returns a pane row for `work` only. The captured `closed` session is therefore kept with no windows (internal/state/capture.go:90-94). The seed's `closed` record has one window (commit_cycle_test.go:113-120). `Commit`'s structural check (internal/state/commit.go:31) sees a change and writes whatever `changed` it is handed.
  - Fix: first commit one cycle so the on-disk index equals the capture (default world, `work` only). Then run a second cycle with `Dump` returning true and assert sessions.json was rewritten. Optionally add a control cycle with `Dump` returning false that leaves the bytes identical.
  - FAILS: if `RunCommitCycle` stopped forwarding the dump's result to `Commit` (e.g. passed `false`), this test would still pass. No other test covers that forwarding: the commit-now suite uses a fake that re-implements the cycle, and no daemon test isolates a scrollback-only change. The daemon would then silently stop rewriting sessions.json (and running the housekeeping pass) on ticks where only scrollback changed.
- [in-scope] [contained] internal/state/commit_cycle_test.go:325 — `waitForCalls(t, firstClient, 3)` returns as soon as the third read is entered, one call earlier than the point the test needs.
  - How it breaks: `ListAllPanesWithFormat` increments `calls` at :91 before it reads the pending flag at :92. `world.markPending()` at :327 can land between those two lines.
  - Fix: wait for 4 calls. The fourth is the `ShowEnvironment` entry (:100-101), which is reached only after the pane rows were read and is where the capture blocks.
  - FAILS: when the race hits, the first commit-now captures X as pending while its skeleton markers (read at call 1) still name it. It then re-files K.bin onto the token and commits that, so the second commit-now's previous index names the token file. The assertion at :350 then fails spuriously. The window is narrow but real, and it widens under the machine load the project already identifies as a flake source.

UNSETTLED:
- "The no-clobber re-file and the skeleton-stage link are unchanged, and their existing tests stay green." Reading settles "unchanged": this task's commit touches neither function. Whether their tests, the converted commit-now/daemon suites and the two converted integration fixtures (`captureRound`, `commitNowRound`) pass needs a run of the unit lane for internal/state, cmd and internal/restore, plus `go test -tags integration -p 1 ./internal/restore`.
