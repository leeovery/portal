TASK: session-scrollback-preview-4-2 — Error-string rendering for (nil, err) Tail outcomes with retry-on-refocus

ACCEPTANCE CRITERIA:
- When Tail returns (nil, err), viewport content equals "(unable to read scrollback)".
- Error string uniform across errno types.
- After landing on an error, cycling away and back reissues a fresh Tail (mock records two calls for that paneKey).
- No field on previewModel stores per-pane error state.
- Chrome unaffected by error branch.

STATUS: Complete

SPEC CONTEXT:
Spec § Read-Failure Handling > Placeholder > Error string: "OS-level read errors render a single short error string in the viewport rather than the placeholder. The wording is build-phase TBD; the same string is used for every error type. Future focus changes onto the same pane retry the read fresh — there is no per-pane error cache."

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go
  - Line 34: const previewReadError = "(unable to read scrollback)" — single package-level constant.
  - Lines 202-215: readFocusedPaneIntoViewport dispatcher with three-arm switch — uniform error branch (case err != nil:) renders previewReadError regardless of errno.
  - Single dispatcher invoked by initial-open (101), Tab (279), ] (296), [ (304) — every focus-changing path funnels through it; no caching, no short-circuit.
  - previewModel struct (48-58) contains only session, enumerator, reader, groups, windowIdx, paneIdx, viewport, width, height — zero error-cache fields.
  - chromeLine (163-173) is a pure function of structural state.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_error_test.go
- Coverage: All 9 acceptance-criteria-aligned scenarios:
  - TestPreviewError_RendersAtInitialOpenWhenTailReturnsNilErr (67).
  - TestPreviewError_StringIsUniformAcrossErrnoTypes (86) — EACCES/EIO/generic produce byte-identical output.
  - TestPreviewError_StringDiffersFromPlaceholder (115).
  - TestPreviewError_StringIsCanonicalWordingUnableToReadScrollback (121).
  - TestPreviewError_RefocusAfterErrorIssuesFreshTailViaTab (127) — Tab away/back asserts pane0 Tail count == 2.
  - TestPreviewError_RefocusAfterErrorIssuesFreshTailViaBracket (173).
  - TestPreviewError_SecondTailCallAfterErrorSeesNewOutcome (219) — transient error → recovery.
  - TestPreviewError_NoPerPaneErrorStateOnPreviewModel (279) — reflection-based field audit.
  - TestPreviewError_ChromeCountsUnaffectedByErrorBranch (303).

CODE QUALITY:
- SOLID: Single dispatcher with single responsibility for outcome translation.
- Complexity: Low.
- Modern idioms: Idiomatic Go switch; value-receiver consistency preserved.
- Readability: Doc comments at lines 26-34 and 175-201 explicitly document the no-cache invariant with spec section references.
- Project conventions: No t.Parallel(); constructor-injected seams.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Dispatcher arm `case err != nil:` (208) technically also catches a (bytes != nil, err != nil) shape that the helper contract never produces. Defensive but worth a one-line comment pinning the precedence rationale.
