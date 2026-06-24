TASK: spectrum-tui-design-6-2 — Consolidate the kill / delete-project destructive-confirm modals behind one parameterised builder (tick-2ea052)

ACCEPTANCE CRITERIA:
- A single renderDestructiveConfirm (or destructiveConfirmSpec + one renderer) owns the destructive-confirm panel render; both kill and delete content functions supply only their distinct data.
- The delete modal's extra project-path body row is expressed as data passed to the shared renderer, not as a forked render path.
- The body-width const (52), the triangle glyph, the state.red title/target colour role, the text.detail consequence colour, and the y verb · esc cancel footer shape are each defined in exactly one place.
- The modal update/state logic (updateKillConfirmModal, updateDeleteProjectModal) is unchanged.
- go build succeeds and the internal/tui test package passes.
- The rendered kill and delete modal content is byte-identical to the pre-refactor output (header, target name, separator, consequence wrap, path row for delete, footer) in both light and dark modes and under the colourless carve-out.

STATUS: Complete

SPEC CONTEXT:
§8.3 (kill confirm) and §8.6 (delete-project confirm) describe the same destructive-confirm element: a state.red `▲ <Title>` header, the target name in state.red, a text.detail consequence line, and a `y <verb> · esc cancel` footer. §8.6 explicitly says it "mirrors the kill modal's destructive treatment" with two deltas — distinct title/verb/consequence copy and an extra body row (the project path in text.detail). The confirm LOGIC is preserved (a reskin), keymap drops `n`. This task is the analysis-cycle DRY consolidation of the two near-verbatim render paths into one parameterised builder; acceptance is byte-identical render plus the consolidation landing.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/destructive_confirm.go:1-152 (new) — owns the shared grammar: destructiveConfirmSpec struct (50-69), renderDestructiveConfirm (84-89), destructiveHeaderRow (94-100), destructiveBodyRows (105-114), destructiveNameRow (120-128), destructiveConsequenceRows (134-143), destructiveFooterRow (149-151). Single-sourced consts: destructiveTitleGlyph="▲" (32), destructiveBodyWidth=52 (37), destructiveKeyCancel/destructiveLabelCancel (41-42).
  - internal/tui/kill_modal.go:40-50 — renderKillModalContent now only assembles a spec (title/targetName/nameTrailer=window count/consequence/verb) and calls renderDestructiveConfirm. Kill data consts at 17-28; killWindowCount pluraliser at 54-60.
  - internal/tui/delete_modal.go:44-54 — renderDeleteModalContent assembles a spec and passes the project path via extraBodyRows (deleteModalPathRow, 60-63), NOT a forked path. Delete data consts at 19-31.
- Notes:
  - All five old per-modal helpers (killModalHeaderRow, deleteModalHeaderRow, killModalConsequenceRows, deleteModalConsequenceRows, the per-modal footer/key-hint wrappers and the duplicated glyph/width consts) are removed — a repo-wide grep returns zero references in production or test code. No orphaned code introduced.
  - delete's path row is correctly DATA: deleteModalPathRow pre-styles the row (text.detail, ansi.Truncate to body-width 52 for the §8.6 over-long-path edge), and the spec carries it in extraBodyRows; destructiveBodyRows (107) splices it between the name row and the single canvas separator with no branching on modal identity.
  - kill's `· N window(s)` count is carried as nameTrailer (same-line trailer) — distinct shape from delete's below-name extraBodyRows, both expressed as pure data on the one spec. Clean separation of the two deltas.
  - Single-sourcing verified: body-width 52, "▲", state.red (via headerStyle(theme.MV.StateRed,...).Bold), text.detail consequence, and the footer shape (delegated to 6-1's renderConfirmCancelFooter in modal_footer.go) each appear in exactly one place.
  - Composition goes through the shared renderJoinedPanel (destructive_confirm.go:88) — the same panel frame help/rename/edit modals use. No bespoke panel drawing.
  - update/state logic untouched: the 6-2 commit (9ea8b9c8) touched only delete_modal.go, destructive_confirm.go (new), kill_modal.go, and the test file — model.go (updateKillConfirmModal at 3125, updateDeleteProjectModal at 2337) was not in the diff.

TESTS:
- Status: Adequate
- Coverage (internal/tui/destructive_confirm_test.go):
  - TestKillDeleteModalContent_ByteIdenticalGolden (28-79) — the heart of the consolidation: pins renderKillModalContent and renderDeleteModalContent against goldens captured from the PRE-refactor render, across dark / light / dark-colourless / light-colourless. Directly proves byte-identical render in both modes + the NO_COLOR carve-out, covering header, name, separator, consequence wrap, the delete path row, and footer.
  - TestRenderDestructiveConfirm_KillSpec (84-112) — shared renderer fed a kill spec (no extra body rows), all four mode/colourless combos, against the same goldens.
  - TestRenderDestructiveConfirm_DeleteSpec (117-147) — shared renderer fed a delete spec WITH the path extraBodyRows (re-styled per mode/colourless inside each case so the path row's own colour role is exercised), all four combos.
  - TestDestructiveConsequenceRows_WordWrapAt52 (153-195) — asserts the factored word-wrap at the single body-width 52 reproduces the known kill/delete line-splitting and that every wrapped line stays within body width. Directly pins the §8.3/§8.6 break points.
- Notes:
  - The goldens are genuine pre-refactor captures (comment 10-14 + commit message confirm capture-before-refactor), so the byte-identical regression is real, not a tautology against post-refactor output.
  - Colourless goldens are correctly shared across dark/light (all hue dropped) — no redundant duplicate literals.
  - The NoCol goldens visibly carry the surviving destructive signal (`\x1b[1m▲\x1b[m` bold glyph) under NO_COLOR, confirming the glyph+bold destructive-emphasis-without-colour invariant (§2.2) holds.
  - Slight overlap between the *ModalContent goldens and the *Spec goldens (same want literals), but it is intentional and load-bearing: the content tests prove the public entry points didn't drift, the spec tests prove the shared renderer reproduces them directly. Not over-tested — each test pins a distinct seam.

CODE QUALITY:
- Project conventions: Followed. Small, single-purpose render helpers; table-driven subtests (golang-testing); no t.Parallel(); DI-free pure render functions threading (mode, colourless) consistent with the rest of internal/tui. Doc comments lead with the role and cite the relevant spec sections.
- SOLID principles: Good. destructiveConfirmSpec is a clean data/render split — SRP honoured (data assembly in kill/delete files, rendering in destructive_confirm.go, state logic untouched in model.go). Open/closed: a third destructive modal would add a spec + call, no edits to the renderer.
- Complexity: Low. Each helper is a handful of lines, one linear loop (the word-wrap), no branching on modal identity in the shared path.
- Modern idioms: Yes. Idiomatic Go — pre-sized make for the wrapped-rows slice, append-based body assembly, ansi.Wordwrap / ansi.Truncate from the established x/ansi dep.
- Readability: Good. The package-level comment block (11-26) and the per-function ASCII layout sketches make the compartment grammar self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The tick's own follow-up note — retarget 6-1's modal_footer_test.go goldens off the production-dead killModalFooterRow/deleteModalFooterRow/killModalKeyHint/deleteModalKeyHint wrappers and delete them — is already resolved: those wrappers were removed in the subsequent task 7-2 commit 26ba5b80 ("remove dead modal footer/key-hint wrappers"), and a repo-wide grep confirms zero remaining references. Nothing left to action for 6-2.)
