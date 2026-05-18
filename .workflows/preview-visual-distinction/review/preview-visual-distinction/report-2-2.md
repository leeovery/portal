TASK: Promote newFramePreviewModelAt to Shared Preview-Model Test Helper (preview-visual-distinction-2-2)

ACCEPTANCE CRITERIA:
- The 5-line construction idiom no longer appears verbatim in pagepreview_resize_test.go or pagepreview_cascade_e2e_test.go.
- Three call sites use newFramePreviewModelAt.
- newFramePreviewModelAt accessible at package scope.
- go test ./internal/tui/... passes.

STATUS: Complete

SPEC CONTEXT: Cycle-1 duplication finding flagged that a 5-line preview-model construction idiom was open-coded in three places. Remediation is purely test-side: promote the helper to the shared helpers file and migrate the duplicating sites.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/pagepreview_helpers_test.go:65-91` (canonical home for both newFramePreviewModel and newFramePreviewModelAt with docstrings)
  - `internal/tui/pagepreview_resize_test.go:17,41` (both open-coded constructions now call newFramePreviewModelAt)
  - `internal/tui/pagepreview_cascade_e2e_test.go:149` (in-loop construction now calls newFramePreviewModelAt)
  - `internal/tui/pagepreview_view_frame_test.go` no longer hosts the helper definition
- Notes: Helper is package-scoped. Migration matches the analysis task's plan exactly.

TESTS:
- Status: Adequate
- Coverage: Test-only refactor; migrated call sites are themselves the tests, continuing to exercise the same resize/cascade contracts. Suite passes.
- Notes: Helper signature (t, windowName, payload, width, height) is minimal. `t.Fatalf` on !ok preserved inside the helper.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(), no tmuxtest. Constructor-injected mocks. Helper file naming matches existing layout.
- SOLID: Good. Single-responsibility helper; default-dim wrapper delegates to explicit-dim helper.
- Complexity: Low. 6-statement function.
- Modern idioms: Yes — t.Helper() marker on both helpers.
- Readability: Good. Docstrings distinguish the two helpers.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] The wider _test.go corpus still has many open-coded `stubEnumerator{}` + `recordingReader{}` + `NewPreviewModel` triples (e.g. pagepreview_brandnew_test.go, pagepreview_error_test.go). Out of scope for this cycle-1 task; future sweep could unify, possibly via a variant taking a groups slice.
