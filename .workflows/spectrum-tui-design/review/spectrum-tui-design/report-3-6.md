TASK: spectrum-tui-design-3-6 — Rename modal reskin (MV): labelled NEW NAME input + focus grammar + blank-screen (tick-a99733)

ACCEPTANCE CRITERIA (from tick + corrigendum):
- Header `Rename session` in text.primary.
- `NEW NAME` label in accent.violet, value in text.primary.
- Always-editing input box: accent.orange border + live block cursor + `◉ EDIT MODE` badge (CURRENT spec per corrigendum 2026-06-22; the gist's "violet cursor, orange deferred to 3-10" wording is superseded — the orange treatment is already present here and is correct).
- `was: <old name>` context line in text.detail from m.renameTarget; over-long name truncates with ellipsis.
- Footer `⏎ rename   esc cancel` (key glyphs accent.blue, labels text.detail); dismiss key in footer.
- Enter renames exactly as before (renameAndRefresh unchanged — parity); empty trimmed name is a no-op; Esc cancels.
- Modal renders on the cleared blank screen (inherits 3-1), border-defined with transparent input fill.
- Under NO_COLOR clears to native bg, renders colourless.
- No literal hex at call sites — every colour a §2.9 token.
- VISUAL VERIFICATION: vhs tape drives Sessions → r → rename modal, writes PNG, compared to `Rename Modal (MV)`. Plus LIGHT-MODE eyeball (tick note 1 / §15.6).

STATUS: Complete

SPEC CONTEXT: §8.4 retargets the bare `m.renameInput.View()` to the MV labelled-input panel. The 2026-06-22 corrigendum revised §13.1/§8.4 focus-vs-edit grammar: state is carried by BORDER COLOUR never a fill (grey→violet→orange); a single always-editing input (rename) renders accent.orange border + live cursor + accent.orange `◉ EDIT MODE` badge. §8.4 explicitly notes the orange always-editing treatment is routed through the shared §13.1 input-box helper (renderInputBox). The implementation here ALREADY applies that final treatment — verified correct against the current spec. The rename flow logic (Enter trim→no-op-if-empty→renameAndRefresh, Esc cancel) is a preserved constraint (reskin not rebuild, §1).

IMPLEMENTATION:
- Status: Implemented (matches CURRENT corrigendum-revised spec)
- Location:
  - internal/tui/rename_modal.go:68 renderRenameModalContent — composes header/body/footer through the shared renderJoinedPanel (single-tone border.separator).
  - :81 renameModalHeaderRow — `Rename session` text.primary bold + always-on `◉ EDIT MODE` badge via shared renderHeaderWithBadge (right-aligned).
  - :107 renameModalLabelRow — `NEW NAME` in accent.violet.
  - :122 renameModalInputBoxRows — routes through shared renderInputBox in inputBoxEditing variant (accent.orange rounded outline, transparent interior).
  - :136 renameInputView — value text.primary, accent.orange block cursor, Prompt cleared, blink disabled; NO_COLOR drops all hues to native fg + bare reverse block.
  - :156 renameModalWasRow — `was: <old>` text.detail, ansi.Truncate to box inner width (ellipsis).
  - :171 renameModalFooterRow — `⏎ rename   esc cancel` via shared renderConfirmCancelFooter (accent.blue glyphs, text.detail labels).
  - internal/tui/modal.go:77 renderRenameModalOnClearedCanvas — centres the panel on the cleared owned canvas (inherits 3-1 blank-screen).
  - internal/tui/model.go:3160 handleRenameKey / :3181 updateRenameModal / :3207 renameAndRefresh — rename flow UNCHANGED (verified: Enter trims, empty→no-op, non-empty→renameAndRefresh; Esc→modalNone; other keys delegate to textinput.Update).
- Notes: Behaviour parity holds — updateRenameModal/renameAndRefresh are byte-identical to pre-reskin logic; only Prompt="" was added in handleRenameKey (the label now carries the field name, so the inline prompt would double up). No literal hex at any call site — every colour is a §2.9 theme.MV token. Reuses the shared panel/input-box/header-badge/footer helpers (DRY) rather than re-authoring; consistent with help/kill/edit modals.

TESTS:
- Status: Adequate
- Coverage (internal/tui/rename_modal_test.go):
  - TestRenameModal_ByteExact — full ANSI-stripped layout oracle (frame, badge, label, rounded box, value+cursor cell, was: line, footer). Strong structural guard.
  - TestRenameModal_Header / _NewNameLabel / _InputValue / _WasLine / _Footer — content + per-token SGR-core colour-role assertions, both Dark and Light modes.
  - TestRenameModal_EditModeBadge / _EditModeBadgeRightAligned — badge present, accent.orange, far-right corner with wide flexible gap.
  - TestRenameModal_OrangeBlockCursor — reverse (SGR 7) block + accent.orange hue.
  - TestRenameModal_OrangeInputBoxOutline — value sits inside a rounded box whose outline is accent.orange (rows above/below are box edges).
  - TestRenameModal_SingleToneJoinedPanel — exactly 2 joined ├───┤ dividers, border.separator present, border.footer absent.
  - TestRenameModal_BodyLayout — flush terminal-native order label→box→was.
  - TestRenameModal_Colourless — §2.5 NO_COLOR: copy + frame/box glyphs survive, zero role hues painted.
  - TestRenameModal_LongOldNameTruncates — ellipsis + no row exceeds frame width (edge case).
  - TestRenameModal_NoLitralEnterArrow — footer uses ⏎ not legacy ↵.
  - TestUpdateRenameModal_EnterRenamesNonEmpty / _EnterEmptyIsNoOp / _EscCancels — full parity coverage of the preserved rename flow via a recordingRenamer seam.
- Notes: Coverage is balanced — not under-tested (every acceptance criterion incl. edge cases and NO_COLOR has a dedicated assertion; the byte-exact oracle would fail loudly on any layout regression) and not over-tested (the colour-role tests are narrowly scoped per element; the byte-exact oracle deliberately delegates colour to the other tests). Parity tests assert the seam (old→new trimmed, no-op, cancel) rather than re-testing textinput internals. Tests would fail if the feature broke (e.g. removing the badge fails _ByteExact + _EditModeBadge; changing the cursor hue fails _OrangeBlockCursor). Visual verification artefacts present: rename-modal.tape/.png + rename-modal-light.tape/.png + reference/rename-modal-mv.png — captured PNGs match the reference frame on layout/structure/colour-role (dark and light both eyeballed against owned canvas).

CODE QUALITY:
- Project conventions: Followed. No new *slog.Logger (N/A — pure render). Tokens-not-hex (§2.8) respected everywhere. Reuses shared helpers (renderJoinedPanel/renderInputBox/renderHeaderWithBadge/renderConfirmCancelFooter) — single-source render seams, no drift with sibling modals. Tests avoid t.Parallel(). Idiomatic Go (small focused funcs, []string row model for exact single-line pagination).
- SOLID principles: Good. Single-responsibility funcs (one row each); rendering cleanly separated from the rename flow logic in model.go. Input-box state is an enum shared across inputs/chips (open/closed for the 3-state grammar).
- Complexity: Low. Each helper is a handful of lines with at most one branch (colourless / truncation guard).
- Modern idioms: Yes. ansi.Truncate for ellipsis, lipgloss style composition, strings.Split row model.
- Readability: Good. Doc comments are thorough and tie each element to its spec section.
- Issues: Two stale doc comments still say "violet-outlined input box" where the code (correctly, per corrigendum) renders ORANGE — model.go:3172 ("a violet-outlined input box") and modal.go:74 ("with the violet-outlined input box nested in the body"). The rename_modal.go file header + per-func comments are correct (orange). These are documentation-only drift, no logic impact.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/model.go:3172 — comment says "a violet-outlined input box"; the rename input renders accent.orange (always-editing, per corrigendum). Change "violet-outlined" → "orange-outlined" to match the code.
- [do-now] internal/tui/modal.go:74 — comment "with the violet-outlined input box nested in the body" is stale for the same reason; change "violet-outlined" → "orange-outlined".
