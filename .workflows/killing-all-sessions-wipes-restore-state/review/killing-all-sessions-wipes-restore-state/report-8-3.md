TASK: Corrections (killing-all-sessions-wipes-restore-state-8-3, tick-a9f564). Fix the CLAUDE.md `state` row sentence that still credits `state commit-now`'s fallback with the unreadable-sessions.json WARN.

ACCEPTANCE CRITERIA:
- The `state` row (`CLAUDE.md:61`) holds the replacement text verbatim where the old sentence stood, between "The daemon's tick and shutdown flush dump, passing the daemon's in-memory index as that fallback." and "Those callers and the lazy-panel integration fixtures all enter through `RunCommitCycle`."
- `rg -F 'zero-value fallback with a WARN' CLAUDE.md` finds nothing
- No other text in CLAUDE.md changes, and no Go source or test file changes

STATUS: complete

SPEC CONTEXT: The bugfix moved reporting of an unreadable or absent sessions.json into the shared commit cycle (`RunCommitCycle`). The cycle logs that WARN once, under the committer's logger, before falling back to the caller's `LoadPrev`. Committers no longer log it in their own fallbacks. CLAUDE.md is the agent-facing architecture record and has to match this. Otherwise an agent could add a duplicate WARN to commit-now's fallback.

IMPLEMENTATION:
- Status: Implemented
- Location: CLAUDE.md:61 (commit 1105e488b, numstat 1/1 on CLAUDE.md only)
- Notes: A word-level diff of 1105e488b shows the old sentence replaced by "`state commit-now` passes an empty index as that fallback, discards the sets and dumps nothing. Whichever committer runs it, the cycle logs one WARN under `daemon` naming why `sessions.json` could not be read before it calls `LoadPrev`." The text is verbatim and sits between the two anchor sentences the criterion names (exact-string match confirmed on line 61). The old phrase "zero-value fallback with a WARN" no longer appears in CLAUDE.md. The commit touches only CLAUDE.md, and later commits leave this file unchanged. The new prose is accurate against the tree:
  - commit-now's fallback is `LoadPrev: func() *state.Index { return &state.Index{} }` and logs nothing (cmd/state_commit_now.go:114).
  - The cycle calls `logUnreadIndex(cycle.Logger, absent, err)` immediately before `prev = cycle.LoadPrev()` (internal/state/commit_cycle.go:124-125). `logUnreadIndex` emits exactly one WARN on either branch (internal/state/commit_cycle.go:156-163).
  - commit-now passes `daemonLogger` (cmd/state_commit_now.go:91, 115). The daemon binds `daemonLogger` (cmd/state_daemon.go:415) and passes `deps.Logger` into the cycle (cmd/state_daemon.go:274). `daemonLogger` is `log.For("daemon")` (cmd/state_common.go:8).
  So "one WARN under `daemon`, for whichever committer, before `LoadPrev`" holds. No other stale mention of the fallback WARN remains in CLAUDE.md.

TESTS:
- Status: Adequate
- Coverage: N/A. This is a prose-only documentation correction. The task forbids Go source or test changes and none were made.
- Notes: None

CODE QUALITY:
- Project conventions: Followed. The prose states substance with no task ids, phases or spec section references.
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
