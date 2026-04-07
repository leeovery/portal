# Idea: Built-in Session Resurrection

Replace tmux-resurrect and tmux-continuum with a native resurrection system inside Portal. Portal already controls the session lifecycle, so it can save and restore state more reliably than external plugins that bolt on via shell scripts and race-prone timers.

## Motivation

tmux-resurrect and tmux-continuum have a well-documented race condition ([#52](https://github.com/tmux-plugins/tmux-continuum/issues/52), [#90](https://github.com/tmux-plugins/tmux-continuum/issues/90), [#57](https://github.com/tmux-plugins/tmux-continuum/issues/57)). When continuum triggers auto-restore on server start, it backgrounds a script that sleeps 1 second then reads a tmux option set by resurrect. If resurrect hasn't finished initializing, the option is empty and restore silently does nothing. Issue #90 reports ~50% failure rate. There is no official fix.

Beyond the race condition:
- Continuum only saves on a 10-minute timer — no save on session create/destroy
- Resurrect's save/restore is implemented as shell scripts that run via `tmux run-shell`, with fragile process detection via `ps` parsing
- The plugin architecture means no direct access to tmux state — everything goes through string formatting and shell commands
- No ability to present commands for user confirmation before re-running them

## Loose Plan

Build a new `resurrect` (or `snapshot`) package in Portal that handles full tmux state capture and restoration. This is an entirely separate feature from the existing hooks system. Hooks handle resuming registered processes; resurrection handles restoring the full tmux environment (sessions, windows, panes, splits, layout, working directories, running commands).

### Save

Capture full tmux state to a JSON file (consistent with Portal's existing stores). State to capture per session:

**Session level:**
- Session name
- Active/attached state

**Window level (tabs):**
- Window index and name
- Layout string (`#{window_layout}` — recursive tree encoding split geometry)
- Active window flag
- Zoom state
- Automatic-rename setting

**Pane level:**
- Pane index
- Working directory (`#{pane_current_path}`)
- Running command — short name via `#{pane_current_command}`, full command line via `ps -ao ppid,args` keyed by pane PID (`#{pane_pid}`)
- Active pane flag
- Pane title

**Optional (future):**
- Pane viewport contents via `tmux capture-pane -epJ`
- Scrollback history

**When to save:**
- On session create and destroy (event-driven, since Portal controls the lifecycle)
- Periodic fallback timer (configurable interval, similar to Zellij's default 60s)
- On explicit user command (`portal save` or similar)

**Where to save:**
- `~/.config/portal/sessions.json` or `~/.local/share/portal/snapshots/` — TBD

### Restore

On server start (detected in Portal's `PersistentPreRunE` bootstrap), read the saved state and recreate the tmux environment:

1. For each saved session: `tmux new-session -d -s <name> -c <dir>`
2. For each window beyond the first: `tmux new-window -d -t <session>:<index> -c <dir>`
3. For each pane beyond the first: `tmux split-window -t <session>:<window> -c <dir>`
4. Apply layout strings: `tmux select-layout -t <session>:<window> <layout_string>` — tmux handles all the geometry automatically
5. Restore focus: `tmux select-pane`, `tmux select-window`, zoom state
6. Rename windows: `tmux rename-window`
7. Re-run commands via `tmux send-keys` (see below)

### Command Restoration

Take inspiration from Zellij's approach: **commands start suspended by default**. Rather than blindly re-running commands (which could have side effects, hit expired credentials, or run destructive operations), show the user what each pane would run and let them confirm. A `--force` flag could override this for automation.

Zellij also detects editor invocations (vim, nvim, emacs, etc.) and handles them specially. Worth considering.

Plain shell panes (just running zsh/bash with no arguments) should just start a fresh shell in the correct working directory — no need to "restore" anything.

### Scope Decisions (TBD)

- Should Portal only resurrect Portal-managed sessions, or all tmux sessions?
- Should this replace resurrect/continuum entirely, or coexist?
- How to handle the default "0" session that tmux always creates on server start?
- Should Portal support grouped sessions?
- What to do about panes running commands that can't be meaningfully restarted (long-running builds, SSH connections, etc.)?

## Research: How Zellij Does It

Zellij's session persistence is a first-class feature built into the server. Key design decisions:

- **Layout-as-source-of-truth**: Sessions are serialized into the same KDL layout format used for initial session creation. Resurrection is literally "start a new session with this layout file."
- **Timer-based saves**: Background task serializes every 60 seconds (configurable). Metadata written every ~1 second.
- **Commands start suspended**: Panes show the command that would run but wait for user confirmation (`start_suspended true`). `--force-run-commands` overrides.
- **Editor detection**: Recognizes vim/nvim/emacs/nano/kak/helix with file arguments and upgrades them to native `edit` pane types for smarter restoration.
- **Global CWD**: Computes longest common path across all panes, stores individual CWDs as relative paths for portability.
- **Dirty-checking**: Skips serialization when layout hasn't changed from default.
- **Optional pane content capture**: Terminal scrollback saved as separate files, off by default.
- **Clean exit handling**: Session metadata deleted on clean exit; layout file intentionally left behind for resurrection.

State captured by Zellij per pane: geometry (position, size), run configuration (command + args, or editor file + line, or plugin), working directory, title, focus state, border settings, colors, split direction, optional viewport contents.

Explicitly not captured: shell history, environment variables, scrollback beyond configured limit, undo/redo state, running process PIDs.

Two files written per session:
- `session-metadata.kdl` — live session info, deleted on clean exit
- `session-layout.kdl` — the actual layout used for resurrection, persists after exit

## Research: tmux APIs Available

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

The `#{window_layout}` string is the key primitive for restoring splits. It's a recursive tree structure encoding the full pane geometry: `<checksum>,<width>x<height>,<x>,<y>[,<pane_id>|{<h-children>}|[<v-children>]]`. Applying it via `select-layout` automatically adjusts all pane sizes to match. The target window must have at least as many panes as the layout describes.

### Limitations

Things tmux cannot provide:
- Shell internal state (env vars, functions, aliases, unexported vars, history)
- Running process state (a mid-flight script cannot be suspended and resumed)
- Pipe/socket/network connections within panes
- Exact terminal state (cursor position within apps, scroll position)
- Full command line — `#{pane_current_command}` only gives the short name (e.g. `vim`), full args require `ps` parsing which is fragile for complex pipelines and backgrounded processes
- `#{pane_current_path}` relies on the shell/program sending OSC 7 escape sequences; may be stale if the program doesn't do this
- Per-window/session options (except those explicitly queried)
- Nested tmux sessions

## Portal's Advantages Over Resurrect

- **Event-driven saves** at the moments that matter, not a dumb timer
- **Go implementation** — direct tmux API calls, proper error handling, no shell script fragility
- **No race conditions** — restoration is part of Portal's bootstrap, not a backgrounded script hoping the timing works out
- **Zellij-style suspended commands** — safer restoration with user confirmation
- **Integrated with Portal's session model** — awareness of which sessions are Portal-managed
- **Single tool** — no separate plugin installation, no TPM dependency for this functionality
