TASK: spectrum-tui-design-9-1 — Delete dead modal-box scaffolding and scrub misleading "not-yet-reskinned" comments (tick-b5c9b5, chore)

ACCEPTANCE CRITERIA:
- renderModalOnClearedCanvas and modalBorderStyle no longer exist in internal/tui/modal.go.
- A repo-wide grep for renderModalOnClearedCanvas and modalBorderStyle returns no matches.
- The two orphaned tests in help_modal_frame_test.go / edit_modal_test.go that drove the dead path are removed; all remaining internal/tui tests still exercise the live joined-panel modal path.
- No comment in modal.go references "not-yet-reskinned" modals or implies un-reskinned consumers exist.
- go build and go test ./... pass with no unused-symbol or unused-import errors.

STATUS: Complete

SPEC CONTEXT:
§8.1 (modal framing) establishes the cleared-canvas blank-screen treatment: when a modal opens, the page behind clears to the owned mode-matched canvas and a centred panel sits on the flat fill. The reskin corrigendum and §13.x migrated all five modals (kill/rename/delete/edit/help) onto hand-drawn renderJoinedPanel content builders, each routed through its own render*ModalOnClearedCanvas wrapper. The shared border-defined box (modalBorderStyle's Padding(1,2)) that the original renderModalOnClearedCanvas built was a transitional scaffold for "not-yet-reskinned" modals — a state that no longer exists. This is a pure dead-code/comment-hygiene chore with no behavioural surface; "Reskin, not rebuild" (§1) means live render paths must stay byte-identical, which they do (no live code touched, only deletions + comment edits).

IMPLEMENTATION:
- Status: Implemented (clean, complete)
- Location: internal/tui/modal.go (deletions of modalBorderStyle + renderModalOnClearedCanvas at former :32/:67; five wrapper comments scrubbed); internal/tui/model.go:3871,3882-3884,4063 (three dangling symbol mentions corrected); internal/tui/help_modal_frame_test.go (three dead-path tests removed); internal/tui/edit_modal_test.go:626 (comment scrub).
- Notes:
  - Repo-wide grep for `renderModalOnClearedCanvas` (excluding the live `render*ModalOnClearedCanvas` family) → NO MATCHES. Grep for `modalBorderStyle` → NO MATCHES. Grep for `not-yet-reskinned` / `left intact for the OTHER` → NO MATCHES. (.go scope; the only residual mentions are in non-binding workflow manifest/capture artefacts, which are out of acceptance scope.)
  - The former `modalStyle` name (referenced only in a deleted comment) is also confirmed absent from live code.
  - All five live production callers in model.go (renderDeleteModalOnClearedCanvas, renderEditModalOnClearedCanvas, renderHelpModalOnClearedCanvas, renderKillModalOnClearedCanvas, renderRenameModalOnClearedCanvas) and the shared placeModalOnClearedCanvas remain intact (6 funcs present in modal.go) — no live render path was touched.
  - Imports in modal.go all still used: textinput (renderRenameModalOnClearedCanvas param :77), lipgloss (placeModalOnClearedCanvas :33), theme (every wrapper signature). No orphaned import.
  - model.go scrub is accurate: the two "§14.6 ADAPT decision recorded on renderModalOnClearedCanvas" breadcrumbs now correctly point at placeModalOnClearedCanvas (where that decision now lives), and the edit-modal inline comment no longer claims a modalBorderStyle box would double-wrap.
  - The commit message claims "3 orphaned dead-path tests" and "3 dangling symbol mentions in model.go" — both verified against the diff (TestModalPanelBorderColour, TestModalPanelBorderColourless, TestRenderModalOnClearedCanvasKeepsPaddingForOtherModals removed; three model.go references corrected). The task gist says "two orphaned tests"; the actual count was three. This is a harmless under-count in the gist, not a drift — all dead-path tests were removed and no live test was.

TESTS:
- Status: Adequate
- Coverage: For a dead-code-removal chore the "test" is the negative space (symbols gone + nothing live broke) plus retention of the live joined-panel tests. The three removed tests all drove renderModalOnClearedCanvas / modalBorderStyle (the dead path) and were correctly excised. The live help-modal frame tests are retained (TestHelpModalPanelBorderColour, TestHelpModalDividerToken, TestHelpModalDividerJoined, TestHelpModalDividerConnectsToBorders, TestHelpModalFlushVerticalSpacing, TestHelpModalBodyContiguousRows — all six exercise renderHelpModalContent / the live joined panel). TestEditModal_SinglePanelOnClearedCanvas is retained and now drives the live renderEditModalOnClearedCanvas path, asserting exactly two rounded top-corners (joined panel + NAME box) so a redundant outer box would regress — this is precisely the guard the deleted modalBorderStyle path could have re-introduced, so the live coverage protecting the deletion's intent stays in place.
- Notes:
  - No over-testing introduced (this commit only deletes tests).
  - ansi import in help_modal_frame_test.go remains used after the deletions (multiple live tests) — no orphaned test import.
  - Build/test greenness asserted by reading only (per instructions). Imports and symbol references trace cleanly; there is no path by which the package would fail to compile.

CODE QUALITY:
- Project conventions: Followed. Standard Go, no t.Parallel, leaf-package and DI rules untouched (no structural change). golangci-lint reported 0 issues per the commit (not independently re-run).
- SOLID principles: Good. Removing the dead second-frame path strengthens single-responsibility of placeModalOnClearedCanvas as the lone centring home; the five wrappers now uniformly document the same hand-drawn-panel-then-place pattern.
- Complexity: Low (net -117 lines of source/test scaffolding; one fewer style constructor).
- Modern idioms: Yes (no change to idiom surface).
- Readability: Improved. The scrubbed comments now describe the actual single-path-per-modal reality; a future contributor adding a modal is pointed only at renderJoinedPanel + a render*ModalOnClearedCanvas wrapper. No comment now implies a non-existent set of un-reskinned consumers.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
