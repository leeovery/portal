TASK: Wire tea.WindowSizeMsg Handler and Delete chromeLine() Method (preview-visual-distinction-1-7)

ACCEPTANCE CRITERIA:
- tea.WindowSizeMsg handler sizes viewport to max(0, msg.Width-previewFrameOverhead), max(0, msg.Height-previewFrameOverhead)
- m.width and m.height recorded on every resize
- chromeLine() method no longer exists in `internal/tui/pagepreview.go`
- View() compiles
- go test ./internal/tui/... passes
- Clamp test asserts viewport.Width == 0 and viewport.Height == 0 at msg.Width=1, msg.Height=0

STATUS: Complete

SPEC CONTEXT: Spec § Resize behaviour: WindowSizeMsg handler must record dimensions and adjust viewport to (W-2, H-2) clamped non-negative. Spec § Code shape changes > Replace chromeLine() with composeChromeLine: chromeLine() method on previewModel deleted; callers invoke pure composeChromeLine.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/pagepreview.go:461-474` (WindowSizeMsg handler)
  - chromeLine() deletion verified by grep returning zero hits for `func \(m previewModel\) chromeLine`
  - `internal/tui/pagepreview_helpers_test.go:41-43` — chromeLineForTest shim replacing the deleted method for in-package chrome tests
- Notes: Handler uses direct field assignment (m.viewport.Width = m.innerWidth(); m.viewport.Height = m.innerHeight()) rather than viewport.SetSize. Inline comment documents bubbles@v1.0.0 lacks SetSize. Inner-dimension arithmetic refactored into innerWidth()/innerHeight() helpers (lines 365-374) — single source of truth shared with View() and NewPreviewModel. Plan called for a temporary stub View(); implementation skipped the stub and landed full task-1-8 View() directly. Benign.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/pagepreview_resize_test.go` covers (1) recording width/height and asserting viewport dimensions == msg − previewFrameOverhead at 100×30, and (2) clamp boundary at 1×0 → viewport 0×0. `pagepreview_layout_test.go:57-62, 134-135` carries equivalent assertions with updated arithmetic.
- Notes: chromeLine() deletion enforced at compile time. Existing chrome substance tests redirected through chromeLineForTest (calls composeChromeLine(200, ...)).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: innerWidth/innerHeight extracted as SSoT helpers.
- Complexity: Low — handler body is five lines.
- Modern idioms: Yes — Go 1.21+ built-in max() used directly.
- Readability: Good — inline rationale for direct field assignment. Constructor exception comment at pagepreview.go:304-307 explains why innerWidth/innerHeight cannot be used inside composite literal.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] chromeLineForTest pins width=200 and calls composeChromeLine directly. Pre-existing chrome substance tests now exercise the pure function through this shim. Cascade-e2e test (1-9) covers this gap end-to-end. Future cleanup could retire chromeLineForTest in favour of direct composeChromeLine call sites.
- [idea] If bubbles is upgraded to a version exposing viewport.SetSize, the WindowSizeMsg site should switch so YOffset auto-clamping is engaged. A TODO-on-upgrade comment would make the upgrade path explicit.
