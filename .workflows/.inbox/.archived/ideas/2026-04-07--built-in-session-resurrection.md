# Built-in Session Resurrection

Replace tmux-resurrect and tmux-continuum with a native resurrection system inside Portal. Portal already owns the session lifecycle, so it can save and restore tmux state more reliably than external plugins that bolt on via shell scripts and race-prone timers.

tmux-resurrect and tmux-continuum have a well-documented race condition (multiple open issues, ~50% failure rate reported) where auto-restore on server start silently does nothing if resurrect hasn't finished initializing. There is no official fix. Beyond the race, continuum only saves on a 10-minute timer with no event-driven triggers, resurrect's save/restore is implemented as fragile shell scripts with `ps` parsing for process detection, and the plugin architecture means no direct access to tmux state.

The idea is a new `resurrect` or `snapshot` package that handles full tmux state capture and restoration — separate from the existing hooks system. Hooks handle resuming registered processes; resurrection handles restoring the full tmux environment: sessions, windows, panes, splits, layout, working directories, and running commands.

On the save side, capture full tmux state to JSON (consistent with Portal's existing stores) — session names, window indices, layout strings via `#{window_layout}`, pane working directories, running commands (short name via tmux format, full command line via `ps`), focus state, zoom state. Save triggers should be event-driven: on session create/destroy since Portal controls the lifecycle, plus a periodic fallback timer and an explicit `portal save` command.

On the restore side, during Portal's `PersistentPreRunE` bootstrap, read saved state and recreate the environment using tmux's `new-session`, `new-window`, `split-window`, `select-layout` (which handles all pane geometry automatically from the layout string), and focus/rename commands.

A key design choice inspired by Zellij: commands should start suspended by default rather than blindly re-running. Show the user what each pane would run and let them confirm. Plain shell panes just get a fresh shell in the correct working directory. A `--force` flag could override for automation. Zellij also detects editor invocations and handles them specially, which is worth considering.

Portal's advantages over the plugin approach: event-driven saves at the moments that matter, Go implementation with direct tmux API calls and proper error handling, no race conditions since restoration is part of bootstrap, Zellij-style suspended commands for safer restoration, integrated awareness of which sessions are Portal-managed, and no separate plugin installation or TPM dependency.

## Resurrection vs Resume Hooks

These are separate concerns that complement each other:

- **Resurrection** recreates the tmux structure — sessions, windows, panes, splits, working directories, running commands. This is the new feature.
- **Resume hooks** fire registered commands into panes — e.g. `claude --resume <uuid>`. This already exists.

Resurrection runs first, hooks run after. Resurrection creates the panes; hooks fill them with the right processes. With Portal controlling resurrection, it can guarantee that structural keys match for hooks to fire — no more depending on a third-party plugin to recreate things in the right shape.

For generic commands (not registered as hooks), Portal can use Zellij-style suspended restoration — show what would run, let the user press Enter to confirm, or auto-run with a `--force` flag.

## How Zellij Captures Running Commands

Zellij captures the **currently running foreground process**, not the original command that spawned the pane. It does this via `ps -ao ppid,args`, finding child processes of each pane's shell PID. So if you open a shell and type `claude`, Zellij captures `claude` — not `zsh`.

On resurrection, it recreates the pane with that captured command in suspended state. The user presses Enter to re-run it. This means Zellij can only replay `claude` (a fresh session), not `claude --resume <uuid>` — the resume UUID is created after the process starts and is not visible in the process tree.

This is exactly the gap Portal's resume hooks fill. Resurrection gets the tmux structure back; hooks provide the Claude-specific resume command with the correct conversation UUID. Zellij has no equivalent — it would just start a fresh Claude session.

Zellij also supports layout files that can specify commands with arguments upfront (e.g. `command "claude"` with `args "--resume"`), but this requires pre-configuring the layout rather than capturing live state.

## tmux APIs Available

tmux provides sufficient APIs for capturing and restoring all the state Portal needs.

### Capture

| Data | Command / Format |
|------|-----------------|
| Sessions | `tmux list-sessions -F '#{session_name}\t#{session_attached}\t#{session_path}'` |
| Windows | `tmux list-windows -a -F '#{session_name}\t#{window_index}\t#{window_name}\t#{window_layout}\t#{window_active}\t#{window_flags}\t#{window_zoomed_flag}'` |
| Panes | `tmux list-panes -a -F '#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_current_path}\t#{pane_current_command}\t#{pane_pid}\t#{pane_active}\t#{pane_title}'` |
| Full command | `ps -ao ppid,args` filtered by `#{pane_pid}` |
| Pane contents | `tmux capture-pane -epJ -S -<history_size> -t <pane_id>` |
| Client state | `tmux display-message -p -F '#{client_session}\t#{client_last_session}'` |

### Restore

| Action | Command |
|--------|---------|
| Create session | `tmux new-session -d -s <name> -c <dir>` |
| Create window | `tmux new-window -d -t <session>:<index> -c <dir>` |
| Split pane | `tmux split-window -t <session>:<window> -c <dir>` |
| Apply layout | `tmux select-layout -t <session>:<window> '<layout_string>'` |
| Set focus | `tmux select-pane -t <target>`, `tmux select-window -t <target>` |
| Rename window | `tmux rename-window -t <target> <name>` |
| Set zoom | `tmux resize-pane -t <target> -Z` |
| Send command | `tmux send-keys -t <target> '<command>' C-m` |
| Set pane title | `tmux select-pane -t <target> -T '<title>'` |

The `#{window_layout}` string is the key primitive for restoring splits. It's a recursive tree encoding the full pane geometry. Applying via `select-layout` automatically adjusts all pane sizes. The target window must have at least as many panes as the layout describes.

### Limitations

Things tmux cannot provide:
- Shell internal state (env vars, functions, aliases, unexported vars, history)
- Running process state (a mid-flight script cannot be suspended and resumed)
- Pipe/socket/network connections within panes
- Exact terminal state (cursor position within apps, scroll position)
- Full command line — `#{pane_current_command}` only gives the short name (e.g. `vim`), full args require `ps` parsing which is fragile for complex pipelines and backgrounded processes
- `#{pane_current_path}` relies on the shell/program sending OSC 7 escape sequences; may be stale if the program doesn't do this

## Open Questions

- Should Portal only resurrect Portal-managed sessions, or all tmux sessions?
- Should this replace resurrect/continuum entirely, or coexist?
- How to handle the default "0" session that tmux always creates on server start?
- Should Portal support grouped sessions?
- What to do about panes running commands that can't be meaningfully restarted (SSH connections, long-running builds)?
- Save format and location — `~/.config/portal/sessions.json` or `~/.local/share/portal/snapshots/`?
