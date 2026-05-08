TASK: session-scrollback-preview-3-1 — Focus state and pane-key resolution helpers

ACCEPTANCE CRITERIA:
- currentGroup() returns m.groups[m.windowIdx].
- currentRawIndices() returns raw tmux WindowIndex and PaneIndices[paneIdx], not ordinals.
- currentPaneKey() composes state.SanitizePaneKey(session, rawWindowIndex, rawPaneIndex), byte-identical to daemon writer.
- degenerate() true iff len(groups) == 1 && len(groups[0].PaneIndices) == 1.
- All helpers pure (no I/O, no mutation, no goroutines).

STATUS: Complete

SPEC CONTEXT:
Chrome counters are 1-based ordinals over enumeration order; pane-key resolution uses raw runtime tmux WindowIndex / PaneIndex. Conflating the two would render "Window 5 of 3" chrome or address a non-existent .bin. Helpers expose only the raw side; ordinal side stays inline at chrome call site (task 3-5).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go
  - currentGroup: L109-111
  - currentRawIndices: L118-121
  - currentPaneKey: L128-131
  - degenerate: L137-139
- Notes: All four are value-receiver methods on previewModel. currentGroup returns m.groups[m.windowIdx] verbatim. currentRawIndices reads g.WindowIndex and g.PaneIndices[m.paneIdx] from the cached group, never the ordinals. currentPaneKey composes via state.SanitizePaneKey with the raw indices. degenerate matches the spec predicate exactly. No struct shape changes from Phase 2; no I/O; no Cmd production. Helpers are reused by cycle handlers (L274, L296, L304) and the read dispatcher (L204), confirming single-source-of-truth usage.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_helpers_test.go
- Coverage:
  - currentGroup: ordinal 1 → middle group; name/index/PaneIndices verified.
  - currentRawIndices: standard 0-indexed (raw 2,7); non-contiguous (0,2,5) with base-index-1 panes.
  - currentPaneKey: 5 cases incl. unsafe session "foo/bar"; oracle = state.SanitizePaneKey(session, raw...) so byte-identical to daemon's key.
  - degenerate: 1x1 true; 1x2, 2x1, 2x2 all false.
- Notes: Plan lists five named tests; file delivers four Go test functions but covers all five named scenarios. Each test asserts one helper; no redundant coverage.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(). Value receivers consistent with other previewModel methods.
- SOLID: Good. Each helper has a single responsibility; currentPaneKey composes currentRawIndices rather than duplicating index arithmetic.
- Complexity: Low. All four helpers are 1-3 lines.
- Modern idioms: Yes. Named return values on currentRawIndices document (windowIndex, paneIndex) order at the signature level.
- Readability: Good. Doc comments explicitly call out raw-vs-ordinal distinction.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] previewModel doc says zero-value currentGroup "would index an empty slice" — actually it would panic on out-of-range, not silently. Minor wording drift.
- [idea] newPreviewModelForHelpers leaves enumerator/reader nil intentionally to enforce purity; a one-line comment that any helper accidentally calling reader/enumerator would nil-panic would make the purity contract self-documenting.
