TASK: spectrum-tui-design-3-9 — Two-mode edit-project modal: MV render + chip grammar + contextual footers (three frames)

ACCEPTANCE CRITERIA (from tick-b1924e):
- Chips bordered + SQUARE + never filled + never green; grey normal / violet focused (no ✕) / orange editing + cursor (no ✕); removal via x.
- NAME input bordered + ROUNDED + never filled; grey unfocused / violet focused / orange editing + cursor; Enter/e enters edit.
- Focused field label accent.violet; others text.detail.
- ◉ EDIT MODE (accent.orange) only while editing, absent in navigate.
- Three per-mode footers exactly worded with the U+23CE (⏎) enter glyph (not ↵).
- Single bundle (NAME+ALIASES+TAGS); blank-screen; renderJoinedPanel chrome.
- NO_COLOR: state via border + cursor + EDIT MODE text + bold/dim, not colour-only.
- No literal hex; legacy ([x]/Add:/[Enter]/(none)) rendering removed.
- Reusable input-box helper extracted (the 3-10 dependency).
- Fixture + 3 vhs tapes wired; PNGs compared against the three reference frames.
- LIGHT-MODE EYEBALL (traceability review c1 finding 2 / §15.6): all three states rendered in light mode against #e1e2e7 and visually confirmed in a real terminal.

STATUS: Issues Found (1 blocking — a required acceptance deliverable is absent)

SPEC CONTEXT:
The corrigendum (top of spec, 2026-06-22) and the locked tick notes REVISED the §8.2/§13.1 grammar: NOTHING fills, state is carried by BORDER COLOUR (grey border.separator → accent.violet focused → accent.orange editing + cursor); inputs render ROUNDED corners, chips SQUARE; chips drop the inline ✕ (removal is `x` on a focused chip, footer carries `x remove`); chips are text.primary, NEVER state.green (green is attached-only, §2.9 rules); ◉ EDIT MODE is accent.orange, shown only while editing. Footer key glyphs accent.blue, labels text.detail; the enter glyph renders ⏎ (U+23CE) not the §8.2-prose ↵ (tick note 4 — cross-modal consistency). Panel chrome reuses renderJoinedPanel single-tone (tick note 2). The light-mode eyeball is an explicit per-task acceptance criterion (§15.6, deferred from task 1-9).

IMPLEMENTATION:
- Status: Implemented (render layer over the already-committed 3-8 state machine, which is untouched — a clean reskin).
- Location:
  - internal/tui/edit_modal.go — the whole MV render: renderEditProjectContent (198), renderInputBox helper (105), inputBoxState/boxStateFor (69/346), editChipFieldRows (365), chipBoxRows (398), addSlotRows (408), editModalHeaderRow + renderHeaderWithBadge (261/276), editFooterGroups + the three footers (523), renderEditableValue cursor (148).
  - internal/tui/modal.go:90 renderEditModalOnClearedCanvas — blank-screen inheritance via placeModalOnClearedCanvas.
  - internal/tui/panel.go:59 renderJoinedPanel — reused single-tone chrome (border.separator), per tick note 2.
  - internal/capture/fixtures.go:440 projectsFixture — wires fakeProjectEditor + fakeAliasEditor; reproduces reference data (flow-v1-api, aliases [fapi,v1], tags [Fabric,api]); model.go:2367 nil-guards e when editors absent.
  - internal/capture/fakes.go:114/126 fakeProjectEditor / fakeAliasEditor (in-memory no-op).
  - testdata/vhs/edit-modal-{navigate-name,chip-focused,edit-in-place}.tape + .png.
- Notes: Grammar is implemented EXACTLY per the corrigendum. Verified all three captured PNGs against the three reference frames (testdata/vhs/reference/*-mv.png) — layout, structure, and colour-role all match: rounded NAME box, square chips, violet/grey/orange border states, faint `+ add`, violet focused-field label, right-aligned orange ◉ EDIT MODE badge, orange chip border + block cursor while editing, and the three correctly-worded footers with the ⏎ glyph. The reusable renderInputBox helper is genuinely consumed by rename_modal.go:124 (inputBoxEditing variant), so the 3-10 dependency is real, not aspirational. The corrigendum's frame-vs-spec divergences (⏎ over ↵; rounded inputs though Paper draws square; the reference's green `⏎ save` glyph rendered accent.blue per §8.2/§3.4) are all governed by the spec text and correctly resolved toward the spec.

TESTS:
- Status: Adequate (thorough, well-scoped, not bloated).
- Coverage: Three test files. edit_modal_render_byte_exact_test.go pins the full ANSI-stripped layout for navigate-name / chip-focused / editing across the SAME panel width (the resize-bug guard) including footers and badge placement. edit_modal_test.go drives every colour-role assertion in BOTH theme.Dark and theme.Light: header tokens, single bundle, focused-label violet, NAME box grey/violet/orange + cursor + no-fill, chip grey/violet/orange + cursor + no-fill + no-✕ + no-green, `+ add` faint, EDIT MODE only-while-editing + right-aligned, panel-width-stable, all three footers, ⏎-not-↵, no-legacy-grammar, zero-chip edge, brand-new-empty-chip edge, NO_COLOR state-via-border+cursor+text, single-panel-on-cleared-canvas, no-green-ever, plus a byte-exact footer colour oracle. edit_modal_render_tags_test.go covers the TAGS block ordering/chips/empty-slot/live-buffer. assertNoFill correctly excludes the legitimate canvas bg and flags only non-canvas backgrounds. Every acceptance bullet and every listed edge case has a matching deterministic render-oracle test.
- Notes: Not over-tested — the byte-exact oracle and the per-role oracles are complementary (structure vs colour bytes), not redundant. Light-mode is asserted in unit tests for all three states, but a light-mode VISUAL capture (the §15.6 eyeball deliverable) is absent — see BLOCKING.

CODE QUALITY:
- Project conventions: Followed. No literal hex at call sites (every colour a §2.9 theme token; the package-glob colour_literal_guard_test.go covers edit_modal.go automatically). Tokens-only, reusable helper extracted, small functions, doc comments tie each function to the spec section. No t.Parallel(). Idiomatic Go.
- SOLID principles: Good. renderInputBox is a single shared primitive (NAME input + chips + rename), boxStateFor centralises the 3-state resolution, the footer-group/right-align machinery is factored cleanly. Single-responsibility per function.
- Complexity: Low. The only mildly dense function is editChipFieldRows (segment build → transpose → join bands), but it is well-commented and linear.
- Modern idioms: Yes. strings.Builder, []rune cursor handling, lipgloss composition.
- Readability: Good. Self-documenting; intent ties back to spec sections throughout.
- Issues: None at the code level.

BLOCKING ISSUES:
- Missing required acceptance deliverable: the §15.6 LIGHT-MODE EYEBALL criterion (tick-b1924e note, traceability review c1 finding 2) requires all THREE edit-modal states captured in light mode (--appearance light, against #e1e2e7) and visually confirmed. No light-mode artifacts exist for this modal: testdata/vhs/ has edit-modal-{navigate-name,chip-focused,edit-in-place}.tape/.png in DARK only. The established codebase pattern (delete-project-modal-light, kill-confirm-modal-light, rename-modal-light each ship a -light.{tape,png} mirror) was not followed for the edit modal. Light-mode rendering IS unit-tested (every render test iterates theme.Light), but the spec criterion is explicitly a real-terminal visual eyeball, not a unit assertion — so the deliverable is the three light tapes (mirror rename-modal-light.tape with `--appearance light`). The orchestrator runs the captures, so the missing items are the three tapes.

NON-BLOCKING NOTES:
- [bug] internal/tui/edit_modal.go:365 (editChipFieldRows) / panel.go:59 (renderJoinedPanel) — the panel auto-sizes contentWidth to the widest row, so an extreme chip field (a very long chip value or many chips) GROWS the panel and can exceed the terminal canvas width; the task's edge note asks the modal primitive to "wrap or truncate so the panel stays within the canvas." No truncation/wrap of an over-wide chip band exists. Never triggers on the fixed reference data (2 short chips/field) and is a latent edge, not a present defect — fix by clamping/eliding the chip band to the canvas-bounded content width.
- [idea] internal/tui/edit_modal.go:46 (editAddSlot) — addSlotRows renders the `+ add` slot accent.violet when focused in navigate mode, but the spec/corrigendum field-navigation focus model is the violet BORDER on a bordered box; the `+ add` slot is a bare faint text slot (no box), so its focused state is signalled by recolouring the text violet rather than a border. This reads fine in the capture, but worth deciding whether a focused `+ add` should adopt a bordered/box treatment for grammar consistency, or stay a recoloured bare slot (current). Decision, not a mechanical edit.
