TASK: spectrum-tui-design-3-10 — Update rename modal to the shared input-box helper (orange-on-edit consistency)

ACCEPTANCE CRITERIA (from tick tick-e123c3):
- Rename input renders via the shared 3-9 input-box helper (no bespoke border duplication).
- Border colour follows the unified grammar; orange-in-edit (always-editing) confirmed/reconciled.
- Visual check vs the Rename Modal (MV) frame; behaviour parity preserved (rename logic untouched).

STATUS: Complete

SPEC CONTEXT:
- Corrigendum (2026-06-22) supersedes the older violet/fill grammar: state is carried by BORDER COLOUR, never a fill — grey (border.separator) → accent.violet (focused) → accent.orange (editing + live cursor); `◉ EDIT MODE` indicator is accent.orange. "The rename modal's single always-editing input is therefore accent.orange with the `◉ EDIT MODE` badge (§8.4 / task 3-10)."
- §8.4: rename input is a single always-focused/always-editing field → accent.orange border + `▌` cursor + accent.orange `◉ EDIT MODE` badge; focused label accent.violet, value text.primary; `was:` line text.detail; footer `⏎ rename · esc cancel`; keys Enter/Esc; logic preserved (parity).
- §13.1: single-input modals (rename) always-editing → accent.orange border + badge; inputs render rounded corners (chips square).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/rename_modal.go (renameModalInputBoxRows:122-125 → renderInputBox; renameModalHeaderRow:81-84 → renderHeaderWithBadge); shared helpers in internal/tui/edit_modal.go (renderInputBox:105-120, renderHeaderWithBadge:276-293, inputBoxEditing:74); wiring at internal/tui/modal.go:77-78 and internal/tui/model.go:4032.
- Notes:
  - GENUINE reuse confirmed. git show c6c10d23 shows the bespoke box drawing (`lipgloss.NewStyle().Border(lipgloss.RoundedBorder())...BorderForeground(theme.MV.AccentViolet...)`) was DELETED and replaced by a single `renderInputBox(value, inputBoxEditing, true, renameInputInnerWidth, mode, colourless)` call — the same helper the edit modal's NAME input + chips use. No border duplication remains.
  - `renderHeaderWithBadge` was DRY-extracted from the edit modal in the same commit; both editModalHeaderRow (edit_modal.go:265) and renameModalHeaderRow now route through it. Edit modal's `showBadge` is gated on edit mode; rename passes `true` (always editing). Edit-modal render byte-for-byte unchanged (only the `width`→`contentWidth` param rename inside the moved body).
  - Border colour follows the unified grammar: rename passes `inputBoxEditing`, which maps via inputBoxBorderToken to theme.MV.AccentOrange. Rounded corners selected (rounded=true, the input convention). Orange-always reconciled per the corrigendum.
  - rename_modal.go added to the colour-literal guard allowlist (colour_literal_guard_test.go) — verified no raw hex; the orange cursor uses theme.MV.AccentOrange.ColorFor(mode), a token.

TESTS:
- Status: Adequate
- Coverage: internal/tui/rename_modal_test.go.
  - Structure/visual: TestRenameModal_ByteExact (full ANSI-stripped oracle, orange rounded box rows present), TestRenameModal_OrangeInputBoxOutline (box outline rows + accent.orange SGR), TestRenameModal_OrangeBlockCursor (reverse + orange), TestRenameModal_EditModeBadge + _EditModeBadgeRightAligned (always-on orange badge, right-aligned), TestRenameModal_NewNameLabel (violet label), TestRenameModal_InputValue, _WasLine, _Footer, _NoLitralEnterArrow, _SingleToneJoinedPanel, _BodyLayout, _Colourless (NO_COLOR carve-out), _LongOldNameTruncates (edge case).
  - Parity: TestUpdateRenameModal_EnterRenamesNonEmpty (trim + renameAndRefresh seam), _EnterEmptyIsNoOp, _EscCancels. recordingRenamer + stubLister seams drive the rename without nil-panic.
- Notes: Well-balanced. The byte-exact oracle pins layout; the colour-role tests cover orange box/cursor/badge + violet label; the three parity tests guard the untouched flow. Not over-tested (each test targets a distinct concern) and not under-tested (the always-editing orange treatment, the right-aligned badge, and the truncation edge case are all covered). No test executed — assessed by reading.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (cmd-package mock-injection convention is irrelevant here; tui tests are pure render). Tokens only, no raw hex (guard-enforced). Small-interface DI for the rename seam preserved.
- SOLID principles: Good. The shared renderInputBox / renderHeaderWithBadge are single-responsibility helpers reused by two modals; rename_modal.go is decomposed into focused one-job functions (header/label/box/was/footer rows).
- Complexity: Low. Each helper is a short, linear render function.
- Modern idioms: Yes. Idiomatic Go; lipgloss/textinput used per repo convention.
- Readability: Good. Doc comments thoroughly explain the always-editing rationale.
- Issues: Two STALE comments still describe the box as "violet" after the orange retarget (see non-blocking notes). The field-LABEL violet references (lines 31, 57, 105) are correct (the NEW NAME label is genuinely accent.violet, distinct from the box border).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/rename_modal.go:37 — comment "the span inside the violet side borders" is stale; the box border is now accent.orange (always-editing). Change "violet" → "orange" (or "the input box's side borders").
- [do-now] internal/tui/rename_modal.go:96 — comment "the three-row violet input box" is stale; the box is now accent.orange. Change "violet" → "orange".
