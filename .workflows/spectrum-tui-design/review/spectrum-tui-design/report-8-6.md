TASK: spectrum-tui-design-8-6 — Extract a single cleared-canvas modal placement helper (DRY consolidation chore)

ACCEPTANCE CRITERIA:
- The lipgloss.Place(... Center, Center ...) cleared-canvas centring appears in exactly one place (placeModalOnClearedCanvas).
- All affected modals (help, kill, delete, rename, edit) render byte-identically to current output.
- No per-modal wrapper re-implements the centring line.
- Tests: existing modal render tests pass byte-identical; a test confirms routing through the single placement helper.

STATUS: Complete

SPEC CONTEXT:
§8.1 (modal framing — shared): modals render on a blank screen cleared to the owned mode-matched canvas, with the modal centred on it. §13.5 restates this (page behind cleared to owned canvas, not a dimmed overlay; Preview is the exception). The centring step this task consolidates is exactly the §8.1/§13.5 "centre the built panel on the cleared canvas" placement. The chore is a pure DRY consolidation flagged from the analysis cycle (duplication source, severity low) — five wrappers had copied the same lipgloss.Place line verbatim across the 3-4…3-9 reskin tasks.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/modal.go:32-34 (placeModalOnClearedCanvas helper) — the sole lipgloss.Place(width, height, Center, Center, panel) call. Wrappers: renderHelpModalOnClearedCanvas (44-47), renderKillModalOnClearedCanvas (54-57), renderDeleteModalOnClearedCanvas (65-68), renderRenameModalOnClearedCanvas (77-80), renderEditModalOnClearedCanvas (90-93) — each builds its own panel via its distinct content builder (renderHelpModalContent / renderKillModalContent / renderDeleteModalContent / renderRenameModalContent / m.renderEditProjectContent) then returns placeModalOnClearedCanvas(panel, width, height). Call sites: model.go:3833,3840,3844 (Projects: delete/edit/help) and model.go:4025,4032,4036 (Sessions: kill/rename/help).
- Notes: The "preferred-minimal" Do-step 3 path was taken (wrappers retained, each delegating to the helper) rather than the aggressive "drop wrappers" variant — a sound choice since the wrappers carry meaningful per-modal doc comments and isolate the content-builder signatures. The generic legacy renderModalOnClearedCanvas named in the tick description has been removed (grep confirms only the five named wrappers + the helper remain) — consistent with the note that task 9-1 stripped dead modal-box scaffolding. Verified against current code. No drift from spec or plan: pure refactor, behaviour-preserving, centring maths unchanged.

TESTS:
- Status: Adequate
- Coverage: internal/tui/modal_placement_consolidation_test.go holds two tests. (1) TestPlaceModalOnClearedCanvas_ByteIdenticalToInline — compares the extracted helper against an inline golden reproduction (prePlaceModalOnClearedCanvas) across 5 panel shapes (empty, single char, single line, multi-line, bordered box) x 5 dims (incl. the 80x24 fallback the modals actually use, and a degenerate 0x0), proving zero output drift. (2) TestModalCentringAppearsInExactlyOnePlace — an AST walk of modal.go asserting the exact centring shape lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, <panel>) appears in exactly one function body and that function is placeModalOnClearedCanvas; isClearedCanvasPlaceCall pins the arg shape (bare width/height idents + lipgloss.Center twice + 5 args), so no wrapper can silently re-copy the line.
- Notes: The byte-identity test directly satisfies the "render byte-identical" criterion at the placement layer (sound — the wrappers differ only in the panel they pass, so identity of the placement function plus unchanged content builders gives full-modal identity transitively). The AST guard is the "routes through the single helper / exactly one place" proof. Well-targeted, no t.Parallel() (correct per the tui package's shared-mutable-state convention). Not over-tested — the per-modal full-render byte-identity is left to the existing per-modal render tests (e.g. help_modal_frame_test.go), avoiding redundant golden duplication here. Not under-tested — both acceptance criteria have a dedicated assertion.

CODE QUALITY:
- Project conventions: Followed. No literal hex; pure Lipgloss styling; no t.Parallel(); AST-guard pattern matches the codebase's existing source-walking guard-test idiom (e.g. the log-ownership guard).
- SOLID principles: Good. Single responsibility cleanly separated — placeModalOnClearedCanvas owns centring only; each wrapper owns content assembly only. The helper is the single point of change for any future placement-maths change (the stated DRY goal).
- Complexity: Low. Helper is a one-line pass-through; wrappers are two lines each.
- Modern idioms: Yes. Idiomatic Go; AST test uses go/parser+go/ast cleanly with small focused predicate helpers (isSelector/isIdent).
- Readability: Good. The helper's doc comment is thorough and explains the why (the §8.1/§13.5 placement, the five-copy accretion, the fillCanvas relationship). Wrapper comments accurately describe each modal's frame.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/modal.go:32-34 — placeModalOnClearedCanvas takes (panel string, width, height int) but every wrapper and call site threads width/height as a pair; consider a small typed {w,h int} content-region value (or reuse an existing dims type if one exists) so the two ints can't be transposed at a call site. Mechanical, low-value; only worth it if a dims type already exists in the package.
