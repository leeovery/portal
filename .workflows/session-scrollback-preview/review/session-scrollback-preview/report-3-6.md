TASK: session-scrollback-preview-3-6 — Chrome layout integration with viewport sizing

ACCEPTANCE CRITERIA:
- previewModel.View() returns a string containing both chrome line and viewport composed vertically.
- After tea.WindowSizeMsg{Width: W, Height: H}, viewport.Width == W and viewport.Height == H - 1.
- A WindowSizeMsg triggers zero ScrollbackReader.Tail calls.
- Cycling does not change chrome row height.
- Small terminal (height ≤ 2) viewport height is computed defensively (never negative).
- Chrome height is a named constant (previewChromeHeight = 1).

STATUS: Complete

SPEC CONTEXT:
Per § Interaction Shape > Layout: "viewport width = terminal width; viewport height = terminal height minus chrome lines. tea.WindowSizeMsg is forwarded to the embedded viewport so the slice re-flows on resize." Per § Refresh Semantics > Read Trigger Events: "Resize is not a read trigger." Per § Refresh Semantics: "loaded N-line buffer is decoupled from viewport dimensions."

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go
  - View() composes chrome + viewport (line 320-322).
  - WindowSizeMsg sets viewport.Width = msg.Width and viewport.Height = max(0, msg.Height-previewChromeHeight) (line 254-259).
  - previewChromeHeight = 1 named constant (line 16).
  - Defensive max(0, ...) prevents negative dimensions.
  - No reader.Tail call in WindowSizeMsg branch.
  - NewPreviewModel also uses max(0, height-previewChromeHeight) for initial sizing (line 89).

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_layout_test.go (and pagepreview_scroll_test.go)
- Coverage: All five required test names from the plan are present:
  - TestPreviewView_RendersChromeLineAboveViewportContent.
  - TestPreviewWindowSizeMsg_SetsViewportHeightToMsgHeightMinusChrome.
  - TestPreviewResizeDoesNotCallTailAcross100Events.
  - TestPreviewView_ChromeRowCountConstantAcrossTabAndBracketCycles.
  - TestPreviewWindowSizeMsg_SmallHeightDoesNotProduceNegativeViewportHeight.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single source of truth for chrome height constant.
- Complexity: Low.
- Modern idioms: Yes (max() builtin).
- Readability: Strong. Value-receiver style consistent throughout.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None of consequence.
