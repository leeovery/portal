TASK: enter-attaches-from-preview-1-5 — Wire attach pipeline seam into tuiConfig, tui.Model, and openTUI production construction

STATUS: Complete

SPEC CONTEXT: Spec § Pre-select + attach sequence > step 4 requires the preview-Enter pipeline to reuse the existing AttachConnector / SwitchConnector code paths. The wiring follows the TUI's established constructor-injected Option pattern (mirrors WithEnumerator / WithScrollbackReader).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:40-49 — `PreviewAttacher` interface
  - internal/tui/model.go:200 — `Model.previewAttacher` field
  - internal/tui/model.go:503-513 — `WithPreviewAttachPipeline` option
  - internal/tui/model.go:1420 — `NewPreviewModel` call site passes `m.previewAttacher`
  - internal/tui/preview_attach.go:13 — compile-time `var _ previewAttachTmux = (*tmux.Client)(nil)`
  - internal/tui/preview_attach.go:86-88 — `NewPreviewAttachPipeline` exported constructor
  - internal/tui/pagepreview.go:52 — `previewModel.attacher` field
  - internal/tui/pagepreview.go:74 — `NewPreviewModel` signature with attacher
  - cmd/open.go:321 — `tuiConfig.previewAttacher` field
  - cmd/open.go:353-355 — `buildTUIModel` appends `WithPreviewAttachPipeline` when non-nil
  - cmd/open.go:431-436 — `openTUI` constructs the production pipeline
- Notes: The seam was intentionally redesigned vs. the original plan — the pipeline no longer holds the `SessionConnector`. The connector is invoked post-TUI in `processTUIResult` to fix the inside-tmux orphan-portal-process regression. `NewPreviewAttachPipeline` therefore takes only `(previewAttachTmux, *state.Logger)`. This design change is documented in preview_attach.go:42-56. Compile-time interface satisfaction is asserted via the `var _` blank assignment.

TESTS:
- Status: Adequate
- Coverage:
  - `TestWithPreviewAttachPipeline_WiresAttacherOntoModel` — option wires onto Model
  - `TestNewPreviewModel_PropagatesAttacherOntoPreviewModel` — NewPreviewModel forwards attacher field
  - `TestNewPreviewModel_AcceptsNilAttacher` — defensive nil acceptance
  - `TestSpaceOnSessionsPage_PassesModelAttacherIntoPreviewModel` — Space-handler propagation
  - `TestNewPreviewAttachPipeline_ReturnsNonNilAttacher` — constructor smoke
  - `TestNewPreviewAttachPipeline_NilLoggerTolerated` — nil-logger no-panic
- Notes: No smoke test in cmd/open_test.go asserts the production `openTUI` path constructs a non-nil attacher. After the Phase 3 connector decoupling, the original plan's "openTUI constructs pipeline backed by AttachConnector/SwitchConnector" assertions no longer apply byte-for-byte (connector left the pipeline), but a non-nil-attacher smoke at the cmd boundary remains uncovered.

CODE QUALITY:
- Project conventions: Followed — Option pattern adjacent to `WithEnumerator` / `WithScrollbackReader`; constructor-injected seam; nil-tolerant test ergonomics.
- SOLID: Good — PreviewAttacher is a single-method seam (ISP); Model depends on the seam, not the concrete pipeline (DIP).
- Complexity: Low — straightforward plumbing.
- Modern idioms: Yes — the compile-time `var _` interface assertion at preview_attach.go:13 is the idiomatic Go pattern.
- Readability: Good — godocs are thorough and reference spec sections.
- Issues: minor docstring/ordering issues noted below.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] `WithPreviewAttachPipeline` docstring at internal/tui/model.go:503-508 says "closing over *tmux.Client + the resolved SessionConnector + a nullable *state.Logger" — the connector reference is stale (it has moved post-TUI). Drop the connector mention.
- [quickfix] cmd/open.go:432 places `defer previewLogger.Close()` BEFORE the err check on lines 433-435. Functions correctly (Close is nil-safe; the defer captures whichever value was returned by OpenLogger), but reads unconventionally — Go idiom is err-check first, then defer.
- [idea] No cmd/open_test.go smoke test asserts `openTUI` constructs a non-nil `cfg.previewAttacher`. Plan called for this; would catch a future wiring regression at the cmd boundary.
