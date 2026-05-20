TASK: Revise function comment at internal/tmux/portal_saver.go:232-241 to match new contract

ACCEPTANCE CRITERIA:
- Targeted comment no longer asserts "absent counts as mismatch" as load-bearing
- New comment documents predicate behaviour AND states alive-check is consulted first
- Cross-references EnsurePortalSaverVersion
- Build + tests pass
- No code outside comment block modified

STATUS: Complete

SPEC CONTEXT: Spec §Change 1 mandates revising the comment that formerly encoded "ErrVersionFileAbsent counts as mismatch — for first-ever bootstrap or user-initiated state-dir cleanup" — alive-check ordering is now authoritative; absence is no longer load-bearing.

IMPLEMENTATION:
- Status: Implemented (in a different shape than literally planned, but spec-faithful — anticipated by task wording)
- internal/tmux/portal_saver.go:282-319 — EnsurePortalSaverVersion godoc (decision-matrix table with alive=yes/absent=no-kill row explicit; rationale paragraph at 296-300 naming the prior bug)
- internal/tmux/portal_saver.go:342-357 — shouldKillSaverOnVersionDecision godoc (closest analog to the deleted portalSaverVersionMismatch; explicit "Absent version file ... no kill — Task 1-4 layers a defensive write here from the caller"; cross-references EnsurePortalSaverVersion as caller holding the alive-check)
- Notes:
  - Task 1-3 removed portalSaverVersionMismatch entirely and replaced it with shouldKillSaverOnVersionDecision; the task description explicitly anticipated this drift.
  - Replacement comment at 342-357 explicitly states "Callers must have already established that the daemon is alive — this helper does not consult BootstrapAliveCheck" — the post-fix contract verbatim.
  - Grep for "absent counts as" across the repo returns only spec/plan/tick artifacts; no source file still asserts the old load-bearing claim.

TESTS:
- Status: N/A (task explicitly states "Tests: None — comment-only edit")
- Coverage: Behavioural contract covered by Tasks 1-1, 1-3, 1-4

CODE QUALITY:
- Project conventions: Followed (godoc style, attached // prefix lines, no blank line between comment and decl)
- SOLID / Complexity / Modern idioms: N/A (comment-only); ASCII markdown matrix is readable and stable
- Readability: Strong — decision matrix at 287-294 is self-documenting; rationale at 296-300 names the prior bug explicitly
- Issues: None

BLOCKING ISSUES: None

NON-BLOCKING NOTES:
- [idea] The shouldKillSaverOnVersionDecision comment at 342-357 cross-references EnsurePortalSaverVersion by naming it as the caller, but lacks an explicit "see X for the full kill-decision matrix" forward-pointer. Functionally equivalent; minor wording polish would help a future reader.
