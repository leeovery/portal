TASK: enter-attaches-from-preview-1-6 — Intercept tea.KeyEnter in previewModel.Update and dispatch pipeline with raw indices

ACCEPTANCE CRITERIA:
- previewModel.Update returns (m, attacher.Run(session, w, p)) for tea.KeyEnter with raw indices from currentRawIndices().
- Enter case returns BEFORE viewport.Update delegation.
- Captured-at-open: raw indices match first group (WindowIndex, PaneIndices[0]).
- Walked: raw indices reflect ]/[/Tab walks.
- Non-contiguous indices: raw tmux values, not slice positions.
- nil attacher → silent no-op.
- Unconditional dispatch regardless of viewport content state.

STATUS: Complete

SPEC CONTEXT:
Spec § Enter binding behaviour mandates intercept-not-forward Enter dispatching the four-call attach pipeline against captured-then-walked (window, pane). Spec § Captured coordinate values pins raw tmux indices. Spec § Mid-load mandates unconditional dispatch regardless of viewport content.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go lines 282-300 (peer to Esc/Home/End/Tab inside tea.KeyMsg switch).
- Line 299: currentRawIndices() yields raw WindowIndex / PaneIndices[paneIdx].
- Line 300: returns (m, m.attacher.Run(session, windowIndex, paneIndex)) — returns BEFORE viewport fallthrough at lines 337-339.
- Lines 296-298: nil-attacher defensive guard returns (m, nil) silently.
- Lines 282-294: godoc explains intercept rationale, raw-indices rationale, unconditional dispatch contract, nil-attacher rationale.
- Update method-level godoc (lines 248-260) updated to mention Enter interception.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_enter_test.go (9 tests, 265 lines).
- Coverage matches the plan's enumerated test list 1:1:
  - TestPreviewEnter_DispatchesWithCapturedRawIndicesWhenNoNavigation
  - TestPreviewEnter_DispatchesWithWalkedIndicesAfterTab
  - TestPreviewEnter_DispatchesWithWalkedIndicesAfterBracket
  - TestPreviewEnter_DispatchesWithRawTmuxIndicesOnNonContiguousSession (WindowIndex=5, PaneIndices=[3] — proves no slice-position math).
  - TestPreviewEnter_NotForwardedToViewport (YOffset before/after equality with recover-on-panic).
  - TestPreviewEnter_NoOpWhenAttacherIsNil
  - TestPreviewEnter_DispatchesWhenViewportHasRealBytes
  - TestPreviewEnter_DispatchesWhenViewportRenderedPlaceholder
  - TestPreviewEnter_DispatchesWhenViewportRenderedReadError
- Local helper newPreviewModelForEnter constructs previewModel directly to unit-isolate the Enter branch.
- No over-testing; viewport-not-forwarded test is a strong drift catcher.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
