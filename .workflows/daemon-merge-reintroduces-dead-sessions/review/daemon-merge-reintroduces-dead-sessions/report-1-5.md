TASK: Preserve hydrate-in-progress merge behaviour (positive test) (1-5)

ACCEPTANCE CRITERIA:
- Add or extend a positive test asserting that a phase-A skeleton-restored pane is still merged from prev with prev's authoritative pane state.
- `internal/restore/session_markers_test.go` and remaining `TestCaptureStructureMergeSkippedPanes` subtests stay green.
- Edge cases: prev pane state wins at matching coords; sessions in both fresh and prev not duplicated; canonical ordering survives merge.

STATUS: Complete

SPEC CONTEXT: Specification → Fix Component A → Preserved Behavior + AC #6: the new structural live-set filter must not regress the legitimate hydrate-in-progress flow. Phase A creates the session in tmux BEFORE setting the marker, so a marker-protected pane has session/window/pane all present in fresh.

IMPLEMENTATION:
- Status: Implemented
- Locations:
  - New positive subtest: `internal/state/capture_test.go:570-641` — `TestCaptureStructureMergeSkippedPanes/hydrate_in_progress_pane_merges_from_prev_at_matching_coords`.
  - Production filter: `internal/state/capture.go:122-147` (`mergeSkippedPanes`) gated by `buildLiveStructure` (`capture.go:155-169`).
  - `internal/restore/session_markers_test.go` untouched.
- Notes: Subtest covers two distinct contracts vs the older skip-set authority subtest at lines 528-568.

TESTS:
- Status: Adequate
- Coverage:
  - Marker set + all three structural levels live in fresh: prev pane's CWD/CurrentCommand wins (lines 614-620).
  - No session duplication when same session in both fresh and prev (624-629).
  - Canonical ordering survives merge (633-640).
  - Existing subtests preserved.
- Notes: Doc comment references spec → Fix Component A → Preserved Behavior + AC #6 — good traceability.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: N/A — test addition.
- Complexity: Low.
- Modern idioms: Standard `t.Run` subtest with structured `state.Index` literals.
- Readability: Good.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [quickfix] Subtest name uses snake_case while peers use sentence-case prose. Mechanical rename for stylistic consistency.
- [idea] New subtest and the immediately-preceding "preserves prior pane data when its key is in the skip set" subtest share an almost-identical fixture. Consider folding both, or adding cross-reference comment. Low priority.
