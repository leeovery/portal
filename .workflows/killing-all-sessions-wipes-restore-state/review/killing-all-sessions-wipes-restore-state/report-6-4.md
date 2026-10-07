TASK: Corrections (killing-all-sessions-wipes-restore-state-6-4, tick-8298cd): rewrite two sentences in the `state` row of CLAUDE.md (line 61) so the commit cycle is described as it stands after phase 6 Task 2.

ACCEPTANCE CRITERIA:
- The `state` row holds the first replacement text verbatim where "The caller's `LoadPrev` is called only once the lock is held." stood, between "…a token-named transcript the other has just filed." and "The acquire is bounded at 5s, …"
- The `state` row holds the second replacement text verbatim where "The daemon's tick and shutdown flush dump, passing the daemon's in-memory previous index. `state commit-now` reads `sessions.json` under the lock, discards the sets and dumps nothing." stood, between "…either way the daemon's next tick retries." and "Those callers and the lazy-panel integration fixtures all enter through `RunCommitCycle`."
- `rg -F 'is called only once the lock is held' CLAUDE.md` and `rg -F 'in-memory previous index' CLAUDE.md` each find nothing
- No other text in CLAUDE.md changes, and no Go source or test file changes

STATUS: complete

SPEC CONTEXT: The bugfix makes every committing cycle run under the commit lock through RunCommitCycle. Phase 6 Task 2 changed the cycle so its skeleton merge, waiting-pane merge, carry and answered-pane hold all read sessions.json as read under the lock, which removes the lag a committer's own in-memory index has behind another committer's commit. The spec already measures "dropped" against the on-disk index, not the daemon's in-memory one, because the in-memory index does not see commit-now's writes. This task brings CLAUDE.md's prose into line with that code.

IMPLEMENTATION:
- Status: Implemented. The second sentence has since been revised again by a later task, and that revision is correct.
- Location: CLAUDE.md:61 (the `state` row), commit 1032450e3. The code it describes is in internal/state/commit_cycle.go:39-42 (LoadPrev contract), internal/state/commit_cycle.go:104-108 and :121-132 (readPriorIndex under the lock, LoadPrev only on a nil read, prev feeding captureAndRefile and keepAnsweredTranscripts), internal/state/commit.go:63-75 (readPriorIndex), cmd/state_daemon.go:271 (the daemon's fallback is deps.PrevIndex) and cmd/state_commit_now.go:113-114 (commit-now's fallback is an empty index).
- Notes:
  - Commit 1032450e3 touches only CLAUDE.md (1 insertion, 1 deletion on line 61). A word-level diff shows exactly the two prescribed replacements and nothing else. At that commit each new sentence sits verbatim between the anchors the criteria name, and both rg patterns match nothing (checked against `git show 1032450e3:CLAUDE.md`).
  - The first replacement still stands verbatim at HEAD and matches the code. RunCommitCycle reads sessions.json through readPriorIndex once it holds the lock. It calls LoadPrev only when that read returns nil, meaning the file is absent or cannot be read or decoded. The one `prev` it ends up with feeds both captureAndRefile (skeleton merge, waiting-pane merge, carry) and keepAnsweredTranscripts (the answered-pane hold).
  - The second replacement was revised by 1105e488b (task 8-3). Task 8-1 had moved the unreadable-index WARN out of commit-now and into the cycle (logUnreadIndex, internal/state/commit_cycle.go:156-163), which made 6-4's wording "commit-now passes a zero-value fallback with a WARN" untrue. The row now says commit-now "passes an empty index as that fallback" and that the cycle logs one WARN under `daemon` before calling LoadPrev. Both statements match commit_cycle.go:121-126 and state_commit_now.go:114. This change is sound and loses nothing the task intended, so it is not a finding.
  - Neither retired phrase appears anywhere in CLAUDE.md at HEAD, and no other passage still claims the cycle merges from the in-memory index.

TESTS:
- Status: Adequate (the change is prose only, so no test applies)
- Coverage: N/A. The task changes no Go source or test file, and the criteria require that.
- Notes: None

CODE QUALITY:
- Project conventions: Followed. The prose names no task IDs, phases or spec sections, and every symbol it names exists (`LoadPrev`, `RunCommitCycle`, `sessions.json`).
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
