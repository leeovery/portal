TASK: Migrate hooks list to Structural Keys

ACCEPTANCE CRITERIA:
- hooks list displays structural keys instead of pane IDs
- All TestHooksListCommand tests pass with structural key values
- go test ./cmd/ -run TestHooksList passes

STATUS: Complete

SPEC CONTEXT: The specification states: "The Hook struct's PaneID field becomes a structural key field. List() populates it with structural keys. The hooks list CLI output displays structural keys instead of pane IDs." The structural key format is `session_name:window_index.pane_index`. The task was created because Phase 2 renamed `Hook.PaneID` to `Hook.Key` in `internal/hooks/store.go`, leaving `cmd/hooks.go` with a compile error referencing the old field name.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/hooks.go:101 — `fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", h.Key, h.Event, h.Command)`
- Notes: The field reference is `h.Key` (not the old `h.PaneID`). No `.PaneID` references exist anywhere in Go source files (confirmed via grep). The `Hook` struct in `/Users/leeovery/Code/portal/internal/hooks/store.go:15-19` defines `Key string` as the field. The `List()` method at `store.go:100-125` populates the `Key` field from the map key and sorts by `Key` then `Event`. Implementation matches the plan and spec precisely.

TESTS:
- Status: Adequate
- Coverage:
  - "outputs hooks in tab-separated format" (hooks_test.go:14) — uses structural key `my-project-abc123:0.0`, verifies exact output format
  - "produces empty output when no hooks registered" (hooks_test.go:40) — empty store edge case
  - "produces empty output when hooks file does not exist" (hooks_test.go:62) — missing file edge case
  - "outputs hooks sorted by key then event" (hooks_test.go:84) — uses structural keys `proj-abc:0.0`, `proj-abc:1.0`, `other-proj:0.0`; verifies sort order
  - "hooks bypasses tmux bootstrap" (hooks_test.go:112) — ensures list command doesn't require tmux
  - "accepts no arguments" (hooks_test.go:131) — validates arg enforcement
- Notes: All test data uses structural key format (`session:window.pane`). No pane ID values (`%N`) appear in list test data. Tests verify exact output strings including structural keys. The store-level `TestList` tests in `store_test.go:356-407` also use structural keys and verify the `Key` field. Test coverage is proportional to the scope of the change.

CODE QUALITY:
- Project conventions: Followed — uses the established DI pattern via `loadHookStore()`, tab-separated output format, Cobra command structure
- SOLID principles: Good — the list command has single responsibility (read and display), uses the `Store` abstraction, no unnecessary coupling
- Complexity: Low — straightforward iteration over `store.List()` results with `fmt.Fprintf`
- Modern idioms: Yes — idiomatic Go error handling with early returns
- Readability: Good — the list command handler is 16 lines, clear and self-documenting
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
