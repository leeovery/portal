# Research: Built-in Session Resurrection

Replace tmux-resurrect and tmux-continuum with a native session resurrection system inside Portal. Portal already owns the session lifecycle, so it can save and restore tmux state more reliably than external plugins.

## Starting Point

What we know so far:
- tmux-resurrect/continuum have a well-documented race condition (~50% failure rate) where auto-restore silently fails if resurrect hasn't finished initializing
- Continuum only saves on a 10-minute timer with no event-driven triggers; resurrect uses fragile shell scripts with `ps` parsing
- Portal controls the session lifecycle already — event-driven saves at create/destroy are possible
- Zellij's approach: capture foreground process via `ps -ao ppid,args`, restore in suspended state (user confirms before re-running)
- Resurrection and resume hooks are complementary: resurrection recreates tmux structure (sessions, windows, panes, splits, dirs); hooks fill panes with the right processes (e.g., `claude --resume <uuid>`)
- tmux APIs are sufficient for full state capture (`list-sessions`, `list-windows`, `list-panes`, `#{window_layout}`) and restore (`new-session`, `split-window`, `select-layout`)
- Key limitations: can't capture shell internal state, running process state, pipe/socket connections, or exact terminal state; `#{pane_current_command}` only gives short name (full args need `ps` parsing)

Open questions from initial ideation:
- Portal-managed sessions only, or all tmux sessions?
- Replace resurrect/continuum entirely, or coexist?
- How to handle the default "0" session tmux creates on server start?
- Support for grouped sessions?
- Handling of non-restartable commands (SSH, long-running builds)?
- Save format and location — `~/.config/portal/sessions.json` or `~/.local/share/portal/snapshots/`?

---
