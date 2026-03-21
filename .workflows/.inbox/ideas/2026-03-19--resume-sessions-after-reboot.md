# Resume Sessions After Reboot

After a system reboot, tmux-resurrect restores session layout, CWDs, and pane structure — but the processes that were running inside them are dead. You open Portal, select a restored session, and find a bare shell prompt where Claude Code (or any long-running process) used to be. With 5-10 Claude sessions running across separate tmux windows, a reboot means manually resuming each one.

The idea is a generic restart command registry. Portal exposes a `register-restart` subcommand that stores opaque command strings keyed to tmux sessions. Any tool — Claude Code, a dev server, a database REPL — can register its own restart command via this facility. Portal has zero knowledge of what the commands do.

On the Claude side, a single user-level `SessionStart` hook (configured once in `~/.claude/settings.json`) would fire on every startup/resume, extract the session UUID from stdin JSON, and call `portal register-restart "claude --resume <uuid>"`. The hook always reflects the current conversation — if the user switches conversations, the registration updates automatically.

After reboot, Portal detects sessions where the pane's active process is just the shell (original process died) and a restart command is registered. The TUI shows these as "resumable." Selecting one sends the registered command to the pane via `tmux send-keys`.

Zellij handled this natively with built-in session resurrection that re-executed saved commands on reattach. tmux-resurrect has `@resurrect-processes` but it re-runs the original launch command, which isn't reliable for Claude (the resume UUID isn't the launch command). The registry approach decouples the restart command from the original launch command entirely.

Open questions include per-session vs per-pane scoping (a session with multiple panes could need different restart commands), whether there should be a confirmation step before auto-sending commands, how to handle the inline grep/sed fragility for extracting session_id from JSON, and whether users should be advised not to add Claude to `@resurrect-processes` to avoid conflicts.
