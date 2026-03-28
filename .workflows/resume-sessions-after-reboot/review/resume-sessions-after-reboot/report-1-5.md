TASK: Hooks Rm Command

ACCEPTANCE CRITERIA:
- `portal hooks rm --on-resume` removes hook from `hooks.json` for the pane in `$TMUX_PANE`
- `portal hooks rm --on-resume` deletes volatile marker `@portal-active-{pane_id}`
- Running without `$TMUX_PANE` set produces error: "must be run from inside a tmux pane"
- Running without `--on-resume` flag produces a cobra required-flag error
- Removing a non-existent hook is a silent no-op (exit 0, no error output)
- `hooksDeps` DI struct allows test injection of the `ServerOptionDeleter`
- All tests pass: `go test ./cmd -run TestHooksRm`

STATUS: Complete

SPEC CONTEXT: The spec defines `hooks rm --on-resume` as a subcommand that reads `$TMUX_PANE`, removes the persistent JSON entry for that pane's `on-resume` event, and deletes the volatile tmux server option `@portal-active-{pane_id}`. The spec requires `rm` to be a silent no-op when no hook exists (supports scripting), requires `$TMUX_PANE` (error if absent), and requires the `--on-resume` flag. `hooks` is added to `skipTmuxCheck` to bypass Portal's tmux bootstrap.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/hooks.go:133-164 (hooksRmCmd), cmd/hooks.go:18-19 (ServerOptionDeleter interface), cmd/hooks.go:27-30 (HooksDeps struct with OptionDeleter field)
- Notes: Implementation matches all acceptance criteria and the plan. The command reads `$TMUX_PANE` via `requireTmuxPane()` (shared with `set`), loads the hook store, calls `store.Remove(paneID, "on-resume")`, then calls `deleter.DeleteServerOption(hooks.MarkerName(paneID))`. The `--on-resume` flag is registered as a required Bool flag in `init()` (line 170-171). The `MarkerName` helper at `internal/hooks/executor.go:53` generates `@portal-active-{paneID}`. The `hooks` command is in `skipTmuxCheck` (cmd/root.go:19). DI is handled via the package-level `hooksDeps` variable with `OptionDeleter` field.

TESTS:
- Status: Adequate
- Coverage: All 8 tests specified in the plan are present in `TestHooksRmCommand` (cmd/hooks_test.go:403-657):
  - "removes hook and volatile marker for current pane" (line 404)
  - "reads pane ID from TMUX_PANE environment variable" (line 442)
  - "returns error when TMUX_PANE is not set" (line 479)
  - "returns error when on-resume flag is not provided" (line 507)
  - "silent no-op when no hook exists for pane" (line 530)
  - "removes correct JSON entry from hooks file" (line 558) -- verifies other panes are untouched
  - "deletes volatile marker with correct option name" (line 595)
  - "cleans up pane key when last event removed" (line 626)
- Notes: Tests are well-structured. Each test seeds the hooks JSON, sets `TMUX_PANE`, injects a mock `ServerOptionDeleter`, and verifies both file state and mock calls. The mock types (`mockOptionDeleter`) are minimal and focused. Edge cases (no-op removal, multi-pane selective removal, pane key cleanup) are covered. No over-testing -- each test verifies a distinct behavior. Tests follow project convention (no `t.Parallel()`, package-level DI with `t.Cleanup`).

CODE QUALITY:
- Project conventions: Followed. Matches the DI pattern used by other commands (`hooksDeps` package-level struct with `t.Cleanup`). Uses small single-method interfaces (`ServerOptionDeleter`). `--on-resume` registered as required flag via Cobra. `hooks` in `skipTmuxCheck`. Shared `requireTmuxPane()` helper reused across `set` and `rm`.
- SOLID principles: Good. `ServerOptionDeleter` is a single-method interface (ISP). DI via `hooksDeps` follows DIP. `Store.Remove` has single responsibility.
- Complexity: Low. The `RunE` is a linear sequence of 4 steps: validate pane, load store, remove hook, delete marker. No branching complexity.
- Modern idioms: Yes. Idiomatic Go error handling, interface-based DI, `fmt.Errorf` for error messages.
- Readability: Good. Code is self-documenting. Comments explain why `hooksDeps` exists and what `requireTmuxPane` does.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- `Store.Remove` (internal/hooks/store.go:83-97) always calls `Save` even when the pane/event did not exist. This is a minor inefficiency -- it rewrites the file for a no-op removal. Not a bug (functionally correct), but could be optimized with an early return when no mutation occurred. Very minor.
