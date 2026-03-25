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
- Whether to confirm before auto-sending commands
- Whether users should be advised not to add Claude to `@resurrect-processes` to avoid conflicts

---

## Resolved: Scoping and Feasibility

**Pane-level scoping confirmed.** Registry keys by tmux pane ID, not session. A tmux session with 4 panes running 4 Claude instances needs 4 independent restart commands.

**No detection needed.** Portal doesn't need to detect whether a pane's process is dead. If a restart command is registered for a pane, execute it. If not, normal behavior. Simpler.

**Claude Code hook payload confirmed viable.** The `SessionStart` hook receives JSON on stdin including `session_id` — the exact UUID needed for `claude --resume <uuid>`. The `source` field distinguishes startup/resume/clear/compact. Hook fires on every session start and resume, keeping the registration current.

**Pane ID is self-discoverable.** `$TMUX_PANE` environment variable is set automatically by tmux in every process running inside a pane. Portal reads it internally via `os.Getenv("TMUX_PANE")` — the hook script never needs to know about pane IDs. This keeps the hook purely tool-specific:

```bash
SESSION_ID=$(cat - | jq -r '.session_id')
portal register-restart "claude --resume $SESSION_ID"
```

Portal handles the pane association transparently.
