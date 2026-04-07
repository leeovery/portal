# Built-in Session Resurrection

Replace tmux-resurrect and tmux-continuum with a native resurrection system inside Portal. Portal already owns the session lifecycle, so it can save and restore tmux state more reliably than external plugins that bolt on via shell scripts and race-prone timers.

tmux-resurrect and tmux-continuum have a well-documented race condition (multiple open issues, ~50% failure rate reported) where auto-restore on server start silently does nothing if resurrect hasn't finished initializing. There is no official fix. Beyond the race, continuum only saves on a 10-minute timer with no event-driven triggers, resurrect's save/restore is implemented as fragile shell scripts with `ps` parsing for process detection, and the plugin architecture means no direct access to tmux state.

The idea is a new `resurrect` or `snapshot` package that handles full tmux state capture and restoration — separate from the existing hooks system. Hooks handle resuming registered processes; resurrection handles restoring the full tmux environment: sessions, windows, panes, splits, layout, working directories, and running commands.

On the save side, capture full tmux state to JSON (consistent with Portal's existing stores) — session names, window indices, layout strings via `#{window_layout}`, pane working directories, running commands (short name via tmux format, full command line via `ps`), focus state, zoom state. Save triggers should be event-driven: on session create/destroy since Portal controls the lifecycle, plus a periodic fallback timer and an explicit `portal save` command.

On the restore side, during Portal's `PersistentPreRunE` bootstrap, read saved state and recreate the environment using tmux's `new-session`, `new-window`, `split-window`, `select-layout` (which handles all pane geometry automatically from the layout string), and focus/rename commands.

A key design choice inspired by Zellij: commands should start suspended by default rather than blindly re-running. Show the user what each pane would run and let them confirm. Plain shell panes just get a fresh shell in the correct working directory. A `--force` flag could override for automation. Zellij also detects editor invocations and handles them specially, which is worth considering.

Portal's advantages over the plugin approach: event-driven saves at the moments that matter, Go implementation with direct tmux API calls and proper error handling, no race conditions since restoration is part of bootstrap, Zellij-style suspended commands for safer restoration, integrated awareness of which sessions are Portal-managed, and no separate plugin installation or TPM dependency.

Open questions include whether to resurrect only Portal-managed sessions or all tmux sessions, how to handle the default "0" session tmux creates on server start, and what to do about panes running commands that can't be meaningfully restarted (SSH connections, long-running builds). Full design notes and tmux API research in `IDEA-session-resurrection.md`.
