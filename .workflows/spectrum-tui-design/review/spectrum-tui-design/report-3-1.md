TASK: spectrum-tui-design-3-1 — Blank-screen modal layer (shared): clear the page behind an open modal to the owned canvas (mode-matched, NO_COLOR → native bg) + centre the border-defined panel; resolve the §14.6 adapt-vs-rework decision.

ACCEPTANCE CRITERIA (from plan / tick-0a70f0):
- When a modal is open the page behind is cleared to the owned mode-matched canvas (no rows, no list, no dimmed overlay) and the panel is centred on that fill.
- The §14.6 adapt-vs-rework decision is recorded in a code comment at the changed site (default adapt-if-small).
- Confirm/input logic of every modal unchanged (kill y, rename Enter, delete y, edit routes keys) — parity.
- Modals remain key-exclusive — page binds (s/x/n/e/d/clear-filter/quit) do not fire while a modal is open; Esc resolves against the modal first.
- Under NO_COLOR the modal blank-screen clears to native bg (no painted canvas), inherited from the Phase 1 carve-out — not a separate path.
- No full-screen frame around the cleared canvas (§3.6) — backdrop a flat fill, panel border-defined.
- One-row-per-delegate pagination invariant intact on dismissal.
- VISUAL: a vhs tape opens a kill-confirm modal and writes a PNG showing the modal centred on a cleared canvas with no list rows behind.

STATUS: Complete

SPEC CONTEXT:
§8.1 — modals render on a blank screen (changed behaviour); page behind cleared to the owned canvas (mode-matched, not literal black), panel centred, border-defined with NO distinct fill; key-exclusive (Esc resolves against modal first); confirm/input logic preserved (parity). §13.5 restates: cleared to owned canvas, not a dimmed overlay; Preview is the exception. §14.6 (open question) — adapt the existing render path vs a modal-system rework, assess at implementation, logic preserved either way. §2.5 — NO_COLOR paints no canvas (native bg), carve-out applies to the modal blank-screen, inherited not double-branched. §3.6 — no full-screen frame; canvas is a flat fill, not a box.

IMPLEMENTATION:
- Status: Implemented (ADAPT path chosen — the prior renderModal/renderListWithModal/modalStyle overlay-splice code is fully removed; no overlay compositing remains in internal/tui).
- Location:
  - internal/tui/model.go:4009-4037 (viewSessionList) and :3819-3845 (viewProjectList) — when m.modal != modalNone, return ONLY the centred panel sized to the inset content region (contentWidth × contentHeight); list/header/footer chrome is not composed.
  - internal/tui/modal.go:32-34 (placeModalOnClearedCanvas) — the single centring expression: lipgloss.Place(width, height, Center, Center, panel).
  - internal/tui/modal.go:44-93 — five per-modal wrappers (help/kill/delete/rename/edit) each build their own hand-drawn panel and route placement through the single helper.
  - internal/tui/model.go:3418-3451 (fillCanvas) + :3501-3556 (fillColourless / insetColourless) — the Phase 1 outer full-terminal fill paints the owned canvas (or native bg under colourless) around the centred panel, with the 80×24 zero-dims fallback (termDims, :3329).
  - Key-exclusive dispatch: model.go:2251 (updateProjectsPage) and :2936 (updateSessionList) route to updateModal and return BEFORE any page binds; updateModal (:3084-3104) dispatches to per-modal handlers; Ctrl+C is the only pre-empt.
- Notes:
  - Blank-screen achieved by NOT composing the chrome (the page simply isn't rendered while a modal is up) rather than over-painting an existing render — the cleanest possible adapt; the splice mechanic was removed, not patched.
  - No full-screen frame (§3.6): the backdrop is fillCanvas's flat fill; the panel border is the hand-drawn rounded box in panel.go (renderJoinedPanel), border-defined with no distinct fill (panelFrameStyle sets foreground only, no background). Compliant.
  - NO_COLOR is inherited, not double-branched: the modal render path makes no colourless decision of its own; suppression happens entirely in the shared fillCanvas → fillColourless/insetColourless path (model.go:3426). Compliant with the task's "do not add a second NO_COLOR branch".
  - §14.6 decision: the in-body comments at model.go:3826 and :4018 read "§14.6 ADAPT decision recorded on placeModalOnClearedCanvas." The ADAPT choice and its substance (clear-to-canvas by returning only the centred panel and letting fillCanvas paint the backdrop) ARE described at the changed site in those same in-body comment blocks, so the criterion is substantively met. However the pointer is slightly inaccurate: the placeModalOnClearedCanvas doc comment (modal.go:21-31) documents the centring-logic CONSOLIDATION (task 8-6), not the adapt-vs-rework rationale — it does not state "we adapted rather than reworked because the splice path could be dropped for a small chrome-suppression change." See NON-BLOCKING NOTES.

TESTS:
- Status: Adequate
- Coverage:
  - Blank-screen / cleared list — modal_blank_screen_test.go: TestModalBlankScreen_ClearsListRowsBehindModal (sanity-asserts rows present pre-modal, then absent post-modal — proves the clear, not a never-there false pass), TestModalBlankScreen_ProjectsDeleteClearsList (shared change inherited by the Projects delete modal).
  - Centring on terminal dims — TestModalBlankScreen_CentresPanelUsingTerminalDims (frame exactly termW×termH; top border neither first nor last row).
  - Owned-canvas backdrop — TestModalBlankScreen_PaintsOwnedCanvasBackdrop (dark+light; asserts the real canvas SGR via canvasSeq).
  - NO_COLOR native bg — TestModalBlankScreen_ColourlessClearsToNativeBg (neither dark nor light canvas SGR present; panel + clear still hold).
  - Zero-dims fallback — TestModalBlankScreen_ZeroDimsFallback (80×24).
  - Flash-band leak edge — TestModalBlankScreen_NoFlashBandLeaksIntoClearedView.
  - Centring consolidation — modal_placement_consolidation_test.go: TestPlaceModalOnClearedCanvas_ByteIdenticalToInline (byte-identical to the pre-consolidation inline form across panels/dims) + TestModalCentringAppearsInExactlyOnePlace (AST guard: the Place call lives in exactly one function).
  - Confirm/input parity (existing handler tests, stay green) — kill_modal_dispatch_test.go: TestKillConfirm_YConfirmsParity / _EscCancelsClearsBoth / _NIgnored; rename_modal_test.go: TestUpdateRenameModal_EnterRenamesNonEmpty / _EnterEmptyIsNoOp / _EscCancels; edit_modal_state_machine_test.go: TestSM_* family; delete handler covered via dispatch.
  - Key-exclusive — help_modal_test.go: "does not fall through to clear-filter/quit on Esc", "consumes all other keys while open"; TestKillConfirm_NIgnored demonstrates a page-bind-like key not falling through.
  - VHS visual — testdata/vhs/kill-confirm-modal.tape + kill-confirm-modal.png present.
- Notes:
  - One gap vs the task's stated test list: there is no test that directly asserts the list pagination is unperturbed AFTER a modal is dismissed (the criterion "it does not perturb the list pagination when the modal is dismissed"). The current tests prove the modal clears the chrome and that fillCanvas is an outer wrap that doesn't touch the list budget, and the design makes a regression structurally implausible (the list state is untouched while the modal is up — only the View() composition skips it), so this is low-risk and the broad one-row-per-delegate suite elsewhere covers the invariant generally — but a dismissal round-trip assertion is not present. See NON-BLOCKING NOTES [quickfix].
  - Not over-tested: each test targets a distinct facet (clear / centre / canvas / colourless / fallback / flash / consolidation). No redundant bloat.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (package-level mutable state honoured). Small DI seams unaffected. Comments are dense but in-house idiom for this codebase.
- SOLID principles: Good. placeModalOnClearedCanvas is the single centring home (DRY — the five verbatim copies collapsed to one, AST-guarded). Per-modal wrappers each own only their panel builder; the placement and the backdrop fill are orthogonal layers.
- Complexity: Low. The modal branch is a flat switch returning early; no nesting growth.
- Modern idioms: Yes. lipgloss.Place for centring; shared style primitives.
- Readability: Good. The viewSessionList/viewProjectList in-body comments explain the clear-to-canvas approach and the inherited NO_COLOR/fallback.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/modal.go:21-31 — the §14.6 ADAPT pointer comments (model.go:3826, :4018) say the decision is "recorded on placeModalOnClearedCanvas", but that helper's doc comment documents the centring consolidation, not the adapt-vs-rework rationale. Add one or two sentences to the placeModalOnClearedCanvas doc comment (or inline at the two changed sites) stating the actual decision: the existing overlay-splice path (renderModal/renderListWithModal/modalStyle) was ADAPTED — in fact removed in favour of not composing the chrome while a modal is up — because clear-to-canvas needed only chrome suppression + the existing Phase 1 fillCanvas, no modal-system rework. This makes the dangling pointer self-contained and fully satisfies the "record the decision (one or two sentences)" acceptance wording.
- [quickfix] internal/tui/kill_modal_dispatch_test.go (or modal_blank_screen_test.go) — add a dismissal round-trip test asserting the list pagination/selection is unperturbed after a modal opens and is dismissed (open kill modal, Esc, then re-render viewSessionList and assert the rows/section header return and the selected index is preserved) to close the one explicitly-listed task test ("it does not perturb the list pagination when the modal is dismissed") that has no direct assertion today.
