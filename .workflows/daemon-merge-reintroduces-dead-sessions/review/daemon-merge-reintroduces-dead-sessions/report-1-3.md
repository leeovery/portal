TASK: Add pane-level filtering to `mergeSkippedPanes` (1-3)

ACCEPTANCE CRITERIA:
- `mergeSkippedPanes` filters every prev pane at session, window, AND pane levels.
- New unit tests cover pane-level filtering.
- `mergePane` and `findOrAppendSession` not given belt-and-braces defensive checks.

STATUS: Complete

SPEC CONTEXT:
Per spec "Fix Component A → Filtering Levels", all three structural levels (session, window, pane) must filter. A prev pane in `skipSet` whose pane index is not present in the otherwise-live fresh window must be dropped. The merge entry point is the single point of enforcement.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/state/capture.go:122-147` — `mergeSkippedPanes` with three-level filter (session @125-128, window @130-133, pane @134-137).
  - `internal/state/capture.go:155-169` — `buildLiveStructure` extends nested map to `map[string]map[int]map[int]struct{}`.
- Notes:
  - Pane filter (line 135) precedes the `skipSet` check.
  - `mergePane` (176-187) and `findOrAppendSession` (193-205) contain no defensive checks; remain pure helpers.
  - Existing pane-index replacement contract preserved.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/state/capture_test.go:947-1008` — "does not merge a skipped pane whose pane index is absent from a live fresh window".
  - `internal/state/capture_test.go:1010-1074` — "canonical ordering preserved after pane-level drop".
  - Pre-existing pane-index replacement contract test preserved.
- Notes: Tests focused, not bloated. Both presence and absence asserted.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — `buildLiveStructure` is a small SRP helper.
- Complexity: Low.
- Modern idioms: Yes — `map[K]struct{}` set pattern, comma-ok lookups.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
