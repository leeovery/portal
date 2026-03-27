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

---

## Working Notes

[Optional - capture in-progress discussion if needed]
