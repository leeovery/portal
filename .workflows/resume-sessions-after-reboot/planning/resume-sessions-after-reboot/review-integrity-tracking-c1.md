---
status: complete
created: 2026-03-27
cycle: 1
phase: Plan Integrity Review
topic: Resume Sessions After Reboot
---

# Review Tracking: Resume Sessions After Reboot - Integrity

## Findings

### 1. Clean Command Hook Cleanup Has Unresolved Design Decision for Server-Not-Running Case

**Severity**: Critical
**Plan Reference**: Phase 3 / Task 4 (resume-sessions-after-reboot-3-4)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 3-4 identifies a critical edge case: when no tmux server is running, `ListAllPanes` returns `([]string{}, nil)` (per Task 3-1's error-swallowing behavior), which would make all hook entries appear stale and get deleted. The task proposes solving this with a `ServerRunning()` check, but no such method exists on `tmux.Client` and no task in the plan creates one. The implementer would have to invent this capability.

The simplest fix is to change the approach: instead of calling `ListAllPanes` (which swallows errors), the clean command should call the Commander directly or use an approach that distinguishes "no server" from "server running with no panes." Since Task 3-1's `ListAllPanes` is specifically designed to swallow errors (matching `ListSessions` behavior), the clean command needs a different strategy.

The cleanest solution: have the clean command call `ListAllPanes` but also attempt a lightweight tmux check first (e.g., `tmux list-sessions` or `tmux info`). However, this adds a method not in the plan. A simpler alternative: skip hook cleanup when `ListAllPanes` returns an empty slice AND the hooks file has entries. This avoids needing a new tmux method but is a heuristic. The most robust approach is to add a `ServerRunning() bool` method to the plan.

The proposed fix below adds a `ServerRunning` check to the `AllPaneLister` interface used by the clean command, resolved via the existing Commander infrastructure.

**Current**:
```markdown
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
```

```markdown
**Edge Cases**:
- No tmux server running produces no hook removal output -- `ListAllPanes` returns an error or empty slice. When it returns an error, hook cleanup is skipped entirely (the command does not know which panes are live, so it cannot determine staleness). When it returns an empty slice (per Task 1 behavior of swallowing errors), all entries would appear stale. To avoid incorrectly removing hooks when the server is simply not running, check: if `ListAllPanes` returns an empty slice, also verify the server is running via a `ServerRunning()` check. If the server is not running, skip hook cleanup entirely. This prevents removing all hooks when the user simply has no tmux server running at the moment.
```

**Proposed**:
```markdown
**Do**:
- Modify `cmd/clean.go`:
  - Add a `cleanDeps` package-level DI struct (following the pattern from `hooksDeps` in `cmd/hooks.go`):
    - Define a local interface in `cmd/clean.go`: `type cleanPaneLister interface { ListAllPanes() ([]string, error) }` -- satisfied by `*tmux.Client`
    - `type CleanDeps struct { PaneLister cleanPaneLister }`
    - `var cleanDeps *CleanDeps` package-level variable
    - Helper to get the `cleanPaneLister`: if `cleanDeps != nil && cleanDeps.PaneLister != nil`, use it; otherwise, create `tmux.NewClient(&tmux.RealCommander{})`.
  - Extend the existing `cleanCmd.RunE`:
    - After the existing project cleanup code (which calls `store.CleanStale()` and prints removed projects), add:
    1. Call `hooksFilePath()` to get the hooks file path. If error, return error.
    2. Create `hooks.NewStore(hooksPath)`
    3. Call `hookStore.Load()` to check if any hooks exist. If the loaded map is empty, skip hook cleanup entirely (no hooks to clean, no output).
    4. Get the `cleanPaneLister` (from DI or real client)
    5. Call `lister.ListAllPanes()` -- if error, skip hook cleanup silently (do not return error; just skip the hook section). Note: `ListAllPanes` (from Task 3-1) swallows errors into empty slices, so this error check is a safety net.
    6. If `ListAllPanes` returns an empty slice AND the hook map is non-empty, skip hook cleanup silently. Rationale: when the tmux server is not running, `ListAllPanes` returns an empty slice (per Task 3-1). An empty live-pane set would incorrectly mark all hooks as stale. Since the clean command is in `skipTmuxCheck` and does not require a running server, the user may have hooks for a server they will start later. The pre-check in step 3 ensures we only reach here when hooks exist, so an empty pane list is ambiguous (server down vs. genuinely no panes). Skipping is the safe choice.
    7. If live panes are non-empty, call `hookStore.CleanStale(livePanes)` -- if error, return error (disk errors during explicit cleanup should be reported to the user, unlike the lazy path).
    8. For each removed pane ID, print: `fmt.Fprintf(w, "Removed stale hook: %s\n", paneID)`. The output format is one line per removed pane.
    9. Return nil.
  - Add `hooksFilePath` helper if not already accessible from `cmd/hooks.go` (it was created in Phase 1 Task 3 in `cmd/hooks.go` and should be accessible since both files are in the `cmd` package).
  - Add `"github.com/leeovery/portal/internal/hooks"` to imports.
```

```markdown
**Edge Cases**:
- No tmux server running produces no hook removal output -- `ListAllPanes` returns `([]string{}, nil)` (per Task 3-1 behavior). The clean command detects this: when the live pane list is empty but hooks exist in the store, it skips hook cleanup entirely rather than incorrectly removing all hooks. This is the safe default for an explicit user command where the server may simply not be running at the moment. No `ServerRunning()` method is needed; the heuristic "empty panes + non-empty hooks = skip" is sufficient because a running server with zero panes is an extremely transient state that does not warrant cleanup.
```

**Resolution**: Fixed
**Notes**: The `ServerRunning()` approach mentioned in the current task would require adding a new tmux method not in the plan. The proposed heuristic (skip when empty panes + non-empty hooks) avoids that dependency while achieving the same user-facing behavior.

---

### 2. Phase 3 Task 3 Modifies ExecuteHooks Signature Without Acknowledging Full Rework Scope

**Severity**: Important
**Plan Reference**: Phase 3 / Task 3 (resume-sessions-after-reboot-3-3)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 3-3 changes the `ExecuteHooks` function signature by adding two parameters (`AllPaneLister`, `HookCleaner`). This breaks all three callers wired in Phase 2 (Tasks 2-3, 2-4, 2-5 via `buildHookExecutor`). The task acknowledges this ("Update all callers of ExecuteHooks") but mixes two concerns: (1) implementing cleanup logic and (2) updating all callers. The caller updates are mechanical but touch multiple files across the cmd package.

The task's Do section mentions updating `buildHookExecutor` and all callers, which is correct. However, the acceptance criteria and tests do not verify that the cmd-level integration still works end-to-end (only that `go test ./cmd/...` passes). The task should be more explicit that it owns the caller updates as part of its scope, including updating any test mocks in the cmd package.

**Current**:
```markdown
**Acceptance Criteria**:
- [ ] `ExecuteHooks` calls `AllPaneLister.ListAllPanes()` at the start, before any other logic
- [ ] `ExecuteHooks` passes the live pane IDs to `HookCleaner.CleanStale()`
- [ ] `ListAllPanes` error does not block hook execution (silently ignored)
- [ ] `CleanStale` error does not block hook execution (silently ignored)
- [ ] Cleanup runs before `loader.Load()` so the load reads the cleaned store
- [ ] All existing `ExecuteHooks` tests continue to pass with no-op cleanup mocks
- [ ] All callers of `ExecuteHooks` (in cmd package) are updated to pass the new parameters
- [ ] All tests pass: `go test ./internal/hooks/...` and `go test ./cmd/...`
```

**Proposed**:
```markdown
**Acceptance Criteria**:
- [ ] `ExecuteHooks` calls `AllPaneLister.ListAllPanes()` at the start, before any other logic
- [ ] `ExecuteHooks` passes the live pane IDs to `HookCleaner.CleanStale()`
- [ ] `ListAllPanes` error does not block hook execution (silently ignored)
- [ ] `CleanStale` error does not block hook execution (silently ignored)
- [ ] Cleanup runs before `loader.Load()` so the load reads the cleaned store
- [ ] All existing `ExecuteHooks` tests continue to pass with no-op cleanup mocks
- [ ] `buildHookExecutor` in the cmd package is updated to pass `*tmux.Client` as `AllPaneLister` and `*hooks.Store` as `HookCleaner` to the new `ExecuteHooks` signature
- [ ] All cmd-package tests that mock `HookExecutorFunc` continue to pass (the func signature is unchanged; only the internal wiring changes)
- [ ] All tests pass: `go test ./internal/hooks/...` and `go test ./cmd/...`
```

**Resolution**: Fixed
**Notes**: The original criterion "All callers of ExecuteHooks (in cmd package) are updated to pass the new parameters" is correct but vague. The proposed version names the specific function (`buildHookExecutor`) and clarifies that `HookExecutorFunc` callers are unaffected (the func type signature does not change, only the internal implementation).

---

### 3. Go Map Iteration Order Claim in Executor Task

**Severity**: Minor
**Plan Reference**: Phase 2 / Task 2 (resume-sessions-after-reboot-2-2)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
The Do section says "Iterate over the loaded hook map's pane IDs (from step 1), following the JSON store's iteration order per spec." Go map iteration order is explicitly randomized. `json.Unmarshal` into a `map[string]map[string]string` does not preserve JSON key order. The spec says "Order follows pane ID iteration from the JSON store" but this is not achievable with Go's standard map type without additional work (e.g., sorting keys or using an ordered map).

Since the spec's intent is just "sequential execution" (not a specific deterministic order), and the actual order doesn't matter functionally (all qualifying panes get executed regardless), this is a documentation accuracy issue rather than a functional one. The implementer might waste time trying to preserve JSON order.

**Current**:
```markdown
    6. Iterate over the loaded hook map's pane IDs (from step 1), following the JSON store's iteration order per spec. For each pane ID in the hook map:
```

**Proposed**:
```markdown
    6. Iterate over the loaded hook map's pane IDs (from step 1). Go map iteration order is non-deterministic; this is acceptable because the spec requires sequential execution but not a specific ordering. For each pane ID in the hook map:
```

**Resolution**: Fixed
**Notes**: Minor clarification that prevents an implementer from trying to preserve JSON key order unnecessarily.

---
