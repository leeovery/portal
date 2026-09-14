## Attempt 1

ISSUES:
- The load-bearing ordering inside the `BootstrapCompleteMsg` arm is asserted only in prose. `internal/tui/model.go:1653-1664` returns early once a mint is staged, bypassing the rest of the arm; its correctness depends on sitting below the `pendingBootstrapWarnings` append at model.go:1651, which `cmd/open.go:618` is the sole consumer of at teardown. No test covers the combination: the existing warnings test ("it stages the complete message's warnings for the teardown on the command-pending route", internal/tui/command_pending_bootstrap_test.go:128) never stages a mint, so it exercises the fall-through, and the new staged-mint test discards both the model and the warnings. Move the block above the append and every test stays green while a cold `portal open -- <cmd>` that picks during the bootstrap silently loses its saver-down / restore warnings — the user runs their command, sees nothing, and finds out at the next reboot when nothing was saved.
  FIX: In `internal/tui/command_pending_staged_mint_test.go`, extend "it mints the staged pick when the bootstrap's complete event arrives": keep the returned model (`model, cmd := model.Update(...)`), send `tui.BootstrapCompleteMsg{Warnings: []tui.BootstrapWarning{{Lines: []string{"Portal's session saver is not running."}}}}`, and assert `model.(tui.Model).PendingBootstrapWarnings()` still holds the one warning alongside the existing `SessionCreatedMsg` assertions — the accessor is already exported and used the same way at command_pending_bootstrap_test.go:141.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- internal/tui/model.go:1807-1808 — the predicate's doc claims "has not yet reported a terminal event", which the fatal path falsifies: `BootstrapFatalMsg` is a terminal event (it is why the receiver is not re-issued, model.go:1671-1672) and it leaves `bootstrapComplete` false, so `bootstrapInFlight()` returns true for a fatal'd picker. Only `createSession`'s separate `fatalActive` refusal keeps that from mattering today; a second caller taking the doc at its word would mint against a fatal'd bootstrap.
  OLD:
// A picker whose bootstrap runs concurrently with it and has not yet reported a
// terminal event. The warm route has no receiver: its bootstrap ran before Init.
  NEW:
// A picker whose bootstrap runs concurrently with it and has not yet completed.
// A fatal leaves bootstrapComplete false, so a fatal'd picker reads as in flight
// too: a caller that must exclude one checks fatalActive itself. The warm route
// has no receiver: its bootstrap ran before Init.
