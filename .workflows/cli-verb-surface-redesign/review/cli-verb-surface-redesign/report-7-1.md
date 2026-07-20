TASK: 7-1 — Repoint bootstrap warnings from the deleted `portal state status` to `portal doctor`

ACCEPTANCE CRITERIA:
- Both `CorruptSessionsJSONWarning` and `SaverDownWarning` rendered text assert `portal doctor` (not `portal state status`)
- Comment-only references out of scope (Task 7-7 covers those)
- No live user-facing string may instruct running a removed command

STATUS: Complete

SPEC CONTEXT:
The cli-verb-surface-redesign spec deletes `state status` and subsumes it into a new public `portal doctor` (spec §"doctor", line 297 "state status is subsumed", line 308 "Subsumes state status", and the retirement map line 431 `| portal state status | portal doctor |`). Any user-facing pointer that previously told users to run `portal state status` must now point at `portal doctor`. The analysis-report-c1 finding that seeded this task flags exactly one user-facing correctness gap: "live bootstrap warnings still point at the deleted portal state status." The pre-existing body copy of both warnings predates this redesign; this task changes only the command each warning tells the user to run.

IMPLEMENTATION:
- Status: Implemented (correct)
- Location: cmd/bootstrap/errors.go:54-59 (CorruptSessionsJSONWarning), cmd/bootstrap/errors.go:64-69 (SaverDownWarning)
- CorruptSessionsJSONWarning line 2 = "Check `portal doctor` or ~/.config/portal/state/portal.log." — repointed.
- SaverDownWarning line 2 = "Run `portal doctor` for details." — repointed.
- Confirmed via git that commit f15ccfd6 (Tcli-verb-surface-redesign-7-1) introduced the `portal doctor` strings, i.e. the warnings previously pointed at `portal state status` and were repointed by this task.
- Repo-wide `grep "portal state status" --include=*.go` returns ONLY the negative-assertion guard + its comment in errors_test.go (lines 103/110/111) — no live production string references the removed command anywhere.
- Notes: The only other repo reference is CHANGELOG.md:127, a historical 0.7.3 (2026-06-09) release note describing a past `portal state status` bug fix. That is a dated historical record, not a live instruction to run the command, and rewriting shipped changelog history is correctly out of scope.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/bootstrap/errors_test.go:68 TestCorruptSessionsJSONWarning_returnsExactSpecCopy — asserts the exact two rendered lines AND calls assertRepointedToDoctor.
  - cmd/bootstrap/errors_test.go:85 TestSaverDownWarning_returnsExactSpecCopy — same shape.
  - cmd/bootstrap/errors_test.go:104 assertRepointedToDoctor — positive (must contain "portal doctor") AND negative (must NOT contain "portal state status") guard over the joined body. This is the direct, load-bearing encoding of the acceptance criteria.
  - cmd/bootstrap_warnings_test.go:195 TestPersistentPreRunE_EmitsWarningsToStderrOnCLIPath — end-to-end CLI path asserts the full rendered stderr lines including "Run `portal doctor` for details." and "Check `portal doctor` or ~/.config/portal/state/portal.log.", proving the repointed text reaches the user unmodified.
- Notes: Not under-tested — both warnings covered at the constructor level (exact copy + intent guard) and at the CLI emission boundary. Not over-tested — the negative `portal state status` guard is technically redundant with the exact-copy assertion, but it is intentional and documents the repoint invariant so it survives any future loosening of the exact-copy check; a reasonable, focused belt-and-suspenders, not bloat. Tests would fail if the string regressed to `state status` or dropped `portal doctor`.

CODE QUALITY:
- Project conventions: Followed. Warning constructors return the canonical `warning.Warning` alias; tests obey the no-`t.Parallel` rule (file header comment reiterates it) and use `t.Helper()` on the shared assertion.
- SOLID principles: Good — single-responsibility constructors, no coupling change.
- Complexity: Low — pure string literals.
- Modern idioms: Yes.
- Readability: Good — godoc on each constructor explains the path; assertRepointedToDoctor is self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
