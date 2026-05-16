TASK: enter-attaches-from-preview-3-2 — Extract Shared Preview-Teardown Helper for Dismiss + Bail Handlers

ACCEPTANCE CRITERIA:
- Single `exitPreviewToSessions` helper exists on `*Model` and is called from both handlers
- Esc-dismiss behaviour unchanged
- Bail behaviour unchanged (still uses `msg.Session` for preserveName)
- No behavioural change visible to tests

STATUS: Complete

SPEC CONTEXT: Phase 3 / Analysis Cycle 1 cleanup. The dismiss + bail handlers had ~identical three-step teardown preludes (set PageSessions, zero m.preview, dispatch refresh cmd). DRY consolidation; pure refactor with no behavioural delta.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:732-746 — helper definition
  - internal/tui/model.go:969-973 — dismiss handler captures `m.preview.session` BEFORE calling helper
  - internal/tui/model.go:990-992 — bail handler uses `msg.Session`, then setFlash + tea.Batch(refresh, tick)
- Notes: Only one `m.preview = previewModel{}` mutation site in the package; helper is the single source of truth.

TESTS:
- Status: Adequate (via existing handler tests)
- Coverage: dismiss path covered by `pagepreview_dismiss_test.go`; bail path covered by `preview_attach_bail_test.go` and `preview_attach_bail_flash_test.go`. Both directly exercise the helper through the public Update path. Regression test `TestEscDismissPathUnchangedAfterBailHandlerAdded` confirms parity.
- Notes: No dedicated unit test on `exitPreviewToSessions` (task marked this optional).

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good — SRP teardown helper; both callers compose extra behaviour around it.
- Complexity: Low — three-line body.
- Modern idioms: Yes.
- Readability: Excellent — docstring explicitly calls out the capture-before-call hazard.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `exitPreviewToSessions` mutates the receiver AND returns a cmd — fine for two internal callers, but if a third caller emerges, splitting into a pure transition helper plus a refresh-cmd factory would reduce footgun surface. (Restated for trace continuity from report-1-7.)
