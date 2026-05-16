TASK: enter-attaches-from-preview-1-7 — Handle previewAttachBailMsg in top-level Model.Update with placeholder bail (transition + refresh, no flash). Phase 2 task 2-5 later extended this handler with flash dispatch; verification scope is that the placeholder transition+refresh core still holds.

ACCEPTANCE CRITERIA:
- Model.Update has `case previewAttachBailMsg` flipping activePage to PageSessions, zeroing m.preview, returning the refresh cmd.
- Bail handler reads session name from msg.Session (not m.preview.session).
- Existing previewDismissedMsg handler unchanged in observable shape.
- previewSessionsRefreshedMsg handler source-agnostic.
- (Phase 1 only) No flash — superseded by Phase 2 task 2-5.
- Refresh cmd non-nil when lister wired; nil tolerated when unwired.

STATUS: Complete

SPEC CONTEXT: Spec § Session-killed-externally bail path > Behaviour mandates bail dispatches a refresh-and-bail action: (1) transition pagePreview → pageSessions (same as Esc); (2) trigger existing sessions-list refresh; (3) emit inline flash (Phase 2). Phase 1's placeholder lands transition+refresh half.

IMPLEMENTATION:
- Status: Implemented (placeholder core preserved across Phase 2 flash overlay).
- Location:
  - internal/tui/model.go:974-992 — `case previewAttachBailMsg`. Calls `m.exitPreviewToSessions(msg.Session)` then `m.setFlash(...)`, returns `tea.Batch(refreshCmd, flashTickCmd(m.flashGen))`.
  - internal/tui/model.go:742-746 — `exitPreviewToSessions` shared helper called by both dismiss and bail. Performs `m.activePage = PageSessions`, `m.preview = previewModel{}`, returns `m.refreshSessionsAfterPreviewCmd(preserveName)`.
  - internal/tui/model.go:953-973 — `case previewDismissedMsg` (Esc) handler — same observable shape after shared-helper refactor.
  - internal/tui/model.go:1010-1022 — `previewSessionsRefreshedMsg` handler — source-agnostic.
  - internal/tui/preview_attach.go:38 — `previewAttachBailMsg{Session string}` type.
- Notes: Placeholder core extracted into shared helper (DRY). Reads from msg.Session per plan's robustness requirement (preview is zeroed inside helper). Phase 2 uses tea.Batch (not Sequence) so flash and page flip render same frame.

TESTS:
- Status: Adequate.
- Coverage (internal/tui/preview_attach_bail_test.go):
  - TestPreviewAttachBailFlipsToPageSessions (line 47) — transition.
  - TestPreviewAttachBailZerosPreviewModel (line 60) — zeroing.
  - TestPreviewAttachBailDispatchesRefreshCmd (line 74) — refresh cmd drained from Phase 2 batch.
  - TestPreviewAttachBailPreservesSessionNameFromMessage (line 108) — proves msg.Session is the source by using different name from preview-open.
  - TestPreviewAttachBailNoListerStillEmitsTickCleanly (line 139) — nil-lister tolerated.
  - TestPreviewAttachBailToleratesListerErrorSilently (line 167) — lister error swallowed.
  - TestPreviewAttachBailEmptySessionNameStillTransitions (line 207) — defensive empty name.
  - TestEscDismissPathUnchangedAfterBailHandlerAdded (line 241) — Esc regression.
- Tests use drainBatchCmds/findRefreshedMsg helpers to peel Phase 2 batch wrapper — placeholder assertions stable across Phase 2 overlay.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, established TUI handler/option shape, seam injection via `modelWithSeamsAndLister`.
- SOLID: Good. `exitPreviewToSessions` is a clean single-responsibility DRY consolidation across the two handlers, with documented capture-before-zero pre-condition.
- Complexity: Low.
- Modern idioms: Yes. Idiomatic tea.Batch + typed message switch.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `exitPreviewToSessions` mutates the receiver AND returns the refresh cmd — fine for an internal helper called from two sites, but if a third caller emerges consider splitting into a pure transition helper + refresh-cmd factory.
