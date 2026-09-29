TASK: lazy-resume-on-attach-12-4 (tick-0937ee) — Corrections: CLAUDE.md's `state` row names `RunCommitCycle` and the commit lock

ACCEPTANCE CRITERIA:
- CLAUDE.md's `state` row no longer says the daemon, `state commit-now` and the lazy-panel integration fixture enter through `CaptureAndRefile`; it says no committer calls `CaptureAndRefile` directly and every committing cycle runs through `RunCommitCycle`
- The row names `commit.lock` (`CommitLock`) as distinct from `daemon.lock` and never unlinked, held from the skeleton-marker read to the end of the housekeeping pass, with `LoadPrev` called only under it and a 5s bounded acquire whose timeout returns `ErrCommitLockHeld` with nothing read or written
- The row's timeout clause describes the tick as task 1 leaves it: `save.requested` consumed before the cycle and re-touched when the cycle fails, and `commit-now`'s touch on its own timeout surviving the tick that holds the lock
- Every other sentence of the `state` row, and the rest of CLAUDE.md, is byte-unchanged; no Go source or test changes

STATUS: complete

SPEC CONTEXT: The specification does not describe the commit lock; it is an implementation mechanism that phase 12 added so that concurrent committers (the daemon tick, the shutdown flush, `state commit-now`) cannot interleave capture, re-file and housekeeping. If they did, one committer's housekeeping could delete a waiting pane's token-named transcript that another had just filed. This task corrects the agent-facing guidance in CLAUDE.md so that nobody adding a new committer would bypass the lock.

IMPLEMENTATION:
- Status: Implemented
- Location: CLAUDE.md:61 (the `state` row), commit 9e07d58b7
- Notes:
  - I checked the commit mechanically. The prescribed OLD passage appeared exactly once in the parent revision, and applying the prescribed OLD-to-NEW replacement to the parent's CLAUDE.md reproduces the committed file byte for byte. The commit touches only CLAUDE.md (1 insertion, 1 deletion), so the fourth criterion holds.
  - It was applied after the tick change it describes: 12-3 (4a7f4a1a5, "the tick consumes save.requested before its cycle") lands before 9e07d58b7.
  - Every claim in the passage holds against the current code:
    - `RunCommitCycle` (internal/state/commit_cycle.go:51-72) takes the lock before `LoadPrev` and `captureAndRefile` (line 58). It then runs the optional `Dump` and calls `Commit` (line 68), which runs `gcOrphanScrollback` (internal/state/commit.go:39), and releases the lock only through the deferred Close (line 56).
    - `CommitLock` (commit_cycle.go:28) names `commit.lock`, which is separate from `daemon.lock`. Nothing unlinks it.
    - The acquire bound is `commitLockTimeout = 5 * time.Second` (line 22). A timeout returns `ErrCommitLockHeld` wrapped (line 94) before anything is loaded.
    - The daemon tick removes `save.requested` before `captureAndCommit` (cmd/state_daemon.go:196) and re-touches it on failure (lines 200-205).
    - `commit-now` routes any `RunCommitCycle` failure through `failCommitNow`, which touches the flag (cmd/state_commit_now.go:114-125). It passes `LoadPrev` as a disk read taken under the lock and no `Dump`.
    - The shutdown flush reaches the same cycle (cmd/state_daemon.go:384). Both lazy-panel fixtures enter through `state.RunCommitCycle` (internal/restore/lazy_resume_panel_integration_test.go:412, internal/restore/lazy_resume_renumbered_restore_integration_test.go:170).
  - Later work changed the passage on purpose. Task 13-1 (5071f5d81) unexported `captureAndRefile`, added `internal/state/commit_guard_test.go`, and rewrote the passage to match. It changed "through `CaptureAndRefile`" to "through `captureAndRefile`", and replaced "Calling `CaptureAndRefile` and `Commit` directly reopens the interleaving." with the sentence "Code outside `internal/state` cannot commit around the lock: …". This is the code moving on, and the text still matches the code. The substance this task delivered (`RunCommitCycle` as the sole entry, the lock, its span, its bound and the timeout recovery) is intact at CLAUDE.md:61 and accurate.

TESTS:
- Status: Adequate
- Coverage: This is a documentation-only task, and the criteria require no Go source or test changes. The mechanisms the prose describes are already covered elsewhere: internal/state/commit_cycle_test.go (serialisation and the lock bound), and cmd/state_commit_now_test.go and cmd/state_daemon_save_request_test.go (the touch and consume behaviour).
- Notes: None

CODE QUALITY:
- Project conventions: Followed. The text refers to no task, phase or spec section, and it names symbols that exist at the paths it gives.
- SOLID principles: Good (N/A for prose)
- Complexity: Low
- Modern idioms: Yes (N/A)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
