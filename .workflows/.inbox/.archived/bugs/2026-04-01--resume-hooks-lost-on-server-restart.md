# Resume hooks lost on tmux server restart

On-resume hooks registered in `hooks.json` are silently removed when the tmux server is killed and restarted, preventing them from ever firing. The hooks simply vanish from disk before they get a chance to execute.

The scenario plays out when two or more Claude Code sessions are running in tmux panes, each with an on-resume hook registered against its pane ID (e.g. `%0`, `%1`). If the tmux server is killed — whether deliberately via `tmux kill-server` or through a crash/reboot — and Portal is reopened, the project picker appears with "No sessions available" and the hooks never fire. Inspecting `hooks.json` afterward reveals the entries have been removed.

The root of the problem is in `ExecuteHooks()` in `internal/hooks/executor.go`, which calls `store.CleanStale()` unconditionally at the start of execution. `CleanStale()` in `internal/hooks/store.go` queries all live panes in the current tmux server via `tmux list-panes -a`, compares them against entries in `hooks.json`, and removes any entries whose pane IDs don't exist in the live server. After a server restart, the new tmux server has no knowledge of the old pane IDs — they're ephemeral identifiers assigned sequentially and reset when the server restarts. So `CleanStale` sees the old `%0` and `%1` as stale and deletes them before they have any chance to be used.

A pane ID like `%0` might survive by coincidence if the new server happens to assign the same ID to its first pane, but this is accidental — it's not the original pane and the hook fires in the wrong context or not at all. The fundamental issue is that hooks are keyed by tmux pane ID, which provides no durable mapping between the hook and the session or project it belongs to.

The impact is that any workflow relying on resume hooks to restore Claude Code sessions after a server restart is broken. The hooks are the mechanism for session continuity, and they're being destroyed at exactly the moment they're needed most.
