TASK: enter-attaches-from-preview-3-1 — Restructure Preview-Enter to Hand Off Connector After TUI Quit

ACCEPTANCE CRITERIA:
- Preview-Enter inside-tmux path leaves no orphan portal process after `switch-client` succeeds.
- Successful preview-Enter returns a cmd that causes the Bubble Tea program to quit (parity with Sessions-page Enter).
- `connector.Connect` is invoked from the post-TUI handoff, not from inside a live `tea.Cmd`.
- Docstring at internal/tui/preview_attach.go:56-58 is consistent with implementation.
- Outside-tmux path behaviour is unchanged (still terminates via `syscall.Exec`).
- All existing tests pass; the pinning test for nil-cmd on success is updated.

STATUS: Complete

SPEC CONTEXT: Spec § Pre-select + attach sequence requires steps 1–3 (HasSessionProbe, SelectWindow, SelectPane) to complete before the connector hands off the terminal. Spec § Transition mechanics requires the sequence to be authored as a single logical unit from preview's Update without intermediate render. Phase 3 Cycle 1 surfaced a defect: the prior pipeline ran connector.Connect inside the tea.Cmd goroutine, which (inside-tmux) left an orphan portal event-loop after switch-client moved the surrounding client elsewhere.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/preview_attach.go:42-150 — new `previewAttachSelectedMsg{Session}` envelope; `previewAttachPipeline` no longer holds a connector; `Run` emits the new envelope on success after steps 1–3.
  - internal/tui/model.go:993-1009 — new handler records `m.selected = msg.Session` and returns `tea.Quit`, mirroring `handleSessionListEnter`.
  - cmd/open.go:412-486 — `connector := buildSessionConnector(client)` resolved once in `openTUI`; consumed post-`p.Run()` via `processTUIResult(model, connector)` for both Sessions-page and Preview-page Enter.
  - cmd/open.go:369-378 — `processTUIResult` unchanged; reuses the existing `model.Selected() → connector.Connect(selected)` path for both Enter shapes.
- Notes: The empty-session defensive guard at preview_attach.go:118-121 is preserved. The `(false, *exec.ExitError)` and `(true, OS-layer err)` discriminator branches are intact.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/preview_attach_selected_test.go — three focused unit tests cover (a) `m.selected` recorded, (b) handler returns `tea.Quit`-bearing cmd, (c) parity with Sessions-page Enter shape.
  - internal/tui/preview_attach_pipeline_handoff_test.go — pipeline success returns `previewAttachSelectedMsg`; ConnectInvokedAfterQuit asserts the connector runs only AFTER tea.Quit, with all 3 tmux calls already executed (orphan-process regression guard).
  - The prior `previewAttachErrorMsg` nil-cmd pinning tests were retired and explicitly noted in preview_attach_bail_test.go:21 and preview_attach.go:62-65.
  - Outside-tmux unchanged behaviour exercised via `processTUIResult` tests in cmd/open_test.go:957-994.
- Notes: The integration-style handoff test at preview_attach_pipeline_handoff_test.go:40-75 is the load-bearing assertion for the original inside-tmux defect.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`; DI seams (`PreviewAttacher`, `previewAttachTmux`) match the rest of the codebase; compile-time assertion `var _ previewAttachTmux = (*tmux.Client)(nil)` preserved.
- SOLID principles: Good. The pipeline shed the connector dependency cleanly — single-responsibility tightened (pre-select only, no transport).
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: Two stale docstrings still describe the old "four-call" pipeline shape.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/model.go:42-44 — `PreviewAttacher` godoc says `Run returns a tea.Cmd that executes the four-call sequence end-to-end`. Reword to "three-call pre-select sequence" or "executes the pre-select steps and emits a selected/bail envelope; the connector handoff is post-TUI in `processTUIResult`."
- [quickfix] internal/tui/pagepreview.go:248-251 — `Update` godoc says Enter "dispatch[es] the four-call pre-select + attach pipeline". Same correction.
