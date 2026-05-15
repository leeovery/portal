---
topic: enter-attaches-from-preview
cycle: 1
total_proposed: 2
---
# Analysis Tasks: enter-attaches-from-preview (Cycle 1)

## Task 1: Restructure preview-Enter to hand off connector after TUI quit
status: pending
severity: high
sources: architecture

**Problem**: The preview-page Enter handler invokes `previewSessionConnector.Connect` from inside a `tea.Cmd` goroutine while the Bubble Tea program is still running, and the `previewAttachErrorMsg{Err: nil}` handler in `internal/tui/model.go:982-992` returns `(m, nil)` (no `tea.Quit`) on success. For the outside-tmux path this is fine — `syscall.Exec` replaces the process and the TUI dies with it. For the inside-tmux path (`SwitchConnector.Connect` → `tmux switch-client`), the tmux client moves to the target session but portal's process keeps event-looping with no UI surface, leaving a stale portal process alive. This diverges from the Sessions-page Enter shape, where `handleSessionListEnter` sets `selected` + `tea.Quit`, the TUI exits cleanly, and `processTUIResult(model, connector)` runs the connector AFTER the program has shut down. The pipeline's own docstring (`internal/tui/preview_attach.go:56-58`) asserts inside-tmux the top-level handler quits the program so the surrounding tmux client repaints — but the handler does the opposite, and the unit test at `internal/tui/preview_attach_bail_test.go:253-264` explicitly pins the current nil-cmd behaviour, locking in the doc/code mismatch.

**Solution**: Restructure preview-Enter to follow the same post-TUI handoff shape as Sessions-page Enter. Split the pipeline so the `tea.Cmd` runs only steps 1-3 (probe + pre-selects), captures `selected` + the (window, pane) coords on the model, and returns `tea.Quit`; have `processTUIResult` consume that intent and call `connector.Connect` post-TUI. This keeps the two Enter paths structurally symmetric.

**Outcome**: After preview-Enter on the inside-tmux path, the portal TUI exits cleanly before `switch-client` is invoked; no orphaned portal process remains. The two Enter paths (Sessions and Preview) share the same connector-after-quit shape.

**Do**:
1. Inspect `internal/tui/preview_attach.go:51-63` and `:117-150` to understand the current pipeline split.
2. Inspect `internal/tui/model.go:982-992` and `cmd/open.go:412-478` (`processTUIResult`) to see the Sessions-page handoff shape.
3. Refactor the pipeline so the `tea.Cmd` performs steps 1-3 only (probe + pre-selects), records `selected` (session name) on the model, and returns a message that the top-level Update handler converts to `tea.Quit`.
4. Update `processTUIResult` to detect the preview-Enter intent on the returned model and call `connector.Connect(selected)` after the Bubble Tea program has shut down.
5. Update or remove the test at `internal/tui/preview_attach_bail_test.go:253-264` that pins the current nil-cmd behaviour; replace it with a test asserting that successful preview-Enter returns a quit signal.
6. Reconcile the docstring at `preview_attach.go:56-58` with the new shape.
7. Run `go build -o portal .` and `go test ./...`.

**Acceptance Criteria**:
- Preview-Enter inside-tmux path leaves no orphan portal process after `switch-client` succeeds.
- Successful preview-Enter returns a cmd that causes the Bubble Tea program to quit (parity with Sessions-page Enter).
- `connector.Connect` is invoked from the post-TUI handoff, not from inside a live `tea.Cmd`.
- Docstring at `internal/tui/preview_attach.go:56-58` is consistent with implementation.
- Outside-tmux path behaviour is unchanged (still terminates via `syscall.Exec`).
- All existing tests pass; the pinning test for nil-cmd on success is updated.

**Tests**:
- Unit: successful preview-Enter (nil Err) returns a cmd whose execution emits `tea.Quit`.
- Unit: preview-Enter records the selected session name on the model so the post-TUI handoff can consume it.
- Integration: simulate the inside-tmux path with a mock `SwitchConnector`; assert `Connect` is called once, after `program.Run()` has returned.
- Regression: outside-tmux path still routes through `AttachConnector` / `syscall.Exec`.

## Task 2: Extract shared preview-teardown helper for dismiss + bail handlers
status: pending
severity: medium
sources: architecture, duplication

**Problem**: The `previewDismissedMsg` handler at `internal/tui/model.go:937-958` and the `previewAttachBailMsg` handler at `internal/tui/model.go:959-981` execute the same three-step preview-teardown prelude before diverging: capture `preserveName`, set `m.activePage = PageSessions`, zero `m.preview = previewModel{}`, then call `m.refreshSessionsAfterPreviewCmd(preserveName)`. Only the source of `preserveName` (`m.preview.session` vs `msg.Session`) and the trailing flash/tick batching differ.

**Solution**: Extract `(m *Model) exitPreviewToSessions(preserveName string) tea.Cmd` that performs the page flip and preview zero, returning the refresh cmd. Both handlers call it; the bail handler additionally calls setFlash and tea.Batch'es the flash tick.

**Outcome**: Both handlers share a single source of truth for the preview-teardown sequence.

**Do**:
1. Read `internal/tui/model.go:937-981` to confirm the exact current shape of both handlers.
2. Add `func (m *Model) exitPreviewToSessions(preserveName string) tea.Cmd` near the existing preview handlers.
3. Replace the dismiss handler body with a capture of `m.preview.session` followed by `return m, m.exitPreviewToSessions(capturedName)`.
4. Replace the bail handler body with `refreshCmd := m.exitPreviewToSessions(msg.Session); m.setFlash(formatSessionGoneFlash(msg.Session)); return m, tea.Batch(refreshCmd, flashTickCmd(m.flashGen))`.
5. Run `go test ./internal/tui/...` and `go test ./...`.

**Acceptance Criteria**:
- A single `exitPreviewToSessions` helper exists on `*Model` and is called from both handlers.
- Esc-dismiss behaviour unchanged.
- Bail behaviour unchanged (still uses `msg.Session` for preserveName).
- No behavioural change visible to tests.

**Tests**:
- Existing dismiss-handler test continues to pass without modification.
- Existing bail-handler test continues to pass without modification.
- Optional: a unit test on `exitPreviewToSessions` directly.
