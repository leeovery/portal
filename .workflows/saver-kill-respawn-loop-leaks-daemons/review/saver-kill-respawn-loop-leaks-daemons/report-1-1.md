TASK: Reframe portalSaverVersionMismatch table tests to cover all six matrix rows

ACCEPTANCE CRITERIA:
- Renamed test exists and runs under `go test ./internal/tmux/...`
- All six cases present with distinct names pinning expected boolean
- Leading comment block reframes the predicate-as-one-input framing
- No production code in portal_saver.go modified by this task (superseded by Task 1-3)

STATUS: Complete

SPEC CONTEXT:
Spec §Testing Requirements required the portalSaverVersionMismatch table test to be reframed so documentation no longer encodes "absent counts as mismatch" as load-bearing. Spec's literal table pinned absent→true at the predicate layer with the alive-check (in EnsurePortalSaverVersion) as the authoritative kill gate.

IMPLEMENTATION:
- Status: Implemented (with codebase evolution beyond task wording)
- Locations:
  - internal/tmux/portal_saver_test.go:1893-2008 (TestShouldKillSaverOnVersionDecision_PredicateMatrix)
  - internal/tmux/portal_saver.go:342-382 (shouldKillSaverOnVersionDecision)
  - internal/tmux/export_test.go:17-23 (test-only re-export)
- Notes: During later cycles, portalSaverVersionMismatch was unified into a single shouldKillSaverOnVersionDecision predicate that encodes the kill-decision matrix (no kill on alive+absent). The reframed table test correctly targets this unified predicate. As a consequence, the absent row pins false (no kill) rather than the spec table's true. The leading docblock (lines 1893-1909) explicitly addresses this evolution and cross-references EnsurePortalSaverVersion as authoritative.

TESTS:
- Status: Adequate
- Coverage: Eight cases cover the six required rows plus two superset cases:
  1. equal_non_dev_match → false (row 1)
  2. mismatched_non_dev → true (row 2)
  3. readErr_ErrVersionFileAbsent_no_kill → false (row 3, reframed)
  4. readErr_non_absent_io_error (fs.ErrPermission) → true (row 4)
  5. dev_version_stored → true (row 5)
  6. dev_version_current → true (row 6)
  7. empty_stored → true (extra dev-equivalent)
  8. empty_current → true (extra dev-equivalent)
- Notes: Driver uses test-only seam tmux.ShouldKillSaverOnVersionDecision. Each case has explanatory comment; failure message names case via inputs.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel(), test-only export seam, idiomatic table test)
- SOLID: Single responsibility predicate, isolated test
- Complexity: Low — table + single t.Run loop
- Modern idioms: Yes
- Readability: Good — per-case inline comments, 13-line docblock explains post-unification framing
- Issues: None blocking

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] The original task AC references the predicate by its old name (portalSaverVersionMismatch) and pins absent→true. The implementation evolved (Task 1-3 unified the predicate). The literal task ACs are not met word-for-word, but the spec-level intent is fully satisfied. Future plan documents could note when task ACs are superseded by later-phase refactors.
- [idea] Cases 7 and 8 (empty_stored, empty_current) are extras beyond the spec's six-row table. They harden dev-short-circuit semantics around empty-string aliases and are worth keeping; flagging only because the task said "exactly the six rows".
