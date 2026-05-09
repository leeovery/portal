TASK: Replace codifying-bug test with session-level filter + add session-level filter (1-1)

ACCEPTANCE CRITERIA:
- `mergeSkippedPanes` filters prev panes against fresh `idx.Sessions` at session/window/pane levels; public signature unchanged; structural map built locally from `idx.Sessions`.
- `mergePane` / `findOrAppendSession` get no belt-and-braces defensive checks.
- Buggy test at `internal/state/capture_test.go:570-617` replaced with its inverse: a prev session whose name is not in fresh must NOT be merged even when paneKey is in `skipSet`.

STATUS: Complete

SPEC CONTEXT:
Spec ("Filtering Levels") mandates filtering against the live fresh index at all three levels. Structural map must be built locally inside `mergeSkippedPanes` from `idx.Sessions` to avoid signature changes. `mergePane` / `findOrAppendSession` are in the same file but their bodies/contracts must remain untouched — single point of enforcement at the merge entry. Session-level case: prev session not in fresh must NOT be merged even when paneKey is in `skipSet`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/state/capture.go:122-147` — `mergeSkippedPanes`; signature unchanged; calls `buildLiveStructure(*fresh)` locally and gates merging at session (lines 125-128), window (130-133), and pane (134-137) levels.
  - `internal/state/capture.go:155-169` — `buildLiveStructure` helper produces nested session→window→pane lookup map.
  - `internal/state/capture.go:176-205` — `mergePane` and `findOrAppendSession` bodies/signatures unchanged; no defensive checks added.
- Notes: Public signature `(fresh *Index, prev Index, skipSet map[string]struct{})` preserved. Doc comment cites spec section.

TESTS:
- Status: Adequate
- Coverage:
  - Buggy subtest "merges a skipped pane's session and window from prev when missing from fresh" no longer exists.
  - Replacement at `internal/state/capture_test.go:643-692` — subtest `does not merge a skipped pane whose session is absent from fresh`.
  - Legitimate-merge case preserved at `capture_test.go:570-641` (`hydrate_in_progress_pane_merges_from_prev_at_matching_coords`).
- Notes: Negative test would fail on the buggy code; not a false-green.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`; helper-style refactor consistent with `capture.go`.
- SOLID principles: Good. Single point of enforcement.
- Complexity: Low. Three nested loops with three early-continue guards.
- Modern idioms: Yes. Comma-ok map idiom; pre-sized maps.
- Readability: Good. Doc comments name spec section.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
