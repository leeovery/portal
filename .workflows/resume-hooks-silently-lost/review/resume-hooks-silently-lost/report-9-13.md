TASK: resume-hooks-silently-lost-9-13 — Two Production Comment Blocks Carry The Design Argument And Cardinality Claims The Project's Comment Standard Forbids

ACCEPTANCE CRITERIA:
- [x] `snapshotLockBound`'s comment states the three conclusions and no argument; the function body is untouched.
- [x] The `skipReasons` block carries no claim about how many surfaces reach a reason.
- [x] `hookSeams`'s doc comment carries no cardinality aside.
- [x] No non-comment byte changes in the three files; the full unit and integration lanes pass unchanged.

STATUS: complete

SPEC CONTEXT:
This is a phase-9 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context). The standard it is measured against is
`.claude/skills/workflow-implementation-process/references/code-quality.md` §Comments, which forbids two
things by name: cardinality claims ("the single caller", "the only site that…") because ordinary
additive change far from the comment falsifies them, and the design argument ("State the conclusion the
code needs, not the debate; the reasoning lives in the project's design artifacts"). The subject matter
the comments describe — the clean's two-phase lock discipline (advisory shared pre-read at a derived
bound, then the exclusive deletion hold) and the closed stand-down `Reason` vocabulary — is settled
behaviour documented in CLAUDE.md; the task changes only how much of the reasoning behind it lives in
source comments.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/lock.go:29-40` — `snapshotLockBound`'s doc comment reduced from ~33 lines to 9,
    body unchanged (`return max(lockTimeout/snapshotLockFraction, lockPollInterval)`).
  - `internal/hooksweep/reason.go:14-17` and `internal/hooksweep/reason.go:27-34` — the const-block and
    `Reasons` doc comments, stripped of the two cardinality claims.
  - `cmd/hooks.go:81-83` — `hookSeams`'s doc comment, one sentence shorter.
  - Commit `6373a1aa`, three files, +14/−45, every changed line a comment line.
- Notes:
  - **Location divergence, sound.** The plan named the second block as `skipReasons` at
    `cmd/run_hook_stale_cleanup.go:32-52`. That file no longer exists: the immediately preceding task
    (`9-12`, commit `a4898f41`) moved the sweep into `internal/hooksweep` and renamed the symbol to the
    exported `Reasons`. The executor applied the edit at the moved site. Both named claims were removed
    — "its caller-facing line is the repair's own" from the const block (`reason.go:17` now reads
    "A sweep that ran and failed is not among them: it declined nothing.") and the whole `lock-timeout
    cannot reach the read-only diagnosis at all` bullet from the `Reasons` block. The substance was
    delivered at the address the code actually occupies; no loss.
  - The three surviving `snapshotLockBound` conclusions are each present and each verified true against
    the code: the pre-read degrades to an unlocked read paying one DEBUG breadcrumb
    (`internal/hooks/store.go:53-54` routes `loadSnapshot` through `loadSharedBounded`, which emits
    `load-unlocked` at DEBUG on `store.go:73`); the bound is derived from `lockTimeout`
    (`lock.go:39`); and the floor claim holds against `acquireLock`'s loop shape
    (`lock.go:59-73` — flock attempt, then deadline test, then a `lockPollInterval` sleep, so a bound
    under one interval still costs one interval).
  - The retained `Reasons` note ("lock-timeout's not-evaluable phrase exists for vocabulary completeness
    rather than for an observed leak") is consistent with the code: `ReasonLockTimeout` is produced only
    by the sweep (`internal/hooksweep/sweep.go:185`) yet is keyed in both copy tables,
    `skippedPrunePhrases` (`cmd/doctor.go:242`) and `notEvaluableDetails` (`cmd/doctor.go:253`).
  - Nothing later undid the work: the only subsequent commit touching any of the three files is
    `3ccac0d7` (task 9-17, the `tmux.Target` type change), which does not touch these comment blocks.
    All three reduced comments are present at HEAD.

TESTS:
- Status: Adequate (correctly, no test change)
- Coverage: The task is comment text alone, so there is nothing new to observe and no test semantics to
  change. The commit touches zero `_test.go` files, which is exactly what the task's Tests section asks
  for. The pins the task names as needing to stay green with no edit are intact:
  `internal/hooks/read_lock_test.go:360-390` still holds `halfRelationCrossoverBound`,
  `sampledMutationBounds`, `pollIntervalFloor` and `assertHalfRelation`, all reading the bound through
  the unchanged `hooks.SnapshotLockBoundForTest` seam (`internal/hooks/locktest.go:17-24`).
- Notes:
  - I checked for the one way a comment-only edit could break a suite: a test that reads comment text.
    No `_test.go` file under `cmd` or `internal` references `CommentGroup`, `ParseComments` or `.Doc`, so
    no guard can observe these blocks. Grepping the Go sources for the deleted phrasings ("thousandth",
    "repair's line alone", "builder of its own") returns nothing — no test or comment elsewhere refers
    back to the removed text.
  - No build directive or `//go:` line is inside or adjacent to any of the three edited blocks, so the
    edits cannot alter the compiled set in either lane.
  - Test execution is out of my remit; the above is a read-based judgement that the lanes are unchanged,
    which for a comment-only diff is the whole of what can go wrong.

CODE QUALITY:
- Project conventions: Followed. Measured against `code-quality.md` §Comments, each surviving block sits
  in the sanctioned categories: `snapshotLockBound`'s is "why" (why derived rather than declared) plus a
  named trap (the floor, and what goes wrong beneath it); `Reasons`'s is "why" (why a constant exists
  with no path to it from one surface); `hookSeams`'s is an ordinary doc comment saying what the function
  returns. None asserts a count, an exclusivity, or a property of a test.
- SOLID principles: N/A — no structural change.
- Complexity: Low — unchanged; `snapshotLockBound` remains a one-expression function.
- Modern idioms: N/A.
- Readability: Good. The 33-line block over a three-line function was the clearest instance of a comment
  outweighing its code in these packages; at 9 lines the ratio is defensible and the three claims are
  each independently checkable against the code beneath them.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
