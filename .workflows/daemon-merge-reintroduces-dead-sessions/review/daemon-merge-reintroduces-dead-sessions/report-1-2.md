TASK: Add window-level filtering to `mergeSkippedPanes` (1-2)

ACCEPTANCE CRITERIA:
- `mergeSkippedPanes` filters every prev pane against the fresh `idx.Sessions` at session, window, and pane levels.
- New unit tests cover window-level filtering.
- `mergePane` and `findOrAppendSession` not given belt-and-braces defensive checks.

STATUS: Complete

SPEC CONTEXT:
Fix Component A — single-file change in `internal/state/capture.go`. Merge must build a local structural map from `idx.Sessions` and filter prev panes at all three levels. Task 1-1 added session-level + the helper; 1-2 is the window-level extension. Helpers remain non-defensive.

IMPLEMENTATION:
- Status: Implemented
- `internal/state/capture.go:122-147` — `mergeSkippedPanes` performs the three-level filter via `live[ps.Name]` (session), `liveWindows[pw.Index]` (window), `livePanes[pp.Index]` (pane).
- `internal/state/capture.go:155-169` — `buildLiveStructure(idx)` projects `idx.Sessions` into nested `map[string]map[int]map[int]struct{}`.
- `internal/state/capture.go:176-205` — `mergePane` and `findOrAppendSession` carry no defensive checks.
- Public signature of `mergeSkippedPanes` unchanged.

TESTS:
- Status: Adequate
- Coverage in `internal/state/capture_test.go`:
  - lines 748-809 — `does not merge a skipped pane whose window is absent from a live fresh session`.
  - lines 811-878 — `drops only stale windows from a mixed prev session`.
  - lines 880-945 — `canonical ordering preserved after window-level drop`.
- All three listed edge cases covered. Tests use existing `captureMock` / `listSessionsFor` / `paneLine` harness; no `t.Parallel()`.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. `buildLiveStructure` single-responsibility.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. `sessionLive` / `windowLive` / `paneLive` naming makes intent legible.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `buildLiveStructure` allocates three nested maps per merge call on every tick where `len(skipSet) > 0`. Likely irrelevant at typical N (<100 panes); flag if scale grows.
- [idea] `mergeSkippedPanes` docstring describes the post-1-3 state. After 1-2 in isolation only two levels filter; pane-level lands in 1-3. Forward-consistent rather than misleading.
