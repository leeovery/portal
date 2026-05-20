TASK: Extract assertKillBeforeNew helper for kill-before-new-session order checks

ACCEPTANCE CRITERIA:
- Helper extracted for kill-before-new-session ordering checks
- Both kill-session and new-session must be present (helper fails if either missing)
- Ordering assertion semantics preserved across four call sites

STATUS: Complete

SPEC CONTEXT: Phase 3 hygiene cycle. Task 3-6 targets duplicated inline ordering scans of mock.Calls for the "kill-session must precede new-session" invariant in internal/tmux/portal_saver_test.go. The invariant guards the daemon-leak regression — BootstrapPortalSaver must kill any stale _portal-saver session before creating its replacement.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/portal_saver_test.go
  - Helper: lines 106-130
  - Call sites: lines 275, 353, 730, 1545 (four call sites, matches plan)
- Notes:
  - Signature assertKillBeforeNew(t *testing.T, calls [][]string), calls t.Helper().
  - Scans for first kill-session and first new-session argv-set; fails if either missing OR if killIdx >= newIdx.
  - Empty-argv slices guarded (`if len(c) == 0 { continue }`).
  - Doc comment (lines 106-108) precisely describes contract.
  - Final assertion at line 127: `if killIdx == -1 || newIdx == -1 || killIdx >= newIdx` — covers all three failure modes.

TESTS:
- Status: Adequate
- Coverage: Four dedicated meta-tests at lines 132-179:
  - TestAssertKillBeforeNew_PassesWhenKillPrecedesNew (happy path)
  - TestAssertKillBeforeNew_FailsWhenKillMissing
  - TestAssertKillBeforeNew_FailsWhenNewMissing
  - TestAssertKillBeforeNew_FailsWhenNewPrecedesKill
  Each uses a stub *testing.T and asserts on stub.Failed() — correct pattern for testing t.Helper-using assertions without polluting parent test results.
- Notes: All branches of the final compound condition exercised. Not over-tested (no redundant permutations). Not under-tested.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel(), standard Go test idioms, t.Helper(), debug-friendly error dumps).
- SOLID principles: Single responsibility.
- Complexity: Low. Single linear scan + simple compound check.
- Modern idioms: Idiomatic.
- Readability: Good. Doc comment + self-documenting variable names (killIdx, newIdx).
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Combined error message at line 128 surfaces `killIdx=-1` when kill is missing — slightly less descriptive than per-mode messages. The unified message keeps the helper compact and is acceptable.
