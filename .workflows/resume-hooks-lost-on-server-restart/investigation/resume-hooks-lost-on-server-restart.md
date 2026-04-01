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

Two distinct problems prevent resume hooks from working after a tmux server restart:

**Problem 1 — Hook deletion (data loss):**
`ExecuteHooks()` in `executor.go:66-68` calls `CleanStale()` with the result of `ListAllPanes()` without checking whether the live pane list is empty. After a server restart, `ListAllPanes()` returns an empty slice (no error, it swallows errors at `tmux.go:237-238`). `CleanStale()` interprets this as "no panes exist" and removes all hook entries from `hooks.json`.

**Problem 2 — Pane ID instability (broken mapping):**
Even if hooks survive the restart (problem 1 fixed), the tracking mechanism is fundamentally broken. Hooks are keyed by tmux pane ID (`%0`, `%1`, etc.) — ephemeral identifiers that reset when the tmux server restarts. After restart:
- Old hooks reference old pane IDs (e.g., `%0` → `claude --resume abc123`)
- New sessions get new pane IDs starting from `%0` again
- A new `%0` is a completely different pane in a potentially different session/project
- Hooks either fire in the wrong context (ID collision) or never fire (no matching ID)

The hook registration (`cmd/hooks.go:83-98`) uses `$TMUX_PANE` as the key, but pane IDs provide no durable link between the hook and the session or project it belongs to. Session names are also non-durable (`{project}-{nanoid}` format, `session/naming.go:41`), but the **session name is at least passed to `ExecuteHooks`** and could serve as a scoping mechanism.

**Why this happens:**
The dual-level tracking design (persistent `hooks.json` + volatile tmux server options) was built assuming pane IDs persist across restarts. The CLAUDE.md docs describe the intent: "volatile markers lost on reboot, so hooks re-fire after restart." But pane IDs are just as volatile as the markers — they're both assigned by tmux and lost on server restart.

### Contributing Factors

- `ListAllPanes()` (`tmux.go:237-238`) swallows errors and returns `([]string{}, nil)` — the caller can't distinguish "no panes" from "tmux not ready"
- `CleanStale` is a pure filter with no awareness of whether an empty live set is meaningful
- The `clean` command (`cmd/clean.go:77-80`) already has the correct guard (`if len(livePanes) == 0 { return nil }`) but this wasn't replicated in `ExecuteHooks`
- The cleanup in `ExecuteHooks` is "best-effort" (errors silently ignored) but the consequence of incorrect cleanup is data loss, not just a missed cleanup
- Hooks are keyed solely by pane ID with no additional context (session name, project path) to enable remapping after restart
- Session names are non-durable (`{project}-{nanoid}`), so even if hooks stored session names, they couldn't match by session name alone — but the project name prefix is stable

### Why It Wasn't Caught

- The test "no tmux server running skips cleanup gracefully" (`executor_test.go:537-568`) explicitly **validates the buggy behavior**: it passes an empty live-panes list to `CleanStale` and asserts it was called, effectively testing that hooks would be wiped
- The mock-based test doesn't capture the real-world consequence because `CleanStale` is mocked and doesn't actually remove data
- No integration test covers the server-restart scenario end-to-end
- The pane ID instability was never apparent because the feature was built and tested on a machine where the tmux server was always running
- The feature was first exercised against a real server restart on a freshly installed MacBook Pro (2026-04-01)

### Blast Radius

**Directly affected:**
- `internal/hooks/executor.go` — `ExecuteHooks` cleanup path (problem 1) and hook matching logic (problem 2)
- `internal/hooks/store.go` — hook data model keyed by pane ID (problem 2)
- `cmd/hooks.go` — hook registration uses `$TMUX_PANE` as sole key (problem 2)
- All resume hook functionality — hooks are either destroyed or orphaned after any server restart

**Potentially affected:**
- `cmd/clean.go` — already has the empty-pane guard, but `CleanStale` is called with the same pane-ID-based model
- `internal/hooks/executor_test.go` — test validates buggy behavior, needs correction

---

## Fix Direction

### Chosen Approach

*To be determined — requires discussion on problem 2 (pane ID instability).*

### Options Explored

*To be determined after discussion.*

### Discussion

*To be captured during discussion.*

### Testing Recommendations

*To be determined.*

### Risk Assessment

*To be determined.*

---

## Notes

- The synthesis agent independently confirmed the root cause (problem 1) with high confidence and no gaps
- `clean.go` guard was added during the `resume-sessions-after-reboot` feature work but wasn't applied to `ExecuteHooks` at the same time
- Problem 1 (empty-pane guard) is straightforward; problem 2 (pane ID durability) needs discussion about the right keying/matching strategy
