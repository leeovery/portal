TASK: session-scrollback-preview-4-4 — Brand-new-session traversal with placeholders on every pane

ACCEPTANCE CRITERIA:
- Initial open on a brand-new session shows placeholder on first pane + correct chrome counts.
- Tab, ], [ all advance focus correctly when every pane is (nil, nil).
- After each cycle, chrome reflects new focus via 1-based ordinals.
- All structural entries reachable regardless of content state.
- Mixed variant: bytes panes render bytes, placeholder panes render placeholder, cycle keys uniform.
- No code path treats (nil, nil) as "skip".

STATUS: Complete

SPEC CONTEXT:
§ Brand-new-session Edge Case — sessions with no .bin content yet must remain fully traversable; chrome from at-open enumeration; placeholder per pane; no live-capture fallback. Mixed split-windowed-in pane case explicitly cited.

IMPLEMENTATION:
- Status: Implemented (test-only — production code unchanged, dispatcher from 4-1/4-2)
- Tests: internal/tui/pagepreview_brandnew_test.go (342 lines, 6 test funcs)
- Production dispatcher: internal/tui/pagepreview.go:202-215; cycle handlers at :273-306; chrome at :163-173.
- All required helpers reused from sibling test files.

TESTS:
- Status: Adequate
- Coverage:
  - TestPreviewBrandNew_EveryPaneRendersPlaceholder — full cycle: (w0,p0) → Tab (w0,p1) → Tab wrap (w0,p0) → ] (w1,p0) → Tab (w1,p1).
  - TestPreviewBrandNew_ChromeCountsAccurateAcrossAllPlaceholderCycles — 6-step driver.
  - TestPreviewBrandNew_NextWindowAdvancesAndTabCyclesWithinWindowUnderAllPlaceholders.
  - TestPreviewBrandNew_CycleKeysDoNotSkipPlaceholderPanes — exhaustive 4-coord traversal map; len(reader.calls) == 4.
  - TestPreviewMixed_BytesPaneAndPlaceholderPanesCoexist.
  - TestPreviewMixed_FocusFromBytesPaneToPlaceholderAndBackIssuesFreshTailCalls — w0p0Calls==2.
- All 6 tests covered. [ wrap is folded into chrome driver. Not over-tested.

CODE QUALITY:
- Project conventions: Followed — no t.Parallel(), interface-injected seams.
- SOLID: Good — brandNewFixtureGroups() factored once, reused.
- Complexity: Low.
- Modern idioms: Good.
- Readability: Good — every test cites spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] TestPreviewBrandNew_CycleKeysDoNotSkipPlaceholderPanes asserts len(reader.calls) == 4. The "4" is "one per focus event in this 4-step sequence" — coincidentally also pane count. A comment clarifying the dimension would help.
- [idea] TestPreviewMixed_FocusFromBytesPaneToPlaceholderAndBackIssuesFreshTailCalls only exercises w0; w1 panes never visited.
