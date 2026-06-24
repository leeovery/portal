TASK: spectrum-tui-design-2-6 — Sessions Flat row anatomy + violet left-bar selection

ACCEPTANCE CRITERIA (from tick-771c41):
- Name = flex left column text.primary (selected: text.on-selection); window count = fixed-width trailing slot text.detail (selected: text.strong) reading "N window"/"N windows"; attached marker = fixed-width trailing slot, ● attached in state.green when attached, empty same-width slot when not.
- Selected row shows a thick accent.violet 2-cell left-bar column + bg.selection tint; unselected rows have no bar/tint.
- Attached bullets and window counts column-align regardless of name length; over-long names truncate with ellipsis without pushing the trailing slots off-row.
- On the selected row the attached marker keeps state.green (not recoloured); all selected-row foregrounds clear the contrast floor against bg.selection.
- Flat row is name-only (no project/path column); row is exactly one delegate line (Height()==1).
- All colours via tokens (no 212/76/#777777 literals survive in the delegate).
- VISUAL VERIFICATION (mandatory): vhs capture of the flat view (mixed attached/unattached, a long name, a selected row) matching Sessions — Modern Vivid v2 / (Light).
- Behaviour parity: selection, attach-target resolution (keys on Session.Name), window-count/attached semantics unchanged.

STATUS: Complete

SPEC CONTEXT: §4.1 (row anatomy) — name flex left column, fixed-width right-pinned trailing slots for window count and attached marker; on the selected row ● attached KEEPS state.green (green-on-bg.selection clears the floor; foreground-on-tint pairings of §2.9). §3.3 (selection) — thick ▌ accent.violet 2-cell left-bar column over bg.selection tint, name in text.on-selection, unselected no bar/tint. §2.7 — over-long names truncate with …. §1 — owned mode-matched canvas painted on every cell. This is a reskin (§1) — selection/attach machinery is preserved, only rendering changes.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/session_item.go — renderSessionRow (lines 362-466) is the §4.1 row anatomy; renderLeftBarColumn (330-336) the §3.3 2-cell bar column; rowBgStyle (292-300) + rowTokenStyle (309-318) the shared bg.selection/canvas leaf paint; constants leftBarColumnWidth/countSlotWidth/attachedSlotWidth/attachedMarker (28-61). Implementing commit 9213797b (commit-msg "2-7" — the known +1 offset vs internal IDs).
- Notes:
  - Name: text.primary, selected → text.on-selection, bold via nameBase (397-400, 419/429). Count: text.detail, selected → text.strong (402-405, 436). Attached: state.green on both selected and unselected (446) — single token, no per-context override. All token-backed; no literal hex at the call site.
  - Flex layout: total = m.Width(); used = bar + indent + nameGap + countSlot + attachedSlot + rightMargin; nameWidth = total-used, floored to 1; ansi.Truncate(name, nameWidth, "…") then padded so the trailing slots are right-pinned (414-453). Width()==0 fallback flows left-to-right with no truncation (418-420).
  - Attached marker slot fixed at lipgloss.Width("● attached"); unattached renders an empty same-width slot (444) preserving column alignment.
  - Pathological-narrow safety clamp: ansi.Truncate(row, total, "…") (462-464) so the fixed ~25-cell trailing block can never overflow the list width and bleed past the gutter.
  - Height()==1 unchanged (230); Spacing()==0 (238); single-line row.
  - 2-7 indent hook preserved intact: it.GroupKey != "" → groupRowIndent folded into the width budget (384-388, 415); flat rows render flush. Matches the plan note "leave the existing indent hook intact for task 2-7."
  - §7.1 bonus: while list.Filtering, selected forced false so no bar/tint renders during input-active (372-374) — consistent with the never-both-input-cursor-and-selected-row invariant; behaviourally additive, not a parity break.
  - Visual verification: testdata/vhs/sessions-flat.png and sessions-flat-light.png (+ .tape) drive a 12-session fixture mixing attached/unattached with an over-long name (agentic-workflows-code-based) and the cursor on row 0. Both match their reference frames (testdata/vhs/reference/sessions-modern-vivid-v2.png and sessions-modern-vivid-light.png): violet ▌ bar, bg.selection band, name flex column, right-pinned aligned count column, green ● attached column, selected-row count in the brighter text.strong tone, name-only flat rows.

TESTS:
- Status: Adequate
- Coverage: internal/tui/session_row_anatomy_test.go is the §4.1 gate, asserting colour roles in exact mode-resolved SGR (both Dark and Light):
  - FlexNameFixedTrailingSlots — name/count/marker present, count right of name, row width == list width.
  - ColumnAlignsRegardlessOfNameLength — count + bullet columns identical across short/long names.
  - EmptyAttachedSlotPreservesAlignment — unattached row has no marker but count column + total width match the attached row.
  - SelectedShowsVioletBarTintAndOnSelectionName — ▌ present, accent.violet fg SGR, bg.selection bg params, text.on-selection name fg.
  - UnselectedHasNoBarOrTint — negative: no ▌, no bg.selection, canvas paint present.
  - AttachedKeepsStateGreenWhenSelected — state.green on both selected and unselected attached rows; light precondition asserts green != on-selection so the marker is a distinct run.
  - SelectedCountInTextStrong — text.strong selected vs text.detail unselected.
  - OverLongNameTruncatesWithoutPushingSlots — full name absent, … present, both slots survive, width == list width.
  - NeverOverflowsAtNarrowWidths — widths {1,5,10,20,25,26,29,40,80} × short/long × attached/unattached × both modes, width never exceeds total.
  - FlatIsNameOnly — Session.Dir set but never leaks; name present.
  - NoRawAnsiColourLiterals — render-level cross-check that 38;5;212 / 38;5;76 / 38;2;119;119;119 are absent.
  - HeightStaysOne — Height()==1, no newline.
  session_item_test.go covers behaviour parity (FilterValue on Session.Name, window pluralization, attached badge present/absent, selected ▌, short-name-no-truncate, group-metadata items still render name-only). colour_literal_guard_test.go is the AST-level source guard (no lipgloss.Color from a raw literal in any tui render file). theme/contrast_test.go owns the numeric foreground-on-tint floor (state.green / text.on-selection / text.strong / text.primary each vs bg.selection in both modes; TestStateGreenClearsCanvasAndSelection justifies the single green token).
- Notes: Not under-tested — every acceptance criterion and every named edge case (state.green-on-selection, alignment regardless of name length, empty attached slot, ellipsis truncation, one-delegate-line) has a direct assertion, in both modes, in exact SGR. Not over-tested — the legacy session_item_test.go cases (window pluralization, badge present/absent) overlap the new anatomy tests at the "is the string present" level but exist to pin behaviour parity for the reskin, which is a legitimate distinct concern; not redundant bloat. The contrast floor lives in the theme package (correct home) rather than being re-derived here.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (package-level mock convention honoured). Tokens only, no call-site hex. Shared leaf-paint helpers (rowBgStyle/rowTokenStyle/renderLeftBarColumn) homed as free functions so the Session and Project delegates share one selection-vs-canvas decision (DRY; these are the 6-3/9-3 refactors, verified against current code). NO_COLOR carve-out threaded through every helper.
- SOLID principles: Good. renderSessionRow is the single row-anatomy owner; structural decisions (bg, token, bar column) delegate to shared free functions. Width-budget arithmetic is localised.
- Complexity: Acceptable. renderSessionRow is linear with two clear branches (Width()==0 fallback; selected vs not via token selection). The safety clamp is a documented no-op on the happy path.
- Modern idioms: Yes. ansi.Truncate / lipgloss.Width for display-width correctness (multibyte ▌/● safe); column measurement in the tests uses display width, not byte offsets.
- Readability: Good. Heavy but accurate doc comments explain every magic width (countSlotWidth=11 fits "999 windows", rowRightMargin symmetry, the clamp rationale). Intent is self-documenting.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/session_item.go:43 — countSlotWidth is a hard-coded 11 ("fits 999 windows"); a session with ≥1000 windows would bleed into the attached slot (the final-row clamp prevents overflow but would visually crowd). Decide whether to cap/derive this rather than pin a literal — pathological, almost certainly never hit; flagging only for completeness.
