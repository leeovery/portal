# Investigation: Resume Hooks Lost On Server Restart

## Symptoms

### Problem Description

**Expected behavior:**
Resume hooks registered in `hooks.json` should survive tmux server restarts and fire when Portal reopens sessions.

**Actual behavior:**
`CleanStale()` removes all hooks from `hooks.json` after a server restart because the old pane IDs no longer exist in the new tmux server. Hooks are destroyed before they get a chance to execute.

### Manifestation

- Hooks silently vanish from `hooks.json` after tmux server kill/restart
- Portal reopens with "No sessions available" in the project picker
- Resume hooks never fire — session continuity is broken
- No error messages or warnings — the deletion is silent

### Reproduction Steps

1. Have two or more Claude Code sessions running in tmux panes, each with an on-resume hook registered (e.g., pane `%0`, `%1`)
2. Kill the tmux server (`tmux kill-server`) or reboot
3. Reopen Portal — project picker shows "No sessions available"
4. Inspect `hooks.json` — entries have been removed

**Reproducibility:** Always

### Environment

- **Affected environments:** Local (any system using tmux + Portal)
- **User conditions:** Multiple panes with registered resume hooks
- **First observed:** 2026-04-01, on freshly installed MacBook Pro — first real test of this functionality on new hardware
- **Workaround:** None — hooks are destroyed before they can be used
- **History:** Likely always been this way; never tested against a server restart scenario before

### Impact

- **Severity:** High
- **Scope:** All users relying on resume hooks for session continuity
- **Business impact:** Core workflow broken — resume hooks are the mechanism for restoring Claude Code sessions after restart

### References

- Inbox bug report: `.workflows/.inbox/.archived/bugs/2026-04-01--resume-hooks-lost-on-server-restart.md`

---

## Analysis

### Initial Hypotheses

`ExecuteHooks()` calls `store.CleanStale()` unconditionally at the start of execution. After a server restart, the new tmux server has no knowledge of old pane IDs — they're ephemeral identifiers that reset. `CleanStale()` sees all old entries as stale and deletes them.

### Code Trace

**Entry point:** `cmd/attach.go:40-42` / `cmd/open.go` — both call `hookExecutor(name)` which invokes `hooks.ExecuteHooks`.

**Execution path:**
1. `cmd/hook_executor.go:14-18` — `buildHookExecutor` creates a closure calling `hooks.ExecuteHooks(sessionName, client, store)`
2. `internal/hooks/executor.go:66-68` — `ExecuteHooks` calls `tmux.ListAllPanes()`, then `store.CleanStale(livePanes)`
3. `internal/tmux/tmux.go:235-241` — `ListAllPanes()` runs `tmux list-panes -a -F #{pane_id}`. **On error (no server/no sessions), returns `[]string{}, nil`** — empty slice, nil error
4. `internal/hooks/executor.go:66` — `err == nil` is always true (ListAllPanes never returns error), so `CleanStale` always runs
5. `internal/hooks/store.go:130-159` — `CleanStale(livePaneIDs)` builds a set from `livePaneIDs`, keeps only hooks whose pane ID is in the set, removes the rest
6. With empty `livePaneIDs` after server restart → every hook entry is "stale" → all hooks deleted from disk

**Key files involved:**
- `internal/hooks/executor.go` — orchestrates cleanup + hook execution, no guard against empty live panes
- `internal/hooks/store.go` — `CleanStale` faithfully removes anything not in the live set
- `internal/tmux/tmux.go` — `ListAllPanes` swallows errors, returns empty slice
- `cmd/clean.go:77-80` — **has the fix already**: skips cleanup when `len(livePanes) == 0` with existing hooks. This guard was not replicated in `ExecuteHooks`.

### Root Cause

`ExecuteHooks()` in `executor.go:66-68` calls `CleanStale()` with the result of `ListAllPanes()` without checking whether the live pane list is empty. After a tmux server restart, `ListAllPanes()` returns an empty slice (no error) because the new server has no sessions yet. `CleanStale()` interprets this as "no panes exist" and removes all hook entries from `hooks.json`.

**Why this happens:**
The cleanup logic assumes that if `ListAllPanes` succeeds, the returned list is a reliable view of all panes. But `ListAllPanes` never returns an error — it swallows errors and returns an empty slice. An empty result is ambiguous: it could mean "no panes exist" or "the server just restarted and sessions haven't been restored yet."

### Contributing Factors

- `ListAllPanes()` (`tmux.go:237-238`) swallows errors and returns `([]string{}, nil)` — the caller can't distinguish "no panes" from "tmux not ready"
- `CleanStale` is a pure filter with no awareness of whether an empty live set is meaningful
- The `clean` command (`cmd/clean.go:77-80`) already has the correct guard (`if len(livePanes) == 0 { return nil }`) but this wasn't replicated in `ExecuteHooks`
- The cleanup in `ExecuteHooks` is "best-effort" (errors silently ignored) but the consequence of incorrect cleanup is data loss, not just a missed cleanup

### Why It Wasn't Caught

- The test "no tmux server running skips cleanup gracefully" (`executor_test.go:537-568`) explicitly **validates the buggy behavior**: it passes an empty live-panes list to `CleanStale` and asserts it was called, effectively testing that hooks would be wiped
- The mock-based test doesn't capture the real-world consequence because `CleanStale` is mocked and doesn't actually remove data
- No integration test covers the server-restart scenario end-to-end
- The feature was implemented on a machine where the server was always running, so the restart path was never exercised until a fresh MacBook install

### Blast Radius

**Directly affected:**
- `internal/hooks/executor.go` — `ExecuteHooks` cleanup path
- All resume hook functionality — hooks are permanently destroyed on any server restart

**Potentially affected:**
- `cmd/clean.go` — already has the guard, but shares the pattern. Worth verifying consistency
- Any future code that calls `CleanStale` with `ListAllPanes` output

---

## Fix Direction

### Chosen Approach

Add an empty-pane guard in `ExecuteHooks` (`executor.go:66-68`), matching the existing pattern in `clean.go:77-80`. Skip `CleanStale` when `len(livePanes) == 0`.

**Deciding factor:** Minimal change, proven pattern already exists in the codebase, and stale entries that survive are harmless (they won't match any live panes during hook execution at lines 91-93).

### Options Explored

Only one approach — the guard pattern from `clean.go` is the obvious and correct fix. No alternatives needed.

### Discussion

Straightforward bug with a clear fix. The guard pattern is already established in `clean.go`, so this is about consistency. User confirmed findings matched understanding and agreed with the fix direction without discussion.

### Testing Recommendations

- Update the existing test "no tmux server running skips cleanup gracefully" (`executor_test.go:537-568`) to assert `CleanStale` is **NOT** called when `livePanes` is empty
- Add a test that verifies hooks survive when `ListAllPanes` returns an empty list (the post-restart scenario)

### Risk Assessment

- **Fix complexity:** Low
- **Regression risk:** Low — the guard only skips cleanup when there's nothing to clean against; all other paths unchanged
- **Recommended approach:** Regular release

---

## Notes

- The synthesis agent independently confirmed the root cause with high confidence and no gaps
- `clean.go` guard was added during the `resume-sessions-after-reboot` feature work but wasn't applied to `ExecuteHooks` at the same time
