TASK: Compose Painted Frame in View() and Initialise Viewport in NewPreviewModel (preview-visual-distinction-1-8)

ACCEPTANCE CRITERIA:
- pagePreview.View() output contains rounded corners ╭ ╮ ╰ ╯.
- All four edges styled with previewBorderColor.
- View() calls composeChromeLine every tick (no cached field).
- NewPreviewModel initialises viewport with `viewport.New(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead))`.
- First-frame correctness without prior WindowSizeMsg.
- injectSGRResets applied to viewport.View() output before frame composition.
- Degenerate width=2/height=4 without panic.
- No production code outside `internal/tui/pagepreview.go` modified.

STATUS: Complete

SPEC CONTEXT: Spec § Top edge composition > Color application: border parts wrapped in `Foreground(previewBorderColor)`, chrome content with no explicit foreground (terminal default). § SGR reset injection: viewport output passes through injectSGRResets before frame composition. § Initial sizing and preview-open ordering: constructor calls viewport.New with both dimensions reduced by previewFrameOverhead. § Resize behaviour: chrome recomputed every tick.

IMPLEMENTATION:
- Status: Implemented (with architectural improvement).
- Location:
  - `internal/tui/pagepreview.go:287-323` — NewPreviewModel
  - `internal/tui/pagepreview.go:174-202` — composeChromeLineParts returns (left, chrome, right) so View() colours border parts only
  - `internal/tui/pagepreview.go:560-576` — rewritten View(): foreground applied to left/right only, chrome unstyled, body wrapped with `Border(RoundedBorder(), false, true, true, true)` + BorderForeground; injectSGRResets on viewport.View() before body composition.
- Notes: Implementation took the stricter spec path. Shared `selectChromeTier` helper (l.213-247) prevents drift. m.width/m.height read each call — no cached chrome field.

TESTS:
- Status: Adequate
- Coverage (`internal/tui/pagepreview_view_frame_test.go`):
  - `TestPreviewView_FrameContainsAllFourRoundedCorners`
  - `TestPreviewView_TopRowWidthEqualsOuterTerminalWidth`
  - `TestPreviewView_ChromeLineContainsWindowPaneIndicatorsAndWindowName`
  - `TestPreviewView_ChromeContentRenderedWithNoExplicitForegroundSGR` (strict colour-boundary contract)
  - `TestPreviewView_AllFourCornerGlyphsPrecededByForegroundSGR`
  - `TestPreviewView_AppliesSGRResetToEveryNonEmptyViewportRow`
  - `TestPreviewView_FirstFrameCorrectnessAtConstruction`
  - `TestPreviewView_AtDegenerateWidth2Height4RendersWithoutPanic`
  - `TestPreviewView_RecomputesChromeEveryTickNoCachedField`
- TestMain forces TrueColor profile so SGR-byte assertions don't get stripped by no-TTY default.
- No `t.Parallel()`; no `tmuxtest` import.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — DRY via shared `selectChromeTier`; colour policy lives only in View().
- Complexity: Low — View() ~16 lines, linear.
- Modern idioms: Yes.
- Readability: Good — doc comment cites spec sections.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Tier-4 collapse in `composeChromeLineParts` returns the entire row as `left` with chrome/right empty. A one-line comment in View() noting "at tier 4 chrome is empty and left contains the entire row" would make the surface contract self-evident at the call site.
- [idea] No early-return short-circuit for `m.innerWidth() < 2` in View(); lipgloss clips gracefully.
