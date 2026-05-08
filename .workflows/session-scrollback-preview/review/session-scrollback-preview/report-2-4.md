TASK: session-scrollback-preview-2-4 — Esc dismiss returns to Sessions list preserving cursor and filter state

ACCEPTANCE CRITERIA:
- Esc while on pagePreview transitions back to PageSessions.
- After dismiss, m.sessionList.Index() equals the value captured before Space.
- After dismiss, m.sessionList.FilterValue() equals value captured before Space (no filter case).
- After dismiss when filter committed before Space, filter remains committed.
- A subsequent Esc on Sessions page falls through to bubbles/list default Esc handling.
- Re-opening preview after dismiss constructs a fresh previewModel.

STATUS: Complete

SPEC CONTEXT:
Spec § Esc Level Tree pins preview-owned level 1: "In preview → return to Sessions list. Filter (if any) stays committed; cursor stays on the previewed session." Spec § No In-preview Between-Session Stepping notes the cursor preservation invariant relies on preview never mutating the underlying sessionList.

IMPLEMENTATION:
- Status: Implemented (with Phase 4 enhancement layered on top).
- Location:
  - internal/tui/pagepreview.go:262-263 — Esc emits previewDismissedMsg via tea.Cmd.
  - internal/tui/pagepreview.go:217-222 — previewDismissedMsg type declaration.
  - internal/tui/model.go:876-897 — top-level Update consumes previewDismissedMsg, flips activePage to PageSessions, zeroes m.preview, and dispatches refreshSessionsAfterPreviewCmd.
  - internal/tui/model.go:898-910 — previewSessionsRefreshedMsg consumer (Phase 4 layer): applies fresh sessions and re-anchors cursor by name.
  - internal/tui/model.go:894 — preserveName captured BEFORE zeroing m.preview (ordering correctness commented inline).
- Notes:
  - Implementation goes beyond strict task 2-4 scope by also dispatching a Sessions-list refresh on dismiss. The plan task notes this is Phase 4 scope. Per recent commits, Phase 4 has landed.
  - When m.sessionLister is nil (modelWithSeams harness), refreshSessionsAfterPreviewCmd returns nil — no refresh, m.sessionList untouched, cursor/filter preserved byte-identically.
  - When m.sessionLister is wired (production), reanchorSessionCursor re-anchors by name.
  - Implementation does NOT mutate m.sessionList directly during dismiss — preserving the spec's "preview never mutates the underlying list" invariant.
  - m.preview = previewModel{} releases viewport memory; re-open via Space constructs a fresh previewModel.

TESTS:
- Status: Adequate.
- Location: internal/tui/pagepreview_dismiss_test.go (6 test functions).
- Coverage: TestPreviewEscReturnsToSessionsPage; TestPreviewEscPreservesListCursor (Index()=3 round-trips); TestPreviewEscPreservesNoFilterState; TestPreviewEscPreservesCommittedFilter (filter "alpha" preserved); TestSecondEscClearsCommittedFilterViaListDefault; TestPreviewReopenAfterDismissConstructsFreshPreviewModel (enumerator.calls=2 and reader.calls=2 after re-open).
- Notes:
  - All six acceptance criteria have direct test counterparts.
  - pressSpaceThenEsc drives the full Esc round-trip including draining the previewDismissedMsg cmd through Update.
  - For nil-SessionLister test path, the refresh cmd is nil so single-cmd-drain captures the full observable surface.
  - No redundant assertions; each test's invariant is distinct.

CODE QUALITY:
- Project conventions: Followed. Constructor injection for seams. No tmuxtest import in dismiss test.
- SOLID principles: Good. previewModel.Update routes Esc to a sentinel msg; page-machine flip and refresh dispatch live in top-level Update, not in previewModel.
- Complexity: Low.
- Modern idioms: Yes. Uses tea.Cmd to defer message emission rather than mutating page state at message time inside previewModel.Update.
- Readability: Good. previewDismissedMsg doc comment documents no-mutation invariant; top-level handler's comment explains preserveName-before-zero ordering.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Task 2-4 cursor-preservation criterion ("m.sessionList.Index() equals the value captured before Space") only holds strictly in the test path where SessionLister is nil. In production, after the Phase 4 refresh, cursor preservation is by name, not by index. If the test harness ever wires a SessionLister, TestPreviewEscPreservesListCursor may need to assert by name rather than by Index().
- [idea] previewModel zero-value reserved for "between opens" per the doc comment. Consider a hasPreview bool sentinel or a *previewModel pointer to make "between opens" state observably distinct.
- [quickfix] previewSessionsRefreshedMsg (Phase 4) currently lives in pagepreview.go alongside previewDismissedMsg (task 2-4). Consider relocating to a Phase 4-themed file. Cosmetic only.
