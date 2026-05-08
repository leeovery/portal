TASK: session-scrollback-preview-2-5 — Filter-mode Space passthrough integration

ACCEPTANCE CRITERIA:
- When SettingFilter() is true, Space passes through to bubbles/list and is consumed as text input.
- When SettingFilter() is true, NewPreviewModel is NOT called.
- After Enter commits the filter, Space opens preview on the highlighted match.
- No second key binding for "open preview while filtering".
- A literal space character is observably present in FilterValue() after typing Space during SettingFilter().

STATUS: Complete

SPEC CONTEXT:
Spec § Filter Behaviour with Preview mandates "default bubbles/list semantics" — preview does not intercept Space while filtering; no magic Space; no second binding for open-while-filtering. Allowing literal-space typing in the filter is a hard requirement.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:1264-1266 — SettingFilter() short-circuit (covers all keys, including Space) before reaching the Space branch.
  - internal/tui/model.go:1273-1288 — Space branch (only one).
- Notes: The code uses an earlier general break at line 1264 (covering all keys during filter mode) rather than placing the SettingFilter check inside the Space branch as literally written in the task's Do snippet. Functionally equivalent for Space and arguably cleaner — satisfies all acceptance criteria.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_filter_test.go
- Coverage:
  - Literal-space passthrough (Space inserts into FilterValue while SettingFilter true).
  - No page transition during SettingFilter Space.
  - Leading-space edge case.
  - Post-Enter-commit Space opens preview on highlighted match.
  - Static single-binding invariant (TestExactlyOneSpaceBranchInUpdateSessionList).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single Space branch. Filter detection in single place.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] TestExactlyOneSpaceBranchInUpdateSessionList uses substring count of tea.KeySpace in model.go's updateSessionList body. A future code change adding tea.KeySpace in a comment or string literal would spuriously inflate the count. Consider tightening to a `case msg.Type == tea.KeySpace` line-anchored match if brittleness emerges.
- [idea] Implementation divergence from the literal Do-snippet (early break for all keys vs. inline SettingFilter check inside Space branch). The chosen approach is better engineering but worth recording in the task closure trail.
