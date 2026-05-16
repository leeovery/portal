TASK: enter-attaches-from-preview-1-4 — Build pre-select attach pipeline tea.Cmd factory with bail message and WARN-swallow logger

ACCEPTANCE CRITERIA (from plan):
- Run returns non-nil tea.Cmd
- Success path: HasSessionProbe → SelectWindow → SelectPane → (connector) in order, exactly once each
- HasSessionProbe (false, ExitError) → previewAttachBailMsg{Session}; no further calls
- HasSessionProbe (true, OS-err) → WARN with ComponentPreview, proceed
- SelectWindow / SelectPane non-zero → WARN with ComponentPreview, proceed
- Connector error → previewAttachErrorMsg{Err}  (superseded — see below)
- Defensive empty-session guard
- No structural enumeration calls

STATUS: Complete

SPEC CONTEXT:
Spec § Pre-select + attach sequence pins the ordered pre-select calls + connector. Phase 3 task 3-1 (already merged; see .workflows/enter-attaches-from-preview/planning/enter-attaches-from-preview/planning.md lines 84-87 and the architecture-c1 analysis) intentionally restructured the pipeline so the connector handoff moved post-TUI to cmd/open.go `processTUIResult`. The plan text for 1-4 predates that restructure, so its `previewAttachErrorMsg` + in-pipeline connector wording is superseded. The current implementation still satisfies the spec's ordering contract because the post-TUI handoff still runs after the three pre-selects complete.

IMPLEMENTATION:
- Status: Implemented (post-Phase-3 shape)
- Location: internal/tui/preview_attach.go (whole file, lines 1-150)
  - `previewAttachTmux` interface lines 23-27 with compile-time drift guard at line 13 (`var _ previewAttachTmux = (*tmux.Client)(nil)`).
  - `previewAttachBailMsg{Session}` line 38; `previewAttachSelectedMsg{Session}` line 54 (replaces planned `previewAttachErrorMsg`).
  - `previewAttachPipeline` struct lines 69-72 (tmux + logger; no connector field per Phase 3 restructure).
  - `NewPreviewAttachPipeline` exported constructor line 86 returning `PreviewAttacher`.
  - `Run` lines 116-150: step 0 empty-session guard → step 1 HasSessionProbe (3-shape discriminator) → step 2 SelectWindow WARN-swallow → step 3 SelectPane WARN-swallow → step 4 emit `previewAttachSelectedMsg`.
  - `state.ComponentPreview = "preview"` constant added at internal/state/logger.go:37.
  - Exported seam `PreviewAttacher` at internal/tui/model.go:40-49.

TESTS:
- Status: Adequate
- Primary file: internal/tui/preview_attach_test.go covers
  - non-nil Cmd return
  - success path ordering and args
  - ExitError bail via a real synthesised `*exec.ExitError` (`makeExitError` runs `sh -c 'exit 1'` — exercises the actual errors.As chain, honest test)
  - OS-layer probe error proceeds with WARN
  - SelectWindow error logs + proceeds
  - SelectPane error logs + proceeds
  - both selects error: both WARNs, still emits selected
  - nil-logger contract (no panic)
  - empty-session defensive bail (zero tmux calls)
- Supplementary: preview_attach_selected_test.go, preview_attach_pipeline_handoff_test.go (post-TUI handoff regression guard for orphan-portal-process), preview_attach_wiring_test.go.
- Notes: No explicit assertion that the recorded call verb set excludes `list-*` / `display-message`. The `fakePreviewAttachTmux` API structurally cannot record those verbs, so it is implicit — marginally under-asserted relative to the plan's "no enumeration" criterion, not blocking.

CODE QUALITY:
- Project conventions: Followed (small DI seam, no package-level mutable state required)
- SOLID: Good — single responsibility, segregated tmux interface, exported `PreviewAttacher` keeps the concrete struct internal
- Complexity: Low — linear function with explicit step comments
- Modern idioms: Yes — `errors.As` discriminator contract on HasSessionProbe, structured-logger nil-receiver tolerance honoured
- Readability: Good — godocs on `previewAttachSelectedMsg` (lines 46-56) and `Run` (lines 90-115) explain the Phase-3 restructure rationale and step semantics inline
- Issues: `PreviewAttacher` godoc at internal/tui/model.go:43-44 still says "executes the four-call sequence end-to-end" — stale relative to the post-Phase-3 three-call shape.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] internal/tui/model.go:40-49 — `PreviewAttacher` godoc references the obsolete "four-call sequence". Update to "three pre-select calls; connector handoff is post-TUI in cmd/open.go `processTUIResult`" to match the accurate docstrings in preview_attach.go.
- [idea] internal/tui/preview_attach_test.go could add an explicit allowlist assertion on `tm.calls` verbs to lock in the no-enumeration acceptance criterion against future fake-API expansions (currently guaranteed by the fake's narrow surface, not by a test).
