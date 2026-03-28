---
topic: resume-sessions-after-reboot
cycle: 2
total_proposed: 1
---
# Analysis Tasks: Resume Sessions After Reboot (Cycle 2)

## Task 1: Consolidate hooks set/rm shared boilerplate in cmd/hooks.go
status: pending
severity: low
sources: duplication

**Problem**: `cmd/hooks.go` contains two duplicated patterns between `hooksSetCmd` and `hooksRmCmd`: (1) two near-identical builder functions (`buildHooksDeps` and `buildHooksDeleteDeps`, lines 35-50) that differ only in which interface field they return, and (2) identical TMUX_PANE validation blocks (lines 87-90 and 137-140) with the same env read, empty check, and error message.

**Solution**: Extract a `requireTmuxPane() (string, error)` helper that returns the pane ID or the standard error. Consolidate the two builder functions into a single `buildHooksClient() *tmux.Client` (or equivalent) that returns the concrete client when no test deps are injected, since `tmux.Client` satisfies both `ServerOptionSetter` and `ServerOptionDeleter`. When `hooksDeps` is set, callers use the appropriate interface field directly from `hooksDeps`.

**Outcome**: Each validation rule and construction pattern has a single source of truth. Future changes (e.g., adding a `--pane` flag, changing the fallback client construction) require modification in one place only.

**Do**:
1. Add a `requireTmuxPane() (string, error)` function that reads `os.Getenv("TMUX_PANE")`, returns it if non-empty, or returns `fmt.Errorf("must be run from inside a tmux pane")`.
2. Replace the inline TMUX_PANE checks in both `hooksSetCmd.RunE` and `hooksRmCmd.RunE` with calls to `requireTmuxPane()`.
3. Replace `buildHooksDeps()` and `buildHooksDeleteDeps()` with a single function (e.g., `buildHooksClient()`) that returns the `tmux.Client` when `hooksDeps` is nil, or restructure so the set command reads `hooksDeps.OptionSetter` and rm reads `hooksDeps.OptionDeleter` directly through a shared nil-check helper. The key constraint: only one `tmux.NewClient(&tmux.RealCommander{})` construction site.
4. Update existing tests to confirm they still pass with the refactored helpers.

**Acceptance Criteria**:
- `requireTmuxPane` is the single place that validates TMUX_PANE presence
- Only one code path constructs the fallback `tmux.Client` for hooks commands
- All existing hooks tests pass without modification (or with minimal mock-setup adjustment)
- No behavioral change: identical CLI behavior, error messages, and exit codes

**Tests**:
- Run `go test ./cmd -run TestHooks` and confirm all pass
- Run `go test ./... ` and confirm no regressions
