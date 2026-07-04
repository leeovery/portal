TASK: analysis-2-2 (tick-0a34ae) — Delete redundant verifyRenameHookFiredOnce; reuse the shared assertHookFireCount helper

ACCEPTANCE CRITERIA:
- verifyRenameHookFiredOnce no longer exists anywhere in internal/restore/.
- The single former call site now calls assertHookFireCount(t, hookFireFile, 1) and the 3-5 headline test still asserts the hook fired exactly once.
- The integration build compiles clean (no unused-import / unused-function errors).
- No behaviour change to the 3-5 test's pass/fail semantics.

STATUS: Complete

SPEC CONTEXT: Analysis-cycle-2 dedup chore. Two functions in the same restore_test package (both //go:build integration) embodied identical read-count-assert logic over the HOOK_FIRED side-effect file: the local verifyRenameHookFiredOnce (single call site, hardcoded want=1) and the already-shared assertHookFireCount(t, file, want). Task collapses the duplicate onto the shared helper so the HOOK_FIRED marker string and count-mismatch message live in exactly one place. No product behaviour is in scope — this is test-package internal consolidation only.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restore/rename_reboot_hook_integration_test.go:344 (call site now assertHookFireCount(t, hookFireFile, 1)); verifyRenameHookFiredOnce definition deleted (former ~378-395, confirmed via git show 26381485).
- Notes: Verified against the actual commit diff (26381485). Change is exactly the three prescribed edits:
  (1) call site line 344 verifyRenameHookFiredOnce → assertHookFireCount(t, hookFireFile, 1);
  (2) function + doc-comment fully deleted (trailing 22 lines removed, file now ends at verifyHookKeyed line 376);
  (3) the incidental comment at line ~209 updated to reference the new helper name ("assertHookFireCount(t, file, 1)") rather than the removed one — a correct, in-spirit follow-through, not scope creep.
  grep across internal/restore/ confirms zero remaining occurrences of verifyRenameHookFiredOnce (the only surviving textual reference to the OLD helper is a historical note in the durability file's header comment about the reboot_roundtrip shape — not a reference to the deleted symbol). assertHookFireCount is unchanged (still at durability_integration_test.go:307-317) and is now shared by three files: rename_reboot_hook (3-5, x1), rename_reboot_durability (3-6, x2), multipane_legacy (3-7, x1). The 3-6 and 3-7 files were untouched by this commit (commit stat shows only the 3-5 file changed, plus tracking metadata).

TESTS:
- Status: Adequate
- Coverage: This is a test-only dedup; no new test is warranted and none was added (correct). The 3-5 headline exactly-once assertion is preserved — assertHookFireCount(t, hookFireFile, 1) is semantically identical to the deleted verifyRenameHookFiredOnce(t, hookFireFile): both ReadFile → strings.Count(data, "HOOK_FIRED") → assert == 1. The read-error path (t.Fatalf on absent file = bare-shell miss) and the mismatch path (t.Errorf) are behaviourally equivalent; only the diagnostic wording differs ("hook fired N times cumulatively; want exactly M" vs "hook fired N times; want exactly 1"), which does not alter pass/fail semantics.
- Notes: No under- or over-testing introduced. The consolidation reduces, not expands, assertion surface.

CODE QUALITY:
- Project conventions: Followed. Both helpers are t.Helper()-marked; the shared one keeps that. No t.Parallel() involved. golang-testing/golang-code-style DRY guidance is exactly what this task serves.
- SOLID principles: Good — single source of truth for the read-count-assert contract.
- Complexity: Low (net -17 lines).
- Modern idioms: Yes — no change.
- Readability: Good. The updated inline comment at line 209 keeps the doc reference consistent with the surviving helper name, avoiding a stale reference.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
