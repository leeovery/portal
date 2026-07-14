TASK: 5.1 — Multi-select mode state machine + session-identity selection set (restore-host-terminal-windows-5-1 / tick-3e042b)

ACCEPTANCE CRITERIA:
- `m` from the normal list sets MultiSelectActive()==true with SelectedSessionCount()==0 (enter with zero selected, no implicit mark).
- A second `m` on a session row sets IsSessionSelected(name)==true and count 1; a third on the same row returns to unselected (idempotent toggle pair).
- In By-Tag mode, toggling any one row of a multi-tag session marks/unmarks the single underlying Session.Name — count changes by exactly 1 regardless of row span.
- `m` on a HeaderItem (or no selectable row) leaves the set unchanged (no-op).
- `Esc` (filter not focused) sets MultiSelectActive()==false and count 0 (exit and clear the whole set).
- Uppercase `M` (Text "M") is a no-op — neither enters the mode nor toggles a mark.
- keymap_dispatch_guard_test.go passes with the new `m` descriptor entry + probe.

STATUS: Complete

SPEC CONTEXT: Spec "Multi-Select Mode → Trigger & marking / Granularity" — `m` enters an explicit mode you can sit in with zero selected (not an implicit mark-on-entry); `M` uppercase stays retired; `m` again toggles the cursor row in/out; `Esc` exits + clears. Selection is keyed on session identity (Session.Name), not the list row, so a By-Tag multi-tag session (Pattern B, multiple rows) marks the underlying session exactly once. HeaderItem rows are non-selectable and skipped (skipHeaderRow invariant). Scope boundary: this task establishes only the mode + set; the N>=2 spawn burst, `Enter` commit, banner/footer/marker, and stickiness are later Phase 5/6 tasks.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:437-438 (Model fields multiSelectMode/selectedSessions), :602-616 (MultiSelectActive/IsSessionSelected/SelectedSessionCount accessors), :3503-3530 (handleMultiSelectToggle), :3536-3543 (exitMultiSelect), :3268-3275 (selectedSessionItem), :3383-3385 (Esc exit branch), :3432-3433 (m dispatch arm), :1281-1299 (sessionDelegate + refreshSessionDelegate chokepoint).
  - internal/tui/keymap.go:97 (help-only `m` descriptor entry, placed after `s`).
  - internal/tui/keymap_dispatch_guard_test.go:143-147 (`m` parity probe).
- Notes: Matches the plan's "Do" list precisely. The `m` dispatch arm sits inside the rune switch below the `if m.sessionList.SettingFilter() { break }` guard, so `m` is a literal filter char while `/` is focused (verified). The Esc leading branch is placed before the FilterApplied progressive-back check and is reachable only when the filter is not focused. Enter (isRuneKey Text=="m") correctly excludes uppercase M. exitMultiSelect nils the set and is reused by the 5.7 N=0 commit. selectedSessionItem returns ok=false for HeaderItem/nil, giving the no-op backstop. Value-receiver + aliased-map mutation is standard Bubble Tea and well-documented in-source; no observable issue since Update always reassigns the model.

TESTS:
- Status: Adequate
- Coverage: internal/tui/multi_select_test.go maps 1:1 to the acceptance criteria — TestMultiSelectEnterMode (enter, count 0), TestMultiSelectToggleIdempotent (mark/unmark pair), TestMultiSelectByTagIdentity (multi-tag identity: 2-row session, toggle via row 0 marks once, toggle via row 1 unmarks — count keyed on name), TestMultiSelectHeaderRowNoop (cursor forced onto index-0 HeaderItem, no-op + mode retained), TestMultiSelectEscExitsAndClears (Esc exits + empties), TestMultiSelectUppercaseMNoop (M does not enter, does not toggle). The dispatch-parity probe is in keymap_dispatch_guard_test.go:143. Each test would fail if the corresponding behaviour broke (real state assertions, not tautologies).
- Notes: Not over-tested — no redundant assertions, no unnecessary mocks; the By-Tag test builds a real grouped list to exercise the multi-row case honestly. multi_select_keymap_test.go / initial_multiselect_option_test.go belong to sibling tasks (5.5 / 5.8) but exercise 5.1 state and add no redundancy here.

CODE QUALITY:
- Project conventions: Followed. Value-receiver handlers returning (tea.Model, tea.Cmd) match the updateSessionList dispatch-arm convention; test accessors mirror the SessionList* naming; no t.Parallel(); single delegate-construction chokepoint (sessionDelegate()) prevents drift between applyCanvasMode and refreshSessionDelegate.
- SOLID principles: Good — handleMultiSelectToggle / exitMultiSelect / selectedSessionItem each single-responsibility; the delegate build is DRY (one source).
- Complexity: Low — clear branches, no nesting beyond the toggle if/else.
- Modern idioms: Yes — map[string]struct{} set, nil-tolerant lazy init, idiomatic delete/insert toggle.
- Readability: Good — thorough doc comments explaining the enter-only-no-mark rule, the aliasing note, and the guard-ordering rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/help_modal_test.go:203-214 — TestHelpModalContent's asserted help-action list (which already pins help-only keys like "New session in cwd", "Rename session") omits the new help-only `m` row "Multi-select mode"; add it so the mode's `?`-help discoverability is regression-guarded. The descriptor (keymap.go:97) already drives the row into the generated body, so the assertion is safe and will pass. (The plan's conditional "update if byte-asserted" was not triggered — the help tests are strings.Contains subset checks, not byte-exact — so this is a coverage gap, not a test failure.)
