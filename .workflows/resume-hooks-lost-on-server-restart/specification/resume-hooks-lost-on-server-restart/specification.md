# Specification: Resume Hooks Lost on Server Restart

## Specification

### Problem Statement

Resume hooks registered in `hooks.json` do not survive tmux server restarts. Two distinct problems cause this:

**Problem 1 — Hook deletion on restart:** `ExecuteHooks()` calls `store.CleanStale()` with the result of `ListAllPanes()`. After a server restart, `ListAllPanes()` returns an empty slice (it swallows errors), so `CleanStale()` treats every hook entry as stale and deletes it. The `clean` command already guards against this (`cmd/clean.go:77-80`), but the guard was never replicated in `ExecuteHooks`.

**Problem 2 — Pane ID instability:** Hooks are keyed by tmux pane ID (`%0`, `%1`), which are ephemeral identifiers that reset on server restart. Even if hooks survive deletion (problem 1 fixed), they reference stale pane IDs that either collide with unrelated new panes or match nothing. The original design incorrectly assumed pane IDs persist across tmux-resurrect — they do not.

### Solution Overview

Two-part fix:

1. **Empty-pane guard:** Add `len(livePanes) > 0` check in `ExecuteHooks` before calling `CleanStale`, matching the existing guard in `clean.go:77-80`. Prevents hook data loss on server restart.

2. **Structural keys replacing pane IDs:** Change the hook storage model from pane-ID-based keys to structural keys using tmux's positional addressing: `session_name:window_index.pane_index`. This format survives tmux-resurrect (which uses the same addressing scheme internally for `send-keys` targeting during restore). Pane IDs do not survive.

### Storage Model

**Current model:** `hooks.json` maps `pane_id → map[event]command` (e.g., `{"%0": {"on-resume": "claude --resume abc"}}`).

**New model:** `hooks.json` maps `structural_key → map[event]command`, where `structural_key` is `session_name:window_index.pane_index` (e.g., `{"my-project-abc123:0.0": {"on-resume": "claude --resume abc"}}`).

**Structural key format:** `session_name:window_index.pane_index`
- `session_name` — the tmux session name (e.g., `my-project-abc123`)
- `window_index` — zero-based window index within the session
- `pane_index` — zero-based pane index within the window
- Separator: colon between session name and window, dot between window and pane

This is the same addressing scheme tmux-resurrect uses for targeting panes during restore.

### Component Changes

**Hook registration (`cmd/hooks.go`):** Instead of using `$TMUX_PANE` as the key, query tmux for the current pane's session name, window index, and pane index. Build the structural key `session_name:window_index.pane_index` and use it as the hook storage key.

**Hook execution (`internal/hooks/executor.go`):**
- Add empty-pane guard: skip `CleanStale` when `len(livePanes) == 0`.
- Match hooks by structural key instead of pane ID. When `ExecuteHooks(sessionName)` runs, query the session's panes with their window/pane indices and look up hooks by `sessionName:windowIndex.paneIndex`.

**Hook storage (`internal/hooks/store.go`):** Update the data model — the map key changes from pane ID to structural key. `CleanStale` cross-references structural keys against live tmux structure instead of pane IDs.

**Volatile markers:** Change marker naming from `@portal-active-%paneID` to a structural-key-based format (e.g., `@portal-active-session:window.pane`).

**Clean command (`cmd/clean.go`):** Update to use structural key model for cleanup. The existing empty-pane guard remains.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
