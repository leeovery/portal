TASK: Extract innerWidth/innerHeight methods on previewModel (preview-visual-distinction-3-1)

ACCEPTANCE CRITERIA:
- Add innerWidth() and innerHeight() value-receiver methods on previewModel.
- Single source of truth for `max(0, dim − previewFrameOverhead)` arithmetic that previously appeared in WindowSizeMsg handler, View()'s chrome composer, and chromeLineAtModelWidth test helper.

STATUS: Complete

SPEC CONTEXT: Spec (§ Resize behaviour, § Initial sizing, § Top edge composition) repeats `max(0, dim − 2)` across three sites. Cycle-2 analysis flagged the duplicated arithmetic as a DRY risk.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/pagepreview.go:360-374` (method defs); usage sites at 472-473 (WindowSizeMsg handler), 562 (View), and `pagepreview_helpers_test.go:62` (chromeLineAtModelWidth).
- Notes:
  - Methods are value-receiver, consistent with rest of previewModel's method surface.
  - Both methods clamp via `max(0, dim − previewFrameOverhead)` matching original inline arithmetic byte-for-byte.
  - NewPreviewModel constructor still uses inline `max(0, width-previewFrameOverhead)` for viewport.New call (line 308) — intentional, called out via inline comment (lines 304-307): m.width / m.height not yet assigned inside composite literal.
  - All previous `- 2` inline occurrences eliminated outside constructor (grep confirms).
  - innerHeight() currently consumed only by WindowSizeMsg handler; added as peer to innerWidth() for symmetry.

TESTS:
- Status: Adequate
- Coverage: Methods exercised indirectly by pagepreview_resize_test.go (27-31), pagepreview_scroll_test.go (151-157), pagepreview_layout_test.go (56-62, 133-135), pagepreview_helpers_test.go:62 (chromeLineAtModelWidth routes through m.innerWidth()).
- Notes: No dedicated direct unit test — acceptable. Adding one would be over-testing for a one-liner whose contract is "delegate to max(0, dim − const)".

CODE QUALITY:
- Project conventions: Followed. Value-receiver, no t.Parallel(), no tmuxtest.
- SOLID: Good — single responsibility per method.
- Complexity: Low — single expression.
- Modern idioms: Yes — Go 1.21+ built-in `max`.
- Readability: Good. Doc comments cite rationale. Inline comment at constructor explains exception.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Constructor inline arithmetic could be replaced with local var (`innerW := max(0, width-previewFrameOverhead)`) computed before composite literal. Pure style call.
- [idea] innerHeight() currently used only in WindowSizeMsg handler. Cheap optionality if View() ever needs inner height for vertical-degradation handling.
