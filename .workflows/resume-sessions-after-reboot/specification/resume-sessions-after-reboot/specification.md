# Specification: Resume Sessions After Reboot

## Specification

### Overview

Portal manages tmux sessions. After a system reboot, tmux-resurrect restores session layout, CWDs, and pane structure — but processes are dead. Portal provides a generic hook system that can resume processes (Claude Code, dev servers, etc.) in their original panes.

The core problem: tmux-resurrect's `@resurrect-processes` re-runs original launch commands, which doesn't work for tools where the resume command differs from the launch command (e.g., `claude --resume <uuid>` vs `claude`).

**Architecture:**

- **Persistent registry** (file on disk): maps pane IDs to restart commands — survives reboot
- **Volatile markers** (tmux server options): indicate a process was registered on this server lifetime — die with the server
- **Execution trigger**: Portal's connection flow (`portal open`) — lazy, not eager
- **Execution condition**: persistent entry exists AND volatile marker absent (server restarted since registration)

External tools register hooks via `xctl hooks set` (e.g., Claude Code's `SessionStart` hook calls `xctl hooks set --on-resume "claude --resume $SESSION_ID"`). Portal handles pane mapping internally using `$TMUX_PANE`.

### Registry Model & Storage

**Scoping model:** Per-pane, flat registry. A simple `pane_id → restart_command` map. No session or window hierarchy stored — when Portal needs to act on a session's processes, it queries tmux for that session's panes and cross-references the registry.

Pane IDs (`%0`, `%1`, etc.) are globally unique across the entire tmux server, assigned sequentially, and persist across tmux-resurrect.

**Storage format:** JSON at `~/.config/portal/hooks.json`.

```json
{
  "%3": { "on-resume": "claude --resume abc123" },
  "%7": { "on-resume": "claude --resume def456" }
}
```

- Follows existing convention: structured data → JSON, simple key-value → flat file
- Hooks are structured (pane × event type) and will grow to support multiple events per pane
- Reuses the atomic write pattern from `project/store.go`

---

## Working Notes

[Optional - capture in-progress discussion if needed]
