TASK: session-tagging-and-grouping-3-8 — Flatten-on-filter and restore-grouping-on-clear

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: filter active flattens; headers absent while filtering; clear restores grouping; Flat-mode filter unchanged; re-group respects current mode on clear; FilterApplied vs Filtering both suppress.

SPEC CONTEXT: spec § Filter Composition / AC14 — while filter active list flattens, headers step aside, clear restores grouped view, filtering otherwise unchanged; render-layer invariant makes this trivial (filter only sees session instances).

IMPLEMENTATION: Implemented.
- session_item.go:153-183 groupHeading, guard :166 `if m.FilterState() != list.Unfiltered { return "", false }`. Headers drawn inside Render (never in m.Items()), so slice never un-grouped — only heading drawing suppressed. Restore-on-clear automatic (guard falls through against still-grouped slice, preserves current mode via retained GroupKey/GroupHeading). != Unfiltered captures both Filtering and FilterApplied. s-literal-while-filtering is separate (3-4). Exemplary comment block.

TESTS: Adequate. session_item_test.go:441-584 TestSessionDelegateFlattenOnFilter — suppresses headers By Project + By Tag; restores on clear (with precondition); Flat-mode unchanged (byte-identical + no separator); restores current mode's headings (By Tag not By Project); both Filtering+FilterApplied; By-Tag duplicate rows expected. Real list.Model+delegate. Behaviour-focused.

CODE QUALITY: Conventions followed (no t.Parallel, behaviour-level, external tui_test package); SOLID good (single chokepoint); low complexity (one early-return); idiomatic enum compare. Excellent comment. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES:
- [idea] session_item_test.go:303 — test redeclares local const groupSeparator mirroring production constant; a future glyph change wouldn't be caught (stale mirror). Optionally bridge via export_test.go; acceptable as-is (external test package tradeoff, low risk).
