TASK: spectrum-tui-design-6-1 — Extract one shared footer key-hint helper and collapse the parallel footer-group types (analysis cycle / DRY consolidation)

ACCEPTANCE CRITERIA (from tick-911013):
- Exactly one function renders the <key/glyph> <label> footer-hint shape; the five per-site clones (killModalKeyHint, deleteModalKeyHint, renameModalKeyHint, previewFooterHint, editFooterGroup) all route through it (or are removed in favour of direct calls).
- Exactly one struct type models the {Key/Glyph, Label} footer-group concept; footerGroup and previewFooterGroup unified.
- Exactly one function renders the confirm/cancel two-hint footer row; the three modal footer-row functions route through it.
- The empty-key (label-only) case is handled inside the shared key-hint helper, output identical to prior editFooterGroup behaviour.
- go build succeeds and the full internal/tui test package passes.
- Rendered footer/modal-footer output unchanged at every call site in light and dark modes and under the colourless/NO_COLOR carve-out.

STATUS: Complete

SPEC CONTEXT:
- §3.4 (footer): key glyphs render accent.blue, labels text.detail, ? glyph accent.violet, over a 1px border.footer rule. Sessions Core keys "↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects" + right-aligned ? help.
- §8.1 (modal framing) / §8.3 / §8.6 / §8.4: kill footer "y kill · esc cancel", delete footer "y delete · esc cancel", rename footer "⏎ rename · esc cancel" — key glyphs accent.blue, labels text.detail. The spec's "·" separator between the two hints is realised as the fixed three-space ("   ") canvas gap (modalFooterGap), pinned by the captured goldens.
- This is a Phase 6 analysis-cycle consolidation: acceptance is byte-identical render + the extraction landing. Verified against CURRENT code (tasks 7-2 removed dead wrappers, 8-7 extracted assembleRightAnchoredRow).

IMPLEMENTATION:
- Status: Implemented (consolidation genuinely landed; in places goes beyond the task's letter, consistently).
- Location:
  - internal/tui/modal_footer.go:38 renderKeyHint — the SINGLE author of the <key> <label> shape (key in keyTok over canvas, single canvas gap, label text.detail, JoinHorizontal). Empty-key early return (modal_footer.go:40-42) renders the label-only fast path.
  - internal/tui/modal_footer.go:53 renderBlueKeyHint — thin accent.blue pin over renderKeyHint; the single accent.blue contextual-hint seam.
  - internal/tui/modal_footer.go:62 renderConfirmCancelFooter — confirm hint + fixed modalFooterGap ("   ", modal_footer.go:73) + cancel hint, both via renderKeyHint.
  - internal/tui/modal_footer.go:26 footerHintGroup — the SINGLE unified {key,label} struct, explicitly replacing footerGroup + previewFooterGroup.
- Call-site routing (all confirmed):
  - footer.go:281 renderFooterEntry → renderKeyHint (left-cluster + ? help entries).
  - pagepreview.go:301 previewFooterFromGroups → renderBlueKeyHint; previewFooterGroups (pagepreview.go:245) returns []footerHintGroup.
  - edit_modal.go:501 joinFooterGroups → renderBlueKeyHint; editFooterGroups (edit_modal.go:523) returns []footerHintGroup; the empty-key consequence note {"", "empty on save = delete"} (edit_modal.go:529) collapses onto renderKeyHint's label-only path.
  - rename_modal.go:172 renameModalFooterRow → renderConfirmCancelFooter (renameKeyConfirm/Label + renameKeyCancel/Label constants).
  - destructive_confirm.go:150 destructiveFooterRow → renderConfirmCancelFooter; the kill (kill_modal.go:46) and delete (delete_modal.go:50) modals feed confirmKey/confirmLabel through destructiveConfirmSpec → renderDestructiveConfirm → destructiveFooterRow.
- Old symbols fully gone from production code: grep for footerGroup/previewFooterGroup/killModalKeyHint/deleteModalKeyHint/renameModalKeyHint/previewFooterHint/editFooterGroup/killModalFooterRow/deleteModalFooterRow finds ZERO production definitions or references (only historical mentions in comments + test names). No residual inline key-hint shape outside the canonical files.
- Notes on going beyond the task's letter (benign, not drift):
  - The task named killModalFooterRow + deleteModalFooterRow as two of the "three modal footer-row functions". In current code those two were collapsed (by adjacent destructive-confirm consolidation) into a single shared destructiveFooterRow, which still routes through renderConfirmCancelFooter. The acceptance criterion ("the three modal footer-row functions route through it") is satisfied — all three destructive/rename footer paths converge on renderConfirmCancelFooter. This is additional DRY, fully consistent with the task's intent.
  - renderBlueKeyHint is an extra named pin not literally listed in the task's "Do" steps, but it is exactly the "callers default keyTok to accent.blue" convenience the description anticipates, and it is independently tested.

TESTS:
- Status: Adequate.
- Coverage (internal/tui/modal_footer_test.go):
  - TestRenderKeyHint (modal_footer_test.go:69): table-driven, normal key+label and empty-key label-only, both modes × colourless true/false — 8 cases against captured pre-refactor goldens.
  - TestRenderBlueKeyHint (modal_footer_test.go:104): same matrix, AND a pin-assertion that renderBlueKeyHint == renderKeyHint(...AccentBlue...) (proves the token is fixed, not just that the golden matches).
  - TestRenderConfirmCancelFooter (modal_footer_test.go:139): kill (y/esc), delete (y/esc), rename (⏎/esc) × both modes × colourless — 12 cases against goldens.
  - TestFooterHintCallSitesByteIdentical (modal_footer_test.go:175): pins the LIVE call-site shapes (preview ←→/window, edit ⏎/e, the empty consequence note, renameModalFooterRow) against the same pre-refactor goldens, asserting the reroute is byte-identical.
  - footer_test.go independently covers the descriptor-driven Sessions/Projects footer (token colours, NO_COLOR carve-out, narrow-degrade) — the renderFooterEntry → renderKeyHint path is exercised end-to-end there.
- The goldens are raw SGR byte strings captured from the PRE-refactor renders (header comment modal_footer_test.go:9-16 documents this and forbids regeneration). This is the correct regression contract for a "byte-identical output" acceptance criterion — it would fail if the key colour, label colour, gap width, or canvas painting drifted.
- Not under-tested: empty-key path, both modes, and the colourless carve-out are all explicitly covered, matching the spec's edge cases.
- Not over-tested: each test verifies a distinct seam (helper shape / blue pin / confirm-cancel row / call-site reroute). Some golden constants are reused across TestRenderKeyHint and TestRenderBlueKeyHint, but that is shared fixtures, not redundant assertions. No excessive mocking; pure-function string assertions.
- Minor observation (non-blocking, see notes): TestFooterHintCallSitesByteIdentical asserts the preview/edit reroutes via renderBlueKeyHint("←→","window",...) rather than calling the live previewFooterFromGroups / joinFooterGroups producers. Because previewFooterHint/editFooterGroup were deleted, the byte-identity is asserted against the shared helper the producers also call, which is sound — but it does not pin that the producers actually thread the right key/label into the helper. The footer_test.go suite covers the producer threading for the Sessions/Projects path; the preview/edit producer threading is covered by their own page tests (not in scope here).

CODE QUALITY:
- Project conventions: Followed. Idiomatic Go, small focused functions, no t.Parallel, package-level theme tokens; the DRY/extraction golang-design-patterns guidance is exactly what this task executes (well past Rule of Three).
- SOLID: Good. Single-responsibility helpers; renderBlueKeyHint correctly composed over renderKeyHint rather than duplicating the token; renderConfirmCancelFooter composed over renderKeyHint.
- Complexity: Low. renderKeyHint is a guard + three styled segments; renderConfirmCancelFooter is three segments; no branching beyond the empty-key fast path.
- Modern idioms: Yes.
- Readability: Good — thorough doc comments tie each helper to its §spec contract and explicitly record what was superseded (the removed per-modal wrappers, the unified struct). The "single canonical place" intent is unmistakable.
- DRY: The duplication the task targeted is eliminated; the convention now lives in exactly one place (renderKeyHint) for the hint and one place (renderConfirmCancelFooter) for the row.
- Security / performance: N/A — pure render-string composition, no I/O.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/modal_footer_test.go:187 — the previewFooterHint/editFooterGroup byte-identity sub-tests assert through renderBlueKeyHint directly rather than driving the live producers (previewFooterFromGroups / joinFooterGroups). Consider whether a thin assertion that the live producers thread the expected key/label into the helper belongs here (decide whether the existing page-level coverage already suffices, given the helper itself is golden-pinned).
