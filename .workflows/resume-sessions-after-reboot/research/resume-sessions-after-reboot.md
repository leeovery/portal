# Research: Resume Sessions After Reboot

Generic restart command registry so Portal can resume processes (Claude Code, dev servers, etc.) in tmux sessions after a system reboot.

## Starting Point

What we know so far:
- After reboot, tmux-resurrect restores session layout, CWDs, and pane structure — but processes are dead
- The idea is a `register-restart` subcommand that stores opaque command strings keyed to tmux sessions
- On the Claude side, a `SessionStart` hook would call `portal register-restart "claude --resume <uuid>"` to keep the registration current
- After reboot, Portal detects sessions where the pane's active process is just the shell and a restart command is registered — TUI shows these as "resumable"
- Selecting a resumable session sends the registered command via `tmux send-keys`
- tmux-resurrect's `@resurrect-processes` re-runs original launch commands, which doesn't work for Claude (resume UUID differs from launch command)

Open questions:
- Per-session vs per-pane scoping (multi-pane sessions may need different restart commands)
- Whether to confirm before auto-sending commands
- How to handle JSON parsing fragility for extracting session_id from hook stdin
- Whether users should be advised not to add Claude to `@resurrect-processes` to avoid conflicts

---
