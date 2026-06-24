TASK: spectrum-tui-design-7-2 — Remove dead per-modal footer/key-hint wrappers and consolidate the accent.blue key-hint path (tick-ae9099, chore / Phase 7 ANALYSIS-CYCLE).

ACCEPTANCE CRITERIA:
- killModalFooterRow, deleteModalFooterRow, killModalKeyHint, deleteModalKeyHint, renameModalKeyHint no longer exist anywhere in the codebase (grep returns no matches).
- Exactly one shared accent.blue key-hint path exists; editFooterGroup and previewFooterHint either both route through it or are deleted in favour of direct renderKeyHint calls.
- The edit modal footer and the preview footer render byte-identically to before this change.
- go build succeeds and go test ./... passes.
- renameModalFooterRow STAYS (live caller).
- Surviving renderConfirmCancelFooter / renderKeyHint golden cases retained.

STATUS: Complete

SPEC CONTEXT:
Spec §3.4 ("Footer — condensed keymap") and §8.1 ("Modals — shared anatomy") pin the footer key-hint contract: key glyphs render in accent.blue (#7AA2F7 dark / #2E5FD0 light — §13.1 token table), labels in text.detail, the dismiss key lives in the footer as `esc <verb>`. The kill/delete/rename modal footers (`y kill · esc cancel`, `y delete · esc cancel`, `⏎ rename · esc cancel` — §8.4/§8.7) and the preview nav footer (`←→ window ⇥ pane ⏎ attach ␣ back` — §9.1, accent.blue glyph + text.detail label, space-separated) all share this one rendered shape. This chore is pure dead-code-removal + Rule-of-Three consolidation: it must NOT change any rendered bytes, only collapse the five independent re-authorings of "key hint in accent.blue" to one canonical path.

IMPLEMENTATION:
- Status: Implemented (approach (a) — the named-seam variant the task preferred)
- Location:
  - internal/tui/modal_footer.go:53 — new renderBlueKeyHint(key, label, mode, colourless) thin pin over renderKeyHint that fixes keyTok=theme.MV.AccentBlue. This is the single canonical blue-hint seam.
  - internal/tui/edit_modal.go:501 — joinFooterGroups routes the edit footer through renderBlueKeyHint (former editFooterGroup wrapper gone).
  - internal/tui/pagepreview.go:301 — previewFooterFromGroups routes the labelled preview hint through renderBlueKeyHint (former previewFooterHint wrapper gone).
  - internal/tui/rename_modal.go:171 — renameModalFooterRow retained, with live caller at rename_modal.go:71 (renderRenameModalContent); routes through renderConfirmCancelFooter.
  - internal/tui/destructive_confirm.go:149 — destructiveFooterRow is the live kill/delete footer path (→ renderConfirmCancelFooter); confirms the five deleted wrappers had zero production callers.
- Verification (repo-wide grep across internal/ + cmd/): the five named dead wrappers (killModalFooterRow, deleteModalFooterRow, killModalKeyHint, deleteModalKeyHint, renameModalKeyHint) return ZERO matches anywhere — neither production nor test. The two live wrappers (editFooterGroup, previewFooterHint) are also gone, both call sites now route through renderBlueKeyHint. renameModalFooterRow survives with exactly one live caller.
- Notes: Approach (a) was the task's stated preference (named canonical seam over inlined call-site tokens). The modal_footer.go header comment (lines 8-20) was updated to document renderBlueKeyHint as the ONE canonical blue-key-hint path and to record that the five superseded per-modal wrappers were removed — keeping the file-level provenance honest. No drift from plan. Note: editFooterGroups() (plural, edit_modal.go:523) is a DISTINCT live function that resolves the group slice — not to be confused with the removed editFooterGroup (singular) wrapper; correctly left intact.

TESTS:
- Status: Adequate
- Coverage (internal/tui/modal_footer_test.go):
  - TestRenderKeyHint — surviving golden case for the canonical single hint (normal + empty-key label-only path), both modes × colourless carve-out. Retained per task instruction.
  - TestRenderConfirmCancelFooter — surviving golden case covering the live kill/delete/rename footer rows (the destructiveFooterRow / renameModalFooterRow production path), both modes × colourless. Retained per task instruction.
  - TestRenderBlueKeyHint — new golden test that (a) pins the captured pre-refactor bytes AND (b) asserts renderBlueKeyHint == renderKeyHint(..., AccentBlue, ...) on every case, so the canonical blue path's AccentBlue pinning is locked after the wrappers were removed. This is exactly the "add a golden asserting it pins theme.MV.AccentBlue" the task's Tests section called for.
  - TestFooterHintCallSitesByteIdentical — the byte-identity regression: previewFooterHint (←→/window), editFooterGroup/normal (⏎/e edit), editFooterGroup/empty (label-only consequence note), and renameModalFooterRow are each asserted against the SAME pre-refactor goldens through the new shared path — proving zero output drift for the live edit + preview footers (the explicit acceptance criterion) across both modes and colourless.
- Edge cases covered: empty-key label-only fast path (the `empty on save = delete` consequence note), colourless NO_COLOR carve-out, and both theme modes — all present.
- Not over-tested: The five dead wrappers' dedicated sub-tests were removed (no orphaned tests of deleted code remain). The golden constants are content-anchored regression fixtures captured pre-refactor; all 7 const families remain referenced (each Dark/Light/NoCol variant in use — no orphaned consts). The TestRenderBlueKeyHint cases reuse the renderKeyHint goldens rather than duplicating new fixtures, and the dual assertion (golden + AccentBlue-pin) is two genuinely distinct properties, not redundant. No excessive mocking (pure-function render tests, no DI).
- Notes: Test comments accurately record the provenance (captured pre-refactor, "do not regenerate — they ARE the regression contract"), which is the right framing for a no-output-drift chore.

CODE QUALITY:
- Project conventions: Followed. No new *slog.Logger, no t.Parallel(), pure-function helpers consistent with the existing footer/modal render style. Token usage routes through theme.MV.AccentBlue (no raw hex at call site) per the spec's semantic-role-token rule (§13.1). Doc comments are thorough and name the §3.4/§8.x contract.
- SOLID: Good. renderBlueKeyHint is a single-responsibility pin; the consolidation removes the Rule-of-Three violation (five independent re-authorings → one seam) without over-abstracting (it is a 1-line delegation, not a new layer).
- DRY: Resolved — this task's entire purpose. One canonical blue-hint path now; a future edit cannot leave a per-modal clone inconsistent.
- Complexity: Low. Net deletion plus one trivial delegating helper.
- Modern idioms: Yes — idiomatic Go; the thin-wrapper-pins-a-default pattern is appropriate.
- Readability: Good. Updated header comment keeps file provenance accurate.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/rename_modal.go:171 — renameModalFooterRow is a one-line wrapper over renderConfirmCancelFooter with a single live caller (rename_modal.go:71); inlining it at the call site would remove the last thin per-modal footer wrapper, but the task explicitly flags this as optional polish (Do step 5) and it requires a judgment call on whether the named row reads better than an inlined renderConfirmCancelFooter — leave as-is unless a future pass standardises footer call sites.
