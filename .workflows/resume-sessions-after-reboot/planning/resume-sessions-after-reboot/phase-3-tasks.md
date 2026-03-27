---
phase: 3
phase_name: Stale Hook Cleanup
total: 4
---

## resume-sessions-after-reboot-3-1 | approved

### Task 1: ListAllPanes Tmux Method

**Problem**: The stale hook cleanup needs to know which pane IDs currently exist across the entire tmux server. Phase 2 added `ListPanes(sessionName string)` which returns panes for a single session, but cleanup needs all live pane IDs globally to determine which hook entries reference panes that no longer exist. There is no method on `tmux.Client` that returns all panes across all sessions.

**Solution**: Add a `ListAllPanes() ([]string, error)` method to `tmux.Client` in `internal/tmux/tmux.go`. This runs `tmux list-panes -a -F "#{pane_id}"` where the `-a` flag lists panes across all sessions. The method follows the same error-handling pattern as `ListSessions`: when no tmux server is running (Commander returns an error), return an empty slice and nil error rather than propagating the error. This keeps the caller from needing to handle server-down as a special case.

**Outcome**: `tmux.Client` has a `ListAllPanes()` method that returns all live pane IDs across the entire tmux server as a string slice (e.g., `["%0", "%1", "%3"]`). When no server is running or the server has no sessions, it returns an empty slice and nil error. Tests verify the tmux command arguments, output parsing, and error handling.

**Do**:
- Add to `internal/tmux/tmux.go`:
  - `ListAllPanes() ([]string, error)` method on `*Client`:
    1. Runs `c.cmd.Run("list-panes", "-a", "-F", "#{pane_id}")`
    2. If the Commander returns an error (no server running, or any other failure), return `([]string{}, nil)` -- follows the same pattern as `ListSessions` which returns `([]Session{}, nil)` on error
    3. If the output is empty string, return `([]string{}, nil)`
    4. Splits output by newline, trims each line, filters out empty strings
    5. Returns the resulting slice of pane ID strings
- Add tests to `internal/tmux/tmux_test.go` in a new `TestListAllPanes` function using the existing `MockCommander` pattern. Follow the table-driven test style used by `TestListSessions`.

**Acceptance Criteria**:
- [ ] `ListAllPanes` calls `tmux list-panes -a -F #{pane_id}` via the Commander
- [ ] `ListAllPanes` returns pane IDs as a string slice (e.g., `["%0", "%1", "%3"]`)
- [ ] `ListAllPanes` returns an empty slice (not nil) and nil error when the Commander returns an error (no server running)
- [ ] `ListAllPanes` returns an empty slice when the output is empty (server with no sessions)
- [ ] `ListAllPanes` trims whitespace from each line and filters out empty lines
- [ ] All tests pass: `go test ./internal/tmux/...`

**Tests**:
- `"ListAllPanes returns pane IDs across multiple sessions"`
- `"ListAllPanes returns single pane ID"`
- `"ListAllPanes returns empty slice when no tmux server running"`
- `"ListAllPanes returns empty slice when output is empty"`
- `"ListAllPanes calls list-panes with -a flag and pane_id format"`
- `"ListAllPanes trims whitespace from output lines"`

**Edge Cases**:
- No tmux server running returns empty slice (not error) -- when the Commander returns an error (e.g., `"no server running on /tmp/tmux-501/default"`), `ListAllPanes` returns `([]string{}, nil)`. This mirrors `ListSessions` behavior and allows cleanup callers to treat "no server" as "no live panes" without error handling.
- Server with no sessions returns empty slice -- when the server is running but has no sessions (possible briefly after server start), the Commander returns an empty string. `ListAllPanes` returns `([]string{}, nil)`.

**Context**:
> The spec says: "When Portal reads hooks (during portal open), cross-reference pane IDs against live tmux panes. Prune entries for panes that don't exist." This requires knowing all live pane IDs. The `-a` flag on `tmux list-panes` returns panes from all sessions, which is what we need for global cleanup. The error-swallowing pattern (returning empty slice on Commander error) follows `ListSessions` exactly and is important because cleanup should not fail when there is no tmux server -- it should simply treat the situation as "no live panes" and remove all stale entries.

**Spec Reference**: `Stale Registration Cleanup` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-3-2 | approved

### Task 2: Hook Store CleanStale Method

**Problem**: Over time, hooks.json accumulates entries for pane IDs that no longer exist (panes closed, sessions killed, etc.). These stale entries waste space and could cause confusion. The hook store needs a way to prune entries for panes that are no longer live, mirroring the existing `CleanStale()` pattern in `internal/project/store.go`.

**Solution**: Add a `CleanStale(livePaneIDs []string) ([]string, error)` method to the `hooks.Store` in `internal/hooks/store.go`. The method accepts a slice of currently live pane IDs (keeping the store decoupled from tmux), loads the hook map, identifies entries whose pane IDs are not in the live set, removes them, saves if anything changed, and returns the list of removed pane IDs. This mirrors the `project.Store.CleanStale()` pattern: load, partition into kept/removed, save only when removals occurred, return removed items.

**Outcome**: The hook store has a `CleanStale` method that prunes entries for panes not in the provided live set. It returns removed pane IDs for reporting, only writes to disk when at least one entry was removed, and handles empty stores and edge cases gracefully.

**Do**:
- Add to `internal/hooks/store.go`:
  - `CleanStale(livePaneIDs []string) ([]string, error)` method on `*Store`:
    1. Call `s.Load()` to get the current hook map. If error, return `(nil, fmt.Errorf("failed to load hooks: %w", err))`.
    2. If the map is empty, return `([]string{}, nil)` -- nothing to clean.
    3. Build a set (map[string]bool) from the `livePaneIDs` slice for O(1) lookup.
    4. Iterate over the hook map keys (pane IDs). For each key:
       - If the pane ID is in the live set, keep it.
       - If the pane ID is NOT in the live set, add it to the `removed` slice and delete it from the hook map.
    5. If `len(removed) == 0`, return `([]string{}, nil)` -- no changes needed, skip the save.
    6. Call `s.Save(kept)` where `kept` is the hook map with stale entries removed. If save error, return `(nil, fmt.Errorf("failed to save after cleaning stale hooks: %w", err))`.
    7. Return `(removed, nil)` -- the list of pane IDs that were removed.
  - Note: The method builds a new map of kept entries rather than mutating the original, for clarity. Alternatively, deleting from the original map and passing it to Save is acceptable since Load returns a fresh copy.
- Add tests to `internal/hooks/store_test.go` in a new `TestCleanStale` function. Tests should create a temp directory, seed a hooks.json file with known entries, call `CleanStale` with various live pane ID sets, and verify both the return value and the persisted file contents.

**Acceptance Criteria**:
- [ ] `CleanStale` accepts a slice of live pane IDs and removes hook entries for panes not in that slice
- [ ] `CleanStale` returns a slice of removed pane IDs (e.g., `["%3", "%7"]`)
- [ ] `CleanStale` returns an empty slice when the store is empty (no hooks to clean)
- [ ] `CleanStale` returns an empty slice when all pane IDs are in the live set (nothing to remove)
- [ ] `CleanStale` removes all entries when no pane IDs are in the live set (empty live set)
- [ ] `CleanStale` only saves the file when at least one entry was removed
- [ ] The hooks.json file reflects the cleaned state after `CleanStale` completes
- [ ] All tests pass: `go test ./internal/hooks/...`

**Tests**:
- `"CleanStale removes entries for panes not in live set"`
- `"CleanStale returns removed pane IDs"`
- `"CleanStale returns empty slice when store is empty"`
- `"CleanStale returns empty slice when all panes are live"`
- `"CleanStale removes all entries when live set is empty"`
- `"CleanStale only saves file when entries were removed"`
- `"CleanStale keeps entries for live panes"`
- `"CleanStale handles mix of live and stale panes"`
- `"CleanStale file reflects cleaned state"`

**Edge Cases**:
- Empty store returns no removals -- `Load()` returns an empty map, `CleanStale` returns `([]string{}, nil)` immediately without calling `Save`.
- All panes still live returns no removals -- every pane ID in the hook map is present in the live set. `removed` is empty, `Save` is not called, returns `([]string{}, nil)`.
- No live panes removes all entries -- `livePaneIDs` is an empty slice (or all hook pane IDs are absent from it). Every entry is removed. `Save` is called with an empty map. Returns all pane IDs as removed.
- File only saved when at least one entry removed -- verified by checking that when all panes are live, the file's modification time does not change (or by seeding a file and verifying its content is unchanged). This mirrors the `project.Store.CleanStale()` optimization.

**Context**:
> The spec says: "When Portal reads hooks (during portal open), cross-reference pane IDs against live tmux panes. Prune entries for panes that don't exist. Invisible to the user." The `CleanStale` method on `project.Store` (in `internal/project/store.go`) is the model to follow: load all entries, partition into kept and removed, save only when removals occurred, return removed items. The key design decision is that `CleanStale` accepts live pane IDs as input rather than querying tmux itself -- this keeps the store package decoupled from tmux and makes testing straightforward (no tmux mocks needed in store tests).

**Spec Reference**: `Stale Registration Cleanup` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-3-3 | approved

### Task 3: Lazy Cleanup in Execution Flow

**Problem**: Stale hook entries (for panes that no longer exist) should be pruned automatically when Portal reads hooks during the connection flow. The spec says this cleanup should be "invisible to the user" and happen lazily, mirroring how the TUI already calls `projectStore.CleanStale()` every time it loads projects. Currently, `ExecuteHooks` in `internal/hooks/executor.go` (created in Phase 2 Task 2) reads the hook store but does not prune stale entries.

**Solution**: Add a stale cleanup step at the beginning of the `ExecuteHooks` function in `internal/hooks/executor.go`, before the pane-matching logic. The cleanup needs to list all live panes (via a new `AllPaneLister` interface) and pass them to the hook store's `CleanStale` method (via a new `HookCleaner` interface). Both operations are best-effort: if listing all panes fails or cleanup fails, execution continues without cleanup. The cleanup runs before the pane-matching logic so that stale entries are removed before they are checked against session panes.

**Outcome**: Every call to `ExecuteHooks` automatically prunes stale hook entries before executing hooks. Cleanup errors are silently ignored and never block hook execution. The hook store stays clean over time without explicit user action.

**Do**:
- Add to `internal/hooks/executor.go`:
  - `AllPaneLister` interface: `ListAllPanes() ([]string, error)` -- satisfied by `*tmux.Client`
  - `HookCleaner` interface: `CleanStale(livePaneIDs []string) ([]string, error)` -- satisfied by `*hooks.Store`
  - Modify `ExecuteHooks` signature to accept two additional parameters:
    - Change from: `func ExecuteHooks(sessionName string, lister PaneLister, loader HookLoader, sender KeySender, checker OptionChecker)`
    - Change to: `func ExecuteHooks(sessionName string, lister PaneLister, loader HookLoader, sender KeySender, checker OptionChecker, allLister AllPaneLister, cleaner HookCleaner)`
  - Insert cleanup logic at the very beginning of `ExecuteHooks`, before the existing `loader.Load()` call:
    1. Call `allLister.ListAllPanes()` -- if error, skip cleanup (do not return; continue to hook execution)
    2. Call `cleaner.CleanStale(livePanes)` -- if error, skip (ignore silently; continue to hook execution)
    3. The cleanup has no return value that affects the rest of the function. It is purely a side effect that prunes the store.
  - The cleanup step is best-effort and self-contained. It does not affect the subsequent `loader.Load()` call (the load will read the now-cleaned store).
- Update all callers of `ExecuteHooks` (created in Phase 2 Tasks 3, 4, 5):
  - In `cmd/hook_executor.go` (or wherever `buildHookExecutor` is defined): the `HookExecutorFunc` returned by `buildHookExecutor` must now also pass `*tmux.Client` (as `AllPaneLister`) and `*hooks.Store` (as `HookCleaner`) to `ExecuteHooks`.
  - Update the `buildHookExecutor` function: it already creates `hooks.NewStore(path)` and has access to `*tmux.Client`. Pass `client` as `AllPaneLister` and `store` as `HookCleaner` in the `ExecuteHooks` call.
- Update all existing tests for `ExecuteHooks` in `internal/hooks/executor_test.go`:
  - Add mock implementations of `AllPaneLister` and `HookCleaner` to each test
  - For tests that are not specifically testing cleanup behavior, use no-op mocks: `AllPaneLister` returns `([]string{}, nil)` and `HookCleaner` returns `([]string{}, nil)`. This means cleanup effectively does nothing and existing test assertions remain valid.
- Add new tests in `internal/hooks/executor_test.go` specifically for cleanup behavior:
  - Verify cleanup runs (mock `CleanStale` is called)
  - Verify `ListAllPanes` error does not block hook execution
  - Verify `CleanStale` error does not block hook execution
  - Verify cleanup runs before `loader.Load()` (by checking call order in mocks)

**Acceptance Criteria**:
- [ ] `ExecuteHooks` calls `AllPaneLister.ListAllPanes()` at the start, before any other logic
- [ ] `ExecuteHooks` passes the live pane IDs to `HookCleaner.CleanStale()`
- [ ] `ListAllPanes` error does not block hook execution (silently ignored)
- [ ] `CleanStale` error does not block hook execution (silently ignored)
- [ ] Cleanup runs before `loader.Load()` so the load reads the cleaned store
- [ ] All existing `ExecuteHooks` tests continue to pass with no-op cleanup mocks
- [ ] All callers of `ExecuteHooks` (in cmd package) are updated to pass the new parameters
- [ ] All tests pass: `go test ./internal/hooks/...` and `go test ./cmd/...`

**Tests**:
- `"cleanup calls ListAllPanes and CleanStale before hook execution"`
- `"ListAllPanes error skips cleanup and continues hook execution"`
- `"CleanStale error skips cleanup and continues hook execution"`
- `"cleanup runs before loader.Load"`
- `"no tmux server running skips cleanup gracefully"`
- `"all existing ExecuteHooks tests pass with cleanup mocks"`

**Edge Cases**:
- No tmux server running skips cleanup gracefully -- `AllPaneLister.ListAllPanes()` returns `([]string{}, nil)` (no error, per Task 1 behavior). `CleanStale` receives an empty live set and removes all stale entries. This is correct: if there is no tmux server, all hook entries are stale.
- Cleanup errors do not block hook execution -- if `ListAllPanes` returns an error (unexpected, since Task 1 swallows errors) or `CleanStale` returns an error (disk write failure), execution continues normally. The cleanup step has no bearing on whether hooks execute.
- Cleanup runs before pane-matching logic -- the cleanup step is the very first thing in `ExecuteHooks`, before `loader.Load()`. This means the load sees the cleaned store. If pane `%99` had a stale entry and was removed by cleanup, the subsequent load will not see it.

**Context**:
> The spec says: "When Portal reads hooks (during portal open), cross-reference pane IDs against live tmux panes. Prune entries for panes that don't exist. Invisible to the user." This mirrors the TUI's lazy cleanup pattern where `_, _ = m.projectStore.CleanStale()` is called in `loadProjects()` (see `internal/tui/model.go` line 568). The return value is discarded and errors are ignored -- the cleanup is purely a housekeeping side effect. The same pattern applies here: `_, _ = cleaner.CleanStale(livePanes)`. Note that the spec also says "Adding hook cleanup to xctl clean is a natural fit" -- that is Task 4. This task handles the lazy/automatic cleanup path.

**Spec Reference**: `Stale Registration Cleanup` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`

---

## resume-sessions-after-reboot-3-4 | approved

### Task 4: Extend Clean Command with Hook Cleanup

**Problem**: The `xctl clean` command (in `cmd/clean.go`) currently only removes stale projects whose directories no longer exist. The spec says it should also remove stale hook entries for panes that no longer exist. Users running `xctl clean` expect it to clean up all Portal-managed stale data.

**Solution**: Extend the `clean` command's `RunE` to also load the hook store, query live panes via `tmux.Client.ListAllPanes()`, call `hooks.Store.CleanStale(livePanes)`, and print a removal line for each pruned pane. The hook cleanup runs after the existing project cleanup. Since `clean` bypasses tmux bootstrap (`skipTmuxCheck`), it creates its own `tmux.Client` for the `ListAllPanes` call -- same pattern as `hooks set`/`hooks rm`. When no tmux server is running, `ListAllPanes` returns an empty slice (per Task 1) which means all hook entries would be stale -- however, it is more user-friendly to skip hook cleanup entirely when no server is running (the user may have hooks registered for a server they will start later). When the hooks file does not exist, the store returns an empty map and cleanup produces no output.

**Outcome**: Running `xctl clean` prints both stale project removals (existing behavior) and stale hook removals (new behavior). Each removed hook entry is printed as `Removed stale hook: %3 (on-resume)`. When there are no stale hooks or no hooks file, no hook-related output is produced.

**Do**:
- Modify `cmd/clean.go`:
  - Add a `cleanDeps` package-level DI struct (following the pattern from `hooksDeps` in `cmd/hooks.go`):
    - `type CleanDeps struct { AllPaneLister AllPaneLister }` where `AllPaneLister` is an interface with `ListAllPanes() ([]string, error)` (can reuse the interface from `internal/hooks/executor.go` or define a local one in `cmd/clean.go` for simplicity -- a local definition is preferred since the cmd package should define its own small interfaces)
    - `var cleanDeps *CleanDeps` package-level variable
    - Helper to get the `AllPaneLister`: if `cleanDeps != nil && cleanDeps.AllPaneLister != nil`, use it; otherwise, create `tmux.NewClient(&tmux.RealCommander{})`.
  - Extend the existing `cleanCmd.RunE`:
    - After the existing project cleanup code (which calls `store.CleanStale()` and prints removed projects), add:
    1. Call `hooksFilePath()` to get the hooks file path. If error, return error.
    2. Create `hooks.NewStore(hooksPath)`
    3. Get the `AllPaneLister` (from DI or real client)
    4. Call `lister.ListAllPanes()` -- if error, skip hook cleanup silently (do not return error; just skip the hook section). Print nothing for hooks.
    5. Call `hookStore.CleanStale(livePanes)` -- if error, return error (disk errors during explicit cleanup should be reported to the user, unlike the lazy path).
    6. For each removed pane ID, iterate over its former events and print: `fmt.Fprintf(w, "Removed stale hook: %s (%s)\n", paneID, event)`. However, `CleanStale` only returns pane IDs (not the event details). To provide more useful output, either:
       - Option A: Change the loop to just print pane IDs: `fmt.Fprintf(w, "Removed stale hook: %s\n", paneID)`
       - Option B: Modify `CleanStale` to return richer data. This adds complexity to the store method.
       - Use Option A for simplicity. The output format is: `Removed stale hook: %3\n` per removed pane.
    7. Return nil.
  - Add `hooksFilePath` helper if not already accessible from `cmd/hooks.go` (it was created in Phase 1 Task 3 in `cmd/hooks.go` and should be accessible since both files are in the `cmd` package).
  - Add `"github.com/leeovery/portal/internal/hooks"` to imports.
- Extend `cmd/clean_test.go`:
  - Add new tests that exercise the hook cleanup path:
    - Create a temp hooks.json file via `t.Setenv("PORTAL_HOOKS_FILE", ...)` (same pattern as `PORTAL_PROJECTS_FILE`)
    - Inject `cleanDeps` with a mock `AllPaneLister` that returns a controlled set of live pane IDs
    - Use `t.Cleanup(func() { cleanDeps = nil })` to restore after each test
    - Verify output includes both project removal lines and hook removal lines
  - Existing project-only tests must continue to pass. They will need a no-op or nil `cleanDeps` (when `cleanDeps` is nil, the real tmux client is created, but since these tests do not set `PORTAL_HOOKS_FILE`, the hooks file will not exist and no hook output will be produced). Alternatively, inject a mock lister returning an empty slice.

**Acceptance Criteria**:
- [ ] `xctl clean` prints stale hook removal messages alongside stale project removal messages
- [ ] Hook removal output format is: `Removed stale hook: %3\n` per removed pane
- [ ] When no hooks file exists, no hook-related output is produced (no error)
- [ ] When no tmux server is running (`ListAllPanes` returns empty slice or error), hook cleanup is skipped silently
- [ ] When all hook panes are still live, no hook removal output is produced
- [ ] Existing project cleanup behavior is unchanged
- [ ] `cleanDeps` DI struct allows test injection of the `AllPaneLister`
- [ ] All existing clean tests continue to pass
- [ ] All tests pass: `go test ./cmd -run TestClean`

**Tests**:
- `"removes stale hooks and prints removal messages"`
- `"no tmux server running produces no hook removal output"`
- `"hooks file missing produces no hook removal output"`
- `"all hooks panes still live produces no hook removal output"`
- `"both project and hook removals printed together"`
- `"only hooks removed when no stale projects"`
- `"only projects removed when no stale hooks"`
- `"existing clean tests still pass"`

**Edge Cases**:
- No tmux server running produces no hook removal output -- `ListAllPanes` returns an error or empty slice. When it returns an error, hook cleanup is skipped entirely (the command does not know which panes are live, so it cannot determine staleness). When it returns an empty slice (per Task 1 behavior of swallowing errors), all entries would appear stale. To avoid incorrectly removing hooks when the server is simply not running, check: if `ListAllPanes` returns an empty slice AND the server is not confirmed running, skip cleanup. The simplest approach: if `ListAllPanes` returns an error, skip. Since Task 1 swallows errors into empty slices, the clean command should call `ListAllPanes` and if the result is an empty slice, also verify the server is running via a `ServerRunning()` check. If the server is not running, skip hook cleanup entirely. This prevents removing all hooks when the user simply has no tmux server running at the moment.
- Hooks file missing produces no hook removal output -- `hooks.Store.Load()` returns an empty map for a missing file. `CleanStale` receives an empty map and returns no removals. No output.
- Both project and hook removals printed together -- the output interleaves naturally: project removals first (existing code), then hook removals (new code). Example output:
  ```
  Removed stale project: myapp (/path/to/myapp)
  Removed stale hook: %3
  Removed stale hook: %7
  ```

**Context**:
> The spec says: "Adding hook cleanup to xctl clean is a natural fit -- it already says 'remove stale projects whose directories no longer exist.' Extending to 'remove hook entries for panes that no longer exist' is semantically identical." The `clean` command currently creates a `project.Store` via `loadProjectStore()` and calls `CleanStale()`. The hook cleanup follows the same structure: create a `hooks.Store` via `loadHookStore()`, get live pane IDs, call `CleanStale(livePanes)`. A key design decision: when there is no tmux server running, the clean command should skip hook cleanup rather than removing all entries. This is because `clean` is an explicit user action (unlike the lazy path in Task 3 which runs during `portal open` where the server is guaranteed to be running), and the user may have hooks registered for a server they will start later. The `clean` command is in `skipTmuxCheck` so it does not require a running tmux server.

**Spec Reference**: `Stale Registration Cleanup` section of `.workflows/resume-sessions-after-reboot/specification/resume-sessions-after-reboot/specification.md`
