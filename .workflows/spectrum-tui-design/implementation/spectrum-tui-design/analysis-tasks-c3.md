---
topic: spectrum-tui-design
cycle: 3
total_proposed: 7
---
# Analysis Tasks: spectrum-tui-design (Cycle 3)

## Task 1: Collapse BootstrapProgressMsg's duplicate friendly-label computation to a single authority
status: approved
severity: medium
sources: architecture

**Problem**: The cold-boot progress channel ships a friendly `Label` (and raw `StepName` `Name`) on every `BootstrapProgressMsg`. The producer computes `ev.Label` in `cmd/bootstrap` and copies it onto the message (`cmd/bootstrap_progress.go:250`), but the only consumer — the model's `BootstrapProgressMsg` Update arm — folds the event through `LoadingProgress.Apply`, which reads ONLY `Index`/`RestoreN`/`RestoreM` and re-derives the friendly label from `Index` via `stepLabelTable`/`LabelForStep`. Nothing in `internal/tui` or its tests reads `msg.Label` or `msg.Name`. This is two parallel encodings of the identical §10.4 11-step→5-label mapping (producer-side in `cmd/bootstrap`, consumer-side in `loading_progress.go`) kept in sync by nobody, so they can silently drift; the wire format carries a structurally-dead field. The `model.go` doc still describes `Label`/`Name` as "task 5-4 / 5-3 placeholder" fields, confirming they were meant to be wired but the design landed on consumer-side derivation without removing the producer-side path. This undermines the documented "SINGLE SOURCE OF TRUTH" intent of `loading_progress.go`.

**Solution**: Pick one authority. Since `loading_progress.go` is the documented single source and is transport-free / independently testable, drop `Label` (and `Name`, if no consumer needs the raw `StepName`) from `BootstrapProgressMsg` and from the producer in `bootstrap_progress.go:247-254`, leaving `Index` as the stable key the consumer maps. If a consumer genuinely needs `Name` later, keep `Name` but delete the redundant `Label` so the friendly mapping exists in exactly one place.

**Outcome**: The §10.4 step→label mapping exists in exactly one place (`loading_progress.go`). `BootstrapProgressMsg` carries only the fields a consumer actually reads; no dead/duplicated label travels the wire, and the producer/consumer mappings can no longer drift.

**Do**:
1. Read `internal/tui/model.go:178-184` (`BootstrapProgressMsg` definition + doc), `cmd/bootstrap_progress.go:247-254` (producer copying `ev.Label`/`Name` onto the message), and `internal/tui/loading_progress.go:84-114` (`LoadingProgress.Apply` consuming `Index`/`RestoreN`/`RestoreM` only).
2. Confirm no production or test code in `internal/tui` reads `msg.Label`; grep the tree for `.Label` / `.Name` references against `BootstrapProgressMsg` to confirm `Name` has no remaining consumer.
3. Remove the `Label` field from `BootstrapProgressMsg`; remove `Name` as well if step 2 confirms no reader, otherwise retain only `Name`.
4. Remove the producer-side computation/assignment of the now-dropped field(s) in `bootstrap_progress.go:247-254` (and the friendly-label derivation that fed it, if it has no other use in `cmd/bootstrap`).
5. Update the `model.go` doc comment that describes `Label`/`Name` as placeholder fields to reflect the final field set.
6. Adjust any tests that constructed `BootstrapProgressMsg` with the removed field(s).

**Acceptance Criteria**:
- `BootstrapProgressMsg` no longer carries a `Label` field; `Name` is retained only if a consumer reads it.
- The §10.4 11-step→5-label mapping exists in exactly one location (`loading_progress.go`); no second authoritative copy is produced or transmitted.
- The producer in `bootstrap_progress.go` no longer computes/assigns the dropped field(s).
- The loading page renders the identical 5-label progression as before for the full 11-step cold-boot sequence (including the restore N/M sub-steps).

**Tests**:
- Existing loading-progress unit tests still pass and continue to assert the §10.4 label progression derived from `Index`.
- A grep/compile-level check (or existing guard) confirms no remaining reference to the removed field(s).
- If `BootstrapProgressMsg` construction appears in tests, those compile and pass against the reduced field set.

## Task 2: Switch Sessions/Projects footer to §3.4 ⏎/␣ glyphs (spec-owner ratified path b)
status: approved
severity: low
sources: standards

**DECISION (spec owner, ratified): PATH (b) — switch to glyphs.** Change the Sessions/Projects footer `Key` forms to `⏎`/`␣` and the nav key to `↑↓` (no slash) so the footer matches §3.4 verbatim AND the Preview footer convention. Update the keymap descriptor (`internal/tui/keymap.go`), `footer.go` if it special-cases the words, and the byte-exact `footer_test.go` assertion to the glyph form. This is a VISUAL change — RE-CAPTURE every affected fixture (all pages rendering the footer: sessions-flat / by-project / by-tag / empty / no-tags-signpost, projects, projects-command-pending, filtering-* + light/nocolor variants) and verify each against its committed reference frame (which per §3.4 should already show the glyphs). Do NOT touch the Preview footer (already glyphs). Path (a) — amending §3.4 to words — was NOT chosen; ignore it below.

**Problem**: §3.4 specifies the condensed Sessions footer "exactly" as `↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` plus right-aligned `? help`. The keymap descriptor stores the footer `Key` forms as the literal words "enter" and "space" (the help-modal `HelpKey` overrides them to ⏎/␣), and the footer renders `Key` directly — so the live footer reads `enter attach` and `space preview`, not the spec's `⏎ attach` / `␣ preview`. The footer's nav key is also rendered as `↑/↓` (with a slash) vs the spec's `↑↓`. The divergence is documented in-code as a deliberate descriptor decision ("the footer keeps 'enter'/'space'"), and the byte-exact footer test (`footer_test.go:41`) enshrines the word form, so this was a conscious choice rather than an oversight. It is nonetheless a literal divergence from §3.4's verbatim footer copy and from the Preview footer convention, which DOES use glyphs (`⏎ attach`, `␣ back` in §9.1 / `previewKeymap`). **This finding recurred in prior cycles and was previously discarded as a visual-gate decision; per the orchestrator it is surfaced this cycle for the spec owner to ratify (word-vs-glyph footer choice + §3.4 wording) rather than silently discarded.** Low severity: cosmetic, fully legible, internally consistent across Sessions/Projects.

**Solution**: This is a spec-owner ratification, not an executor's choice. Two paths: (a) ratify the word form by amending §3.4 to read `enter attach … space preview` (and note the slash-nav `↑/↓` form), so the spec and the byte-exact test agree; or (b) switch the Sessions/Projects footer `Key` forms to the ⏎/␣ glyphs (and `↑↓` for nav) to match §3.4 verbatim and the Preview footer convention. Prefer (a) if the word form tested better in the §15 in-terminal validation; the choice belongs to the spec owner.

**Outcome**: The spec owner has ratified one form. Either §3.4 + the byte-exact footer test agree on the word form, or the implementation + test use the ⏎/␣ glyphs to match §3.4 verbatim and the Preview footer convention. The Sessions/Projects footer and the spec are no longer in literal disagreement.

**Do**:
1. Route this item to the spec owner for a word-vs-glyph decision (this is a visual/spec-copy ratification gate, not a code-only change).
2. If path (a) — ratify words: amend §3.4 in the specification to read `enter attach … space preview` and note the `↑/↓` slash-nav form; leave the keymap descriptor, `footer.go` render, and `footer_test.go:41` as-is.
3. If path (b) — switch to glyphs: change the Sessions/Projects footer `Key` forms in the keymap descriptor (`internal/tui/keymap.go:88-90`, `:128`) to ⏎ / ␣ and the nav key to `↑↓`; update `internal/tui/footer.go:64-66` if it special-cases the words; update the byte-exact assertion in `internal/tui/footer_test.go:41` to the glyph form.
4. Whichever path, ensure the spec, the rendered footer, and the byte-exact test all agree afterward.

**Acceptance Criteria**:
- The spec owner has chosen (a) or (b); the chosen form is reflected consistently in the spec §3.4, the keymap descriptor / `footer.go` render, and the byte-exact `footer_test.go` assertion.
- After the decision, no residual disagreement remains between §3.4's verbatim footer copy and the live Sessions/Projects footer.
- The Preview footer convention (⏎/␣ glyphs) is left intact regardless of path.

**Tests**:
- `footer_test.go`'s byte-exact footer assertion matches the ratified form and passes.
- If path (b), a render-level test confirms the Sessions/Projects footer shows ⏎/␣ glyphs (and `↑↓` nav) matching §3.4.

## Task 3: Model the no-matches footer membership structurally instead of by magic label string
status: approved
severity: low
sources: architecture

**Problem**: `noMatchesFooterEntries` derives the §7.3 reduced footer by copying `filteringFooterEntries()` and dropping the entry whose `Label == "browse results"` (`internal/tui/filtering_no_matches.go:80`). The correspondence is held by an exact-string match against a label defined in a different file (`internal/tui/filter_footer.go:62`). If the input-active footer's "browse results" copy is ever reworded, the filter silently stops removing the entry and the no-matches footer regains a browse-results hint pointing at zero results — a correctness regression with no compile-time or obvious-test signal. This is correctness-by-string-coincidence across a file boundary, the same class of fragility the codebase otherwise avoids (the keymap descriptors model footer membership as a Core flag rather than label matching).

**Solution**: Model the membership structurally rather than by display text — e.g. have `filteringFooterEntries` build from a small set where the browse-results entry is identifiable without its display string (a flag, or compose the no-matches set directly from the shared `type`/`esc` entries) so a copy change cannot break the filter.

**Outcome**: The §7.3 no-matches footer is derived without depending on the display text of the "browse results" entry. Rewording the input-active footer copy can no longer silently break the no-matches footer's filtering.

**Do**:
1. Read `internal/tui/filter_footer.go:56-66` (`filteringFooterEntries`, where the "browse results" entry is defined) and `internal/tui/filtering_no_matches.go:76-86` (`noMatchesFooterEntries`, the label-string drop).
2. Choose a structural model: either tag the browse-results entry with an identifying flag (mirroring the keymap descriptor's Core-flag membership pattern), or compose `noMatchesFooterEntries` directly from the shared `type`/`esc` entries rather than copy-and-filter.
3. Implement so `noMatchesFooterEntries` no longer references the `"browse results"` display string.
4. Verify the rendered §7.3 no-matches footer is byte-identical to its current output.

**Acceptance Criteria**:
- `noMatchesFooterEntries` no longer matches on the `"browse results"` label literal (no cross-file display-string coupling remains).
- The §7.3 no-matches footer renders exactly as before (browse-results hint absent).
- Rewording the input-active footer's "browse results" copy does not change which entries the no-matches footer contains.

**Tests**:
- A unit test asserts the no-matches footer excludes the browse-results entry, written so it would still pass if the browse-results display copy were changed (i.e. it does not itself depend on the literal).
- Existing filtering-footer render tests continue to pass with byte-identical output.

## Task 4: Remove dead SessionItem/ProjectItem Title()/Description() (or derive the attached marker from the const)
status: approved
severity: low
sources: architecture

**Problem**: The custom `SessionDelegate`/`ProjectDelegate` render rows entirely through `renderSessionRow` / `renderRowLine`; only `FilterValue()` is consumed off the items by `bubbles/list`. `SessionItem.Title()`/`Description()` and `ProjectItem.Title()`/`Description()` are referenced only by tests now (no production caller). That alone is harmless leftover, but `SessionItem.Description()` additionally hard-codes the string `label + "  ● attached"` — duplicating both the attached-marker glyph/text and its two-space spacing — independently of the `attachedMarker` const (`internal/tui/session_item.go:57`) and the token-styled render in `renderSessionRow`. The two have already diverged in form (the live render builds a fixed-width column-aligned slot; `Description()` builds a bare two-space concatenation), and the literal `"● attached"` sits outside the centralised marker constant. The colour-literal guard cannot catch it because it is a plain string, so a future change to the marker text would silently leave `Description()` stale — a latent inconsistency seam: a dead method re-authoring a piece of the row vocabulary instead of deriving from the const.

**Solution**: Either delete the now-dead `Title()`/`Description()` methods on both items (updating the tests that exercise them to assert the live render path instead), or, if they are retained as the item's logical text projection, have `Description()` reuse the `attachedMarker` const rather than re-spelling `"● attached"` so the marker text lives in one place.

**Outcome**: The attached-marker text exists in exactly one place (`attachedMarker`). No production-dead method re-authors a stale copy of the row vocabulary; either the dead methods are gone or they derive from the const.

**Do**:
1. Read `internal/tui/session_item.go:124-138` (`Title`/`Description`), `:57` (`attachedMarker` const), the live `renderSessionRow` attached-marker render, and `internal/tui/project_item.go:33-41`.
2. Confirm via grep that no production code calls these `Title()`/`Description()` methods (only tests).
3. Decide deletion vs derivation:
   - Deletion: remove `Title()`/`Description()` on both `SessionItem` and `ProjectItem`; update the tests that exercised them to assert the live render path (`renderSessionRow` / `renderRowLine`) instead.
   - Derivation: keep the methods but rewrite `SessionItem.Description()` to build the attached marker from the `attachedMarker` const rather than the literal `"● attached"`.
4. Run the affected `internal/tui` tests.

**Acceptance Criteria**:
- The literal `"● attached"` no longer appears in `SessionItem.Description()` (either the method is removed or it derives from `attachedMarker`).
- No production-dead `Title()`/`Description()` remains carrying a hand-spelled copy of row vocabulary; if methods are kept they derive from the centralised const.
- The live attached-marker render path (`renderSessionRow`) is unchanged.

**Tests**:
- Tests that previously exercised `Title()`/`Description()` are updated to assert the live render path (deletion) or to confirm the marker is sourced from `attachedMarker` (derivation), and pass.
- The session/project row render tests continue to pass with unchanged output.

## Task 5: Point loading_view.go at the shared header.go leaf canvas-style helpers
status: approved
severity: low
sources: duplication

**Problem**: The loading screen independently re-implements the exact "role-token foreground over `Background(canvas)` for the mode, bare style under NO_COLOR" leaf-style pair that `header.go` already exports and that the rest of the TUI shares. `loadingFg(fg, mode, colourless)` (`internal/tui/loading_view.go:249-265`) is byte-identical in behaviour to `headerStyle(fg, mode, colourless)`, and `loadingStyle(mode, colourless)` is byte-identical to `headerCanvasBg(mode, colourless)` (`internal/tui/header.go:87-104`). Every other surface authored in this work unit — `section_header.go`, `footer.go`, `filter_footer.go`, `notice_band.go`, the destructive/rename/edit/help modal files — routes through the `header.go` pair; `loading_view.go` is the one render file that forked its own copy (the §10 loading-phase executor did not see the §3 header helpers). This is the canonical copy-paste-across-task-boundary pattern: two names for one concept that must move together if the NO_COLOR carve-out or the canvas-background composition ever changes.

**Solution**: Delete `loadingStyle`/`loadingFg` and point the loading render at the shared `headerStyle`/`headerCanvasBg` (or, if the loading code wants its own terse names, make them one-line aliases that delegate, the way `SessionDelegate.rowBg`/`rowToken` delegate to the shared free functions). No behaviour change — the bodies are already identical. This collapses the third independent copy of the leaf-paint rule into the single `header.go` source.

**Outcome**: The leaf canvas-paint rule (role-token fg over canvas Background, bare under NO_COLOR) lives in exactly one place (`header.go`). The loading render shares it; any future change to the NO_COLOR carve-out or canvas composition is made once.

**Do**:
1. Read `internal/tui/loading_view.go:249-265` (`loadingStyle`, `loadingFg`) and `internal/tui/header.go:87-104` (`headerStyle`, `headerCanvasBg`) and confirm the bodies are behaviourally identical.
2. Replace `loading_view.go`'s call sites of `loadingFg`/`loadingStyle` with `headerStyle`/`headerCanvasBg`, OR convert `loadingFg`/`loadingStyle` into one-line delegating aliases of the header helpers (mirroring the `SessionDelegate.rowBg`/`rowToken` delegation pattern).
3. Delete the forked bodies of `loadingStyle`/`loadingFg` (whether removed outright or reduced to delegating one-liners).
4. Confirm the loading screen renders byte-identically in light, dark, and NO_COLOR modes.

**Acceptance Criteria**:
- `loading_view.go` no longer contains an independent re-implementation of the header leaf-paint pair; it routes through `headerStyle`/`headerCanvasBg` (directly or via thin delegating aliases).
- The leaf canvas-paint rule exists in exactly one authoritative source (`header.go`).
- The loading screen renders identically (light / dark / NO_COLOR) to current output.

**Tests**:
- Existing loading-view render tests pass with byte-identical output across light, dark, and NO_COLOR.
- If aliases are kept, a test (or compile-level check) confirms they delegate to the header helpers rather than re-implementing.

## Task 6: Extract a single cleared-canvas modal placement helper
status: approved
severity: low
sources: duplication

**Problem**: Five of the six `render*ModalOnClearedCanvas` wrappers in `internal/tui/modal.go:67-135` (`renderModalOnClearedCanvas`, `renderHelpModalOnClearedCanvas`, `renderKillModalOnClearedCanvas`, `renderDeleteModalOnClearedCanvas`, `renderRenameModalOnClearedCanvas`, `renderEditModalOnClearedCanvas`) are structurally identical: build a panel string from per-modal content, then `return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)`. They differ only in which content builder is called (`renderHelpModalContent` / `renderKillModalContent` / `renderDeleteModalContent` / `renderRenameModalContent` / `m.renderEditProjectContent`). The shared "centre this already-built panel on the cleared canvas" step is the §8.1/§13.5 blank-screen placement, repeated verbatim five times — one copy per modal accreted across the 3-4/3-5/3-6/3-7/3-8/3-9 reskin tasks. The risk is low (single trivial line) but it is the same parallel-wrapper smell the team already collapsed elsewhere (`destructive_confirm.go`, `modal_footer.go`).

**Solution**: Introduce one `placeModalOnClearedCanvas(panel string, width, height int) string` returning the `lipgloss.Place` result, and have each `render*OnClearedCanvas` wrapper call it with its built panel — or, more aggressively, have each modal's call site build its panel and call the single placement helper directly, dropping the per-modal wrappers entirely. Keeps the centring maths (and any future change to it, e.g. a non-centred placement) in exactly one place.

**Outcome**: The §8.1/§13.5 cleared-canvas centring placement exists in exactly one helper. Each modal supplies only its content/panel; the centring maths is no longer copied five times.

**Do**:
1. Read `internal/tui/modal.go:67-135` and confirm the five identical wrappers reduce to "build panel → `lipgloss.Place(width, height, Center, Center, panel)`", differing only in the content builder.
2. Add `placeModalOnClearedCanvas(panel string, width, height int) string` returning the `lipgloss.Place` result.
3. Either (preferred-minimal) have each `render*ModalOnClearedCanvas` wrapper build its panel then return `placeModalOnClearedCanvas(panel, width, height)`, or (more aggressive) drop the per-modal wrappers and call the helper from each modal's call site after it builds its panel.
4. Confirm each modal (help, kill, delete, rename, edit) still centres identically on the cleared canvas.

**Acceptance Criteria**:
- The `lipgloss.Place(... Center, Center ...)` cleared-canvas centring appears in exactly one place (`placeModalOnClearedCanvas`).
- All affected modals (help, kill, delete, rename, edit) render byte-identically to current output.
- No per-modal wrapper re-implements the centring line.

**Tests**:
- Existing modal render tests for help / kill / delete / rename / edit pass with byte-identical output.
- A test (or shared assertion) confirms the modals route through the single placement helper.

## Task 7: Extract the right-anchored footer row assembler shared by footer.go and filter_footer.go
status: approved
severity: low
sources: duplication

**Problem**: `footerKeyRow` (`internal/tui/footer.go:140-165`) and `filterFooterRow` (`internal/tui/filter_footer.go:130-152`) implement the same row geometry: render a left cluster, render the right-aligned `? help` anchor, then "if the right anchor doesn't fit beside the cluster (`leftWidth+1+rightWidth > w`) pad-left-and-return, else emit cluster + canvas flex spacer + right anchor". The narrow-degrade branch and the spacer computation are byte-equivalent; only the cluster renderer (`renderFooterCluster` vs `renderFilterCluster`) and the right-anchor source differ. `filter_footer.go`'s own comments acknowledge it "mirrors footerKeyRow's right-anchor layout" — the mirroring is the duplication. The two diverged across the standard-footer task (3-1/3-2) and the filter-footer task (2-8/7), so a future change to the right-anchor degrade rule must be made in both.

**Solution**: Extract the right-anchor row assembler — `assembleRightAnchoredRow(left string, leftWidth int, rightSeg string, rightWidth, w int, mode, colourless) string` owning the fit test, the `headerPadRight` degrade, and the flex-spacer join — and have both `footerKeyRow` and `filterFooterRow` call it after rendering their own left cluster and resolving the shared right `? help` anchor. The left-cluster fitting (`fitLeftCluster`'s ellipsis logic) stays footer.go-specific; only the final right-anchor layout merges.

**Outcome**: The right-anchored footer row geometry (fit test + narrow-degrade + flex-spacer join) lives in one assembler shared by the standard and filter footers. A future change to the right-anchor degrade rule is made once.

**Do**:
1. Read `internal/tui/footer.go:140-165` (`footerKeyRow`) and `internal/tui/filter_footer.go:130-152` (`filterFooterRow`) and confirm the fit-test / `headerPadRight` degrade / flex-spacer branches are byte-equivalent.
2. Add `assembleRightAnchoredRow(left string, leftWidth int, rightSeg string, rightWidth, w int, mode, colourless) string` owning the fit test, the `headerPadRight` narrow-degrade, and the flex-spacer join.
3. Refactor `footerKeyRow` to render its left cluster (keeping `fitLeftCluster`'s ellipsis logic footer.go-specific), resolve the right `? help` anchor, then call `assembleRightAnchoredRow`.
4. Refactor `filterFooterRow` likewise to render its own left cluster and call the shared assembler.
5. Confirm both footers render byte-identically across wide and narrow widths (including the narrow-degrade boundary `leftWidth+1+rightWidth > w`).

**Acceptance Criteria**:
- The fit test, `headerPadRight` narrow-degrade, and flex-spacer join exist in exactly one assembler (`assembleRightAnchoredRow`); neither `footerKeyRow` nor `filterFooterRow` re-implements them.
- The left-cluster fitting/ellipsis logic remains footer.go-specific (not merged).
- Both the standard and filter footers render byte-identically to current output at wide widths and at/below the narrow-degrade boundary.

**Tests**:
- Existing footer and filter-footer render tests pass with byte-identical output.
- A test exercises the narrow-degrade boundary (`leftWidth+1+rightWidth > w`) for both footers and confirms identical degrade behaviour through the shared assembler.
