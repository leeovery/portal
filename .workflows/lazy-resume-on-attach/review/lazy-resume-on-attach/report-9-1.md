TASK: Pending Re-File Never Overwrites an Existing Token-Named Transcript (lazy-resume-on-attach-9-1, tick-79925b)

ACCEPTANCE CRITERIA:
- A waiting pane's first waiting tick finds it displaced (a live pane now sits at the address it was filed under) and is cancelled after that live pane's capture is written and before the commit. The shutdown flush that follows leaves `scrollback/pane-<token>.bin` holding the waiting pane's frozen bytes and the positional file holding the live pane's capture, and commits each pane's record naming its own file.
- The same displaced first tick instead fails its commit. The next tick leaves both files' bytes as the failed tick left them, and commits the waiting pane's record naming the token path and the live pane's naming the positional path.
- `portal state commit-now` runs while the on-disk `sessions.json` still names the waiting pane's positional path, with the token-named file already holding its frozen bytes and the positional file holding another pane's capture. It commits the waiting pane's record naming the token path and changes neither file's bytes. A daemon starting from that same on-disk index does the same on its first tick.
- A re-file that finds a file already named `pane-<token>.bin` points the record at that path, leaves that file's bytes unchanged, and leaves the positional file where it is with its bytes unchanged.
- Two re-files of one pane from indexes that both name its positional path (the daemon's and `commit-now`'s, in either order) leave the token-named file holding the bytes the first of them moved; the second overwrites nothing.
- The rest of the re-file's rules hold as today: with no token-named file present the bytes move onto it, the positional name holds nothing afterwards, and the dedup entry for that name is gone; a missing source, or a record naming no file, adopts the token path; a rename failing for any reason other than a missing source or an existing token-named file leaves the record on its stored path and its dedup entry in place, and emits one WARN carrying only `pane_key`, `path` and `error`.

STATUS: complete

SPEC CONTEXT: Spec section 7.2 (and its 2026-09-21 corrigendum): a frozen (waiting) pane's scrollback leaves the positional namespace for as long as it waits. It is re-filed under `scrollback/pane-<PortalPaneID>.bin`, because the saver's writer derives its path from the live address, so the next pane to take that address would otherwise overwrite the transcript. That transcript has no other copy. This task closes the retry-path version of that loss. A displaced first waiting tick re-files, the intruder writes the vacated positional file, and the tick then fails to commit (cancel then flush, a Commit error, a crash, or a mid-tick commit-now). A later re-file from an index that still names the positional path would then rename the intruder's bytes over the token file. The spec does not state the no-clobber rule. It is a sound review remediation that tightens the spec's stated property rather than diverging from it.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:120-135 (`refilePendingPane`): adopts the token path on success, on a missing source, and on an existing token-named file.
  - internal/state/scrollback.go:143-152 (`placeStoredScrollback`): an empty stored path becomes a missing source. `fs.ErrNotExist` and `fs.ErrExist` both mean adopt. Any other error goes to the single WARN at :130.
  - internal/state/scrollback.go:156-173: `errNoReplaceUnsupported`, the `renameNoReplace` seam, and `moveNoClobber` with its link(2) fallback.
  - internal/state/rename_noreplace_darwin.go:13-22: `renamex_np` with `RENAME_EXCL`; `ENOTSUP` maps to unsupported.
  - internal/state/rename_noreplace_linux.go:13-22: `renameat2` with `RENAME_NOREPLACE`; `EINVAL` maps to unsupported.
- Notes: The check and the move are one syscall (`RENAME_EXCL`/`RENAME_NOREPLACE`), or `link(2)` where the flag is refused. Either way an existing name is refused atomically, as the Do section asks. The code has evolved since the task landed. The old `renameStoredScrollback` became `placeStoredScrollback` with an injected `place` func, so the later skeleton-link path (scrollback.go:242) shares the same adopt rule. `CaptureAndRefile` is now the unexported `captureAndRefile`, reached only through `RunCommitCycle` (internal/state/commit_cycle.go:58) under the commit lock. That is a sound later tightening: the single-entry-point intent holds, and the lock serializes commit-now against the daemon as well. The adopt path also drops the dedup entry for the positional name (scrollback.go:134). The only effect is one redundant rewrite of the live occupant's own capture, which does no harm. The residual the task accepts is adopting a stale, never-reclaimed token file. It needs gc failures across the whole gap between two waits, and even then restores the pane's own earlier bytes.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: cmd/state_daemon_resume_pending_test.go:431-460. A `dispatchHook` cancels on the intruder's `capture-pane`, so the write lands and the next pane's ctx check aborts before the commit. The test asserts `PrevIndex` did not advance and the positional file holds the intruder, then runs `defaultShutdownFlush`, then calls `assertDisplacedFilesKept` (:343-360), which checks both files' bytes and both records.
  - AC2: :462-489. `breakCommitTarget` makes sessions.json a directory, so the commit fails. The test asserts both files, then runs the next tick and calls `assertDisplacedFilesKept`.
  - AC3: commit-now at cmd/state_commit_now_test.go:1259-1281 (production `RunCommitCycle`, on-disk index staged by `stageDisplacedOnDisk`). Daemon cold start at cmd/state_daemon_resume_pending_test.go:491-513 (`ReadIndex` plus `SeedHashMap`).
  - AC4: internal/state/scrollback_test.go:587-610.
  - AC5: internal/state/scrollback_test.go:612-648, both orders.
  - AC6: move at :513-538; missing source at :564-585; record naming no file at :793-813; WARN at :736-775. The WARN case now stages its failure with a read-only scrollback dir (`denyScrollbackWrites`, :494-501) instead of a directory at the token path, as the Do section required. It asserts exactly one WARN and only the `pane_key`, `path` and `error` keys.
  - Fallback path: :650-701 cover the link(2) fallback for both the move and the adopt case, through `StubRenameNoReplaceUnsupported` (internal/state/export_test.go:5-12).
- Notes: With a plain rename, every displacement test would fail: the intruder's bytes would land in the token file. So the tests do pin the fix. No redundancy of note. No new `CommitNowDeps` field and no real-tmux test were added, so the two seam and teardown-guard conditions do not apply.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel`. The package-level seam is restored through `t.Cleanup`. Test exports live in export_test.go. Platform files carry build tags and match the shipped darwin/linux targets.
- SOLID principles: Good. The injected `place` func lets the move path and the link path share one adopt/refuse rule.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments at scrollback.go:88-98, :137-142, :154-155 and :160-162, and in both platform files, hold against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
