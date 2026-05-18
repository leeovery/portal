TASK: Extract firstLine test helper for top-row extraction idiom (preview-visual-distinction-3-2)

ACCEPTANCE CRITERIA:
- A package-scoped `firstLine` test helper exists alongside other shared preview-test helpers.
- All in-package preview test sites that previously hand-rolled `strings.IndexByte(s, '\n')` for top-row extraction now delegate to the helper.
- No behavioural change to tests; helper is pure.

STATUS: Complete

SPEC CONTEXT: Phase 3 housekeeping driven by Analysis Cycle 2. Maintainability refactor surfaced by review of Phase 1 test corpus. Phase 1 acceptance criteria unaffected.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Definition: `internal/tui/pagepreview_helpers_test.go:45-53`
  - Call sites migrated:
    - `pagepreview_view_frame_test.go:48` (TopRowWidthEqualsOuterTerminalWidth)
    - `pagepreview_view_frame_test.go:86` (ChromeContentRenderedWithNoExplicitForegroundSGR)
    - `pagepreview_view_frame_test.go:188` (FirstFrameCorrectnessAtConstruction)
    - `pagepreview_cascade_e2e_test.go:138` (tier 4 top-edge pattern)
    - `pagepreview_view_routing_test.go:64` (chrome-on-first-line routing assertion)
- Notes: Helper co-located in canonical `pagepreview_helpers_test.go` hub. Signature `func firstLine(s string) string` — no `t *testing.T` (cannot fail). Grep for prior inline pattern `IndexByte(..., '\n')` returns only the helper's own definition. The `LastIndexByte` at `pagepreview_view_frame_test.go:131` is a different semantic operation and correctly left alone.

TESTS:
- Status: Adequate (helper is test infra; correctness exercised transitively).
- Coverage: Newline-present branch exercised by all five migrated tests; no-newline path is documented safe fallback.
- Notes: No standalone unit test for `firstLine` — over-testing would not be appropriate for a 5-line helper.

CODE QUALITY:
- Project conventions: Followed. Test-only file, unexported, no `t.Parallel()`, no `tmuxtest`.
- SOLID: N/A at this scale; single clear responsibility.
- Complexity: Minimal (one branch).
- Modern idioms: Idiomatic Go (`strings.IndexByte` for single-byte search).
- Readability: Good. Godoc comment states contract and purpose.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
