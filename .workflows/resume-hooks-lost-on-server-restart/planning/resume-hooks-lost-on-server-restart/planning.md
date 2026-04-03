# Plan: Resume Hooks Lost On Server Restart

## Phase 1: Empty-Pane Guard
status: approved
approved_at: 2026-04-03

**Goal**: Fix the immediate data loss bug where `ExecuteHooks` calls `CleanStale` with an empty pane list after server restart, deleting all hook entries. Surgical fix matching the existing guard in `cmd/clean.go:77-80`.

**Why this order**: Critical safety fix — stops hooks from being deleted on server restart with minimal code change and minimal risk. Must land before the structural key migration because Problem 2's changes are meaningless if hooks keep getting wiped. Also fixes the existing test at `executor_test.go:537-568` which currently asserts incorrect behavior (expects `CleanStale` to be called with empty list).

**Acceptance**:
- [ ] Existing test "no tmux server running skips cleanup gracefully" updated to assert `CleanStale` is NOT called when `ListAllPanes` returns empty
- [ ] New test verifies hook data survives when `ListAllPanes` returns empty (post-restart, pre-resurrect scenario)
- [ ] `ExecuteHooks` skips `CleanStale` when `livePanes` has length zero
- [ ] All existing tests pass without modification (beyond the corrected test)

### Tasks

| # | ID | Task | Edge Cases | Status |
|---|-----|------|------------|--------|
| 1 | resume-hooks-lost-on-server-restart-1-1 | Guard CleanStale from empty pane list in ExecuteHooks | ListAllPanes returns nil vs empty slice; ListAllPanes error path (no regression) | approved |

## Phase 2: Structural Key Infrastructure and Pane Querying
status: approved
approved_at: 2026-04-03

**Goal**: Introduce the structural key model (`session_name:window_index.pane_index`) at the tmux and hooks-store layers. Change `ListPanes` and `ListAllPanes` to return structural keys instead of pane IDs. Add new `tmux.Client` method to resolve a pane ID (`$TMUX_PANE`) to its structural key. Update `MarkerName` to accept structural keys. Update `Hook` struct and `Store` method semantics.

**Why this order**: The tmux layer and store are the foundation all consumers depend on. The executor, command handlers, and clean command all consume `ListPanes`, `ListAllPanes`, `CleanStale`, and `MarkerName`. These must be updated before their callers can switch to structural keys.

**Acceptance**:
- [ ] `ListPanes(sessionName)` returns structural keys in `session:window.pane` format instead of pane IDs
- [ ] `ListAllPanes()` returns structural keys in `session:window.pane` format instead of pane IDs
- [ ] New method on `tmux.Client` resolves a pane ID (e.g., `%0`) to its structural key via `tmux display-message`
- [ ] `MarkerName` produces markers in `@portal-active-{structural_key}` format
- [ ] `Hook` struct field semantics updated from pane ID to structural key
- [ ] `CleanStale` parameter renamed and works with structural keys
- [ ] Unit tests cover structural key construction, resolution method, and updated `ListPanes`/`ListAllPanes` output parsing

### Tasks

| # | ID | Task | Edge Cases | Status |
|---|-----|------|------------|--------|
| 1 | resume-hooks-lost-on-server-restart-2-1 | Update ListPanes and ListAllPanes to Return Structural Keys | session names with colons or dots; multi-window multi-pane output | pending |
| 2 | resume-hooks-lost-on-server-restart-2-2 | Add ResolveStructuralKey Method to tmux.Client | invalid pane ID; tmux command failure | pending |
| 3 | resume-hooks-lost-on-server-restart-2-3 | Update Hook Struct and Store Semantics for Structural Keys | — | pending |
| 4 | resume-hooks-lost-on-server-restart-2-4 | Update MarkerName Format and Executor Tests to Structural Keys | — | pending |

## Phase 3: Consumer Migration to Structural Keys
status: approved
approved_at: 2026-04-03

**Goal**: Update all consumers of the structural key infrastructure: `ExecuteHooks` (executor), `hooks set`, `hooks rm`, `hooks list` (cmd/hooks.go), and `clean` command (cmd/clean.go). All hook registration, execution, removal, listing, and cleanup now use structural keys end-to-end.

**Why this order**: With the tmux layer and store returning structural keys (Phase 2), the consumers can now be migrated. Separated from Phase 2 because it touches different packages (cmd layer vs internal layer), has different testing patterns, and the combined scope would exceed 8 tasks.

**Acceptance**:
- [ ] `hooks set` resolves `$TMUX_PANE` to a structural key and stores hooks under that key
- [ ] `hooks rm` resolves `$TMUX_PANE` to a structural key and removes hooks/markers using that key
- [ ] `hooks list` displays structural keys instead of pane IDs in output
- [ ] `ExecuteHooks` matches hooks by structural key, sends keys using structural key as tmux `-t` target, and sets volatile markers using structural key format
- [ ] `clean` command works with structural key model for hook cleanup
- [ ] Multi-pane test: session with multiple panes has independent hook entries keyed by distinct structural positions
- [ ] Graceful no-op test: hooks with structural keys that don't match any live panes produce no errors
- [ ] All existing tests updated to use structural key values
- [ ] Full test suite passes: `go test ./...`
