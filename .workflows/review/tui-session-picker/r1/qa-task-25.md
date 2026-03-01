TASK: Replace ANSI-unaware placeOverlay with lipgloss.Place (tick-d2056e)

ACCEPTANCE CRITERIA:
- placeOverlay function is removed from internal/tui/modal.go
- renderModal uses lipgloss layout functions for positioning
- All existing modal-related tests pass without modification
- Modal overlays center correctly over styled list content

STATUS: Complete

SPEC CONTEXT: The specification (Modal System section) states: "lipgloss.Place() positions styled content over the list output in View()." The original placeOverlay used raw rune counting, causing misalignment with ANSI-styled background content. The task description anticipated that lipgloss.Place might not support compositing over existing content and provided a fallback path (Do step 2): use ANSI-aware width measurement via lipgloss.Width() instead of len([]rune(...)).

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/modal.go:51-86
- Notes: The old placeOverlay function is fully removed (grep confirms zero hits in .go files). The replacement renderModal uses lipgloss.Width() and lipgloss.Height() for ANSI-aware measurement of the styled modal, and charmbracelet/x/ansi functions (ansi.StringWidth, ansi.Truncate, ansi.TruncateLeft) for ANSI-aware line compositing. This follows the task's Do step 2 path since lipgloss.Place() does not support compositing a foreground over an existing background -- it only places content in whitespace. The approach is correct: it centers the modal via lipgloss-measured offsets, then composites each foreground line over the background using ANSI-aware truncation. No raw rune counting remains. The renderListWithModal helper (lines 31-46) provides dimension fallback (80x24) and delegates to renderModal.

TESTS:
- Status: Adequate
- Coverage:
  - "overlay contains modal content and list view" -- verifies modal content and background are both present
  - "overlay is non-empty string" -- basic smoke test
  - "overlay differs from plain list view" -- ensures compositing modifies the output
  - "modal content has border styling" -- verifies lipgloss border characters present
  - "modal centers correctly over ANSI-styled background" -- KEY test for this task: builds a background with raw ANSI escape sequences, verifies rune count differs from display width as a precondition, renders the modal, then measures the display-column offset of the top-left border character to confirm correct centering
- Notes: The ANSI centering test is well-designed. It validates the precondition (rune length != display width), constructs a full 24-line styled background, and measures the border position using ansi.StringWidth -- exactly what the task's Tests section requested. Existing modal integration tests in model_test.go do not call renderModal directly (they test through the full View() pipeline), so they are unaffected by the refactor.

CODE QUALITY:
- Project conventions: Followed -- Go idioms, proper function documentation, unexported helpers
- SOLID principles: Good -- renderModal has a single responsibility (compositing), renderListWithModal separates dimension fallback from compositing
- Complexity: Low -- straightforward loop over foreground lines, clear left/fg/right compositing
- Modern idioms: Yes -- uses max() builtin (Go 1.21+), charmbracelet/x/ansi library for ANSI-aware string operations
- Readability: Good -- clear variable names (fgLines/bgLines, overlayX/overlayY, left/right), well-commented logic
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The task title says "lipgloss.Place" but the implementation correctly uses manual ANSI-aware compositing because lipgloss.Place does not support overlay-on-background. The task's Do step 2 explicitly anticipated this and the implementation follows that path. This is not drift -- it is the expected fallback.
- The acceptance criterion "renderModal uses lipgloss layout functions for positioning" is met via lipgloss.Width() and lipgloss.Height() for measurement, even though lipgloss.Place() itself is not used for the actual compositing step.
