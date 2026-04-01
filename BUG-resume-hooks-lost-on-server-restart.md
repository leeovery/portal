# Bug: Resume hooks lost on tmux server restart

## Summary

On-resume hooks registered in `hooks.json` are silently removed when the tmux server is killed and restarted, preventing them from ever firing.

## Context

Two Claude Code sessions were running in tmux panes `%0` and `%1`, each with an on-resume hook registered:

```json
{
  "%0": {
    "on-resume": "claude --resume ed161e91-3fb2-4eec-adb6-96931b1a73a8"
  },
  "%1": {
    "on-resume": "claude --resume 537ebbc0-75c2-4cd0-9a36-e28f3e7e97fe"
  }
}
```

## Steps to reproduce

1. Register on-resume hooks for two panes (e.g. `%0` and `%1`)
2. Verify hooks.json contains both entries
3. Run `tmux kill-server`
4. Open Portal (which starts a fresh tmux server)
5. Portal shows the projects page with "No sessions available"
6. Check hooks.json — entries have been removed

## What happened

After killing the tmux server and reopening Portal:

- Portal started a new tmux server and displayed the project picker
- It showed "No sessions available" under Recent Projects
- The hooks did not fire
- Inspecting `hooks.json` afterward revealed the `%1` entry had been removed entirely. The `%0` entry survived only because the new tmux server coincidentally assigned `%0` to its first pane — not because it was the original pane

## Diagnosis

The issue traces to `ExecuteHooks()` in `internal/hooks/executor.go` (around line 64-109). At the start of execution, it calls `store.CleanStale()` which:

1. Queries all live panes in the **current** tmux server via `tmux list-panes -a`
2. Compares them against entries in `hooks.json`
3. Removes any entries whose pane IDs don't exist in the live server

After a `tmux kill-server`, the new tmux server has no knowledge of the old pane IDs. The old `%0` and `%1` are gone. When `CleanStale` runs, it sees these pane IDs don't exist (or only `%0` exists by coincidence as the first pane of the new server) and removes the entries from disk before they have any chance to be used.

The hooks are keyed by tmux pane ID (e.g. `%0`, `%1`), which is ephemeral — assigned sequentially by the tmux server and reset when the server restarts. There is no durable mapping between the hook and the session or project it belongs to.

## Affected files

- `internal/hooks/executor.go` — `ExecuteHooks()` calls `CleanStale()` unconditionally before processing hooks
- `internal/hooks/store.go` — `CleanStale()` removes entries for pane IDs not found in the live tmux server
