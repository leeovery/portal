<div align="center">

# Portal

**Interactive session picker for tmux**

A CLI that gives you fast, fuzzy session management from bare shell,
<br>with project memory, path aliases, and a keyboard-driven TUI.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](https://go.dev)

[Install](#install) · [Quick Start](#quick-start) · [Commands](#commands) · [Shell Integration](#shell-integration) · [Configuration](#configuration)

</div>

---

Portal is a CLI that runs at bare shell (before entering tmux) and provides an interactive TUI for picking, creating, and managing tmux sessions. It remembers your projects, resolves paths via aliases and zoxide, auto-detects git roots for new sessions, and automatically starts the tmux server and restores your saved sessions after a reboot.

After [shell integration](#shell-integration), you interact with Portal through two functions: **`x`** (session picker / opener) and **`xctl`** (subcommands like list, kill, alias). The function names are customizable — see `--cmd` below.

## Features

- **Interactive session picker** — fuzzy-filterable TUI for switching, creating, renaming, or killing tmux sessions from a bare shell.
- **Scrollback preview** — press `Space` on any session in the picker for a read-only Quick Look of its terminal scrollback. Cycle windows and panes without attaching. See [Scrollback Preview](#scrollback-preview).
- **Path aliases + zoxide** — open sessions by short name (`x work`) or recent directory; falls back to a TUI filter when nothing matches.
- **Project memory + git roots** — new sessions auto-resolve to the repository root and are remembered for next time.
- **Reboot-safe sessions** — automatic tmux server bootstrap plus full restoration of session structure, layout, working dirs, and ANSI scrollback after a reboot. Replaces tmux-resurrect/tmux-continuum.
- **Per-pane resume hooks** — register commands (`npm start`, `claude --resume`, etc.) that fire only when a pane is re-created from saved state.

## Install

### Requirements

- **tmux ≥ 3.0** (released Feb 2020) — Portal uses array-indexed global hooks
  (`set-hook -ga`) which require 3.0+. Earlier versions are not supported;
  Portal exits with `Portal requires tmux ≥ 3.0 (found <version>). Please upgrade.`
- **Go** (build from source), **macOS or Linux**.

**macOS**

```bash
brew install leeovery/tools/portal
```

**Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/leeovery/portal/main/scripts/install.sh | bash
```

**Go**

```bash
go install github.com/leeovery/portal@latest
```

## Quick Start

```bash
# Add shell integration (creates x() and xctl() functions)
echo 'eval "$(portal init zsh)"' >> ~/.zshrc

# Launch the interactive picker
x

# Open a session at a path
x ~/Code/myproject

# Open with a command
x ~/Code/myproject -e "make dev"

# Set up an alias
xctl alias set work ~/Code/work-project
x work

# List running sessions
xctl list

# Kill a session
xctl kill myproject
```

## Shell Integration

Portal generates shell functions via `portal init`. Add to your shell profile:

```bash
# zsh
eval "$(portal init zsh)"

# bash
eval "$(portal init bash)"

# fish
portal init fish | source
```

This creates two functions:

- **`x()`** — launches Portal (interactive picker or path-based session creation)
- **`xctl()`** — direct access to Portal subcommands (`list`, `kill`, `alias`, etc.)

Customize the function name with `--cmd`:

```bash
eval "$(portal init zsh --cmd p)"   # creates p() and pctl()
```

## Commands

> Examples below use the default `x` / `xctl` function names. If you used `--cmd p`, substitute `p` and `pctl`. You can also call the `portal` binary directly.

### `x` (open)

Interactive session picker or path-based session creation. `x` maps to `portal open`.

```bash
x                                    # interactive TUI
x ~/Code/myproject                   # open session at path
x myalias                            # resolve alias → path → session
x ~/Code/app -e "make dev"           # run command in new session
x ~/Code/app -- npm start            # alternative command syntax
```

| Flag | Description |
|---|---|
| `-e, --exec` | Command to execute in the new session |

Path resolution order: aliases → zoxide → TUI with filter.

New sessions auto-resolve to the git repository root when applicable.

### `xctl attach`

Attach to an existing tmux session by name.

```bash
xctl attach myproject
```

### `xctl list`

List running tmux sessions.

```bash
xctl list                            # auto-detect format
xctl list --long                     # full details
xctl list --short                    # names only
```

| Flag | Description |
|---|---|
| `--long` | Full session details (name, status, window count) |
| `--short` | Session names only, one per line |

### `xctl kill`

Kill a tmux session by name.

```bash
xctl kill myproject
```

### `xctl alias`

Manage path aliases for quick session access.

```bash
xctl alias set work ~/Code/work      # create alias
xctl alias rm work                   # remove alias
xctl alias list                      # list all aliases
```

### `xctl hooks`

Register per-pane commands that re-execute automatically when a session is attached after a reboot. `hooks set` must be run from inside a tmux pane; `hooks rm` defaults to the current pane but accepts `--pane-key` to remove a hook for any pane (including ones that no longer exist).

```bash
xctl hooks set --on-resume "npm start"            # register a resume hook
xctl hooks rm --on-resume                         # remove the current pane's hook
xctl hooks rm --on-resume --pane-key 'sess:0.1'   # remove a specific entry (works outside tmux)
xctl hooks list                                   # list all hooks
```

**When hooks fire:** Portal fires resume hooks ONLY when a pane is freshly recreated
from saved state on reboot recovery — i.e., the tmux server has just been started
fresh and Portal has restored sessions. Hooks do NOT fire on every detach / reattach
within a single server lifetime. If a pane still exists, its hook process either
already ran or was explicitly killed; firing again would double-launch long-running
processes like `claude --resume`. This is deliberate.

### `xctl clean`

Remove stale projects whose directories no longer exist on disk, and prune hooks for panes that no longer exist.

```bash
xctl clean              # prune stale projects + hooks (rotated logs preserved)
xctl clean --logs       # also delete rotated portal.log.<date> files, keeping today's
```

`xctl clean` preserves rotated logs by default; pass `--logs` to additionally sweep every rotated `portal.log.<date>` file (see [Logging](#logging)).

### `xctl state`

Inspect or tear down Portal's saved-session state used for reboot restoration.

```bash
xctl state status                    # daemon + state health
xctl state cleanup                   # remove hooks + stop daemon
xctl state cleanup --purge           # also wipe ~/.config/portal/state/
```

- `xctl state status` — print daemon status, last save time, captured counts,
  state size, and recent warnings. Exits non-zero when the daemon is down, last
  save is stale, or warnings are present in the last hour.
- `xctl state cleanup [--purge]` — kills the daemon and removes Portal's tmux
  hook entries. With `--purge`, also removes `~/.config/portal/state/`.

### `xctl version`

Print the Portal version.

```bash
xctl version
```

### `portal init`

Output shell integration script for eval. See [Shell Integration](#shell-integration). This is the one command you call via the `portal` binary directly.

```bash
portal init zsh
portal init bash --cmd p
```

## TUI Keybindings

| Key | Action |
|---|---|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `Enter` | Select session / confirm |
| `Space` | Preview scrollback of highlighted session (sessions list only) |
| `/` | Filter mode (fuzzy search) |
| `R` | Rename session |
| `K` | Kill session |
| `q`/`Esc` | Quit |

The TUI has four views: session list, project picker, file browser, and scrollback preview.

### Scrollback Preview

`Space` on the highlighted session opens a Quick Look-style preview of that
session's saved scrollback so you can disambiguate similarly-named sessions
without paying the attach/detach cost. The preview is read-only — opening and
dismissing it leaves the session byte-identical (no hydration, no resume-hook
firing, no tmux state mutation).

| Key | Action |
|---|---|
| `]` | Next window (wraps) |
| `[` | Previous window (wraps) |
| `Tab` | Next pane within the current window (wraps) |
| `↑`/`↓`/`j`/`k`/`PgUp`/`PgDn`/`Home`/`End` | Scroll within the loaded buffer |
| `Esc` | Return to the sessions list |

Each pane shows the last ~1000 lines of saved scrollback, anchored at the
tail. Chrome shows `Window M of N · Pane X of Y · <window-name>` plus key
hints. A pane that has no saved content yet (brand-new session, daemon
hasn't ticked) renders `(no saved content)`.

## Automatic Server Bootstrap & Restoration

Portal automatically starts the tmux server if absent AND restores saved sessions
in the same bootstrap step. After a reboot, your sessions return with structure,
layout, zoom, working directories, and scrollback (including ANSI colour). On any
tmux-needing command, Portal checks the server, starts it if missing, and re-creates
saved sessions that aren't already live. Scrollback injects lazily when you attach.
Resume hooks fire on freshly-recreated panes. The TUI shows a "Restoring sessions…"
loading screen for at most ~1.2s; the CLI is silent.

Replaces tmux-continuum/tmux-resurrect for session persistence — uninstall those
plugins if you have them (or set `@continuum-restore off` for tmux-resurrect/continuum)
to avoid duplicate restoration.

Pair this with [resume hooks](#xctl-hooks) to automatically re-run pane commands
(dev servers, editors, etc.) after a reboot.

## Configuration

Portal resolves its config directory using XDG: `$XDG_CONFIG_HOME/portal/` if set, otherwise `~/.config/portal/`. Each file also has a per-file env var override that takes full precedence.

| File | Purpose | Env override |
|---|---|---|
| `aliases` | Path aliases (key=value, one per line) | `PORTAL_ALIASES_FILE` |
| `projects.json` | Remembered project directories | `PORTAL_PROJECTS_FILE` |
| `hooks.json` | Per-pane resume hooks (pane → event → command) | `PORTAL_HOOKS_FILE` |
| `state/` | Saved session structure + scrollback for automatic restoration on reboot. Contains: `sessions.json` (structure index), `scrollback/*.bin` (per-pane content), `daemon.pid` + `daemon.version` (liveness markers), `portal.log` (structured, rotating diagnostics — see [Logging](#logging)). See [Privacy Considerations](#privacy-considerations). | `PORTAL_STATE_DIR` |

Projects are auto-populated when you create new sessions and cleaned with `xctl clean`.

## Logging

Portal writes a structured diagnostic log to `state/portal.log` (under `PORTAL_STATE_DIR`). It is human-readable text with a `subsystem:` prefix on every line, so `grep "daemon:" portal.log` (or `restore:`, `saver:`, `hydrate:`, …) reconstructs what any subsystem did. `portal.log` is a symlink to a calendar-daily file (`portal.log.<date>`), so `tail -f portal.log` always follows today's log.

- **Rotation** — a new file each local day; older files are kept read-only. A size-cap safety valve rolls over to `portal.log.<date>.N` if a single day ever grows huge.
- **Retention** — rotated files older than 30 days are deleted automatically (one breadcrumb logged per deletion). `xctl clean --logs` sweeps them on demand.
- **Level** — defaults to `info` (a few lines per meaningful event). Set `PORTAL_LOG_LEVEL=debug` to capture full reconstruction detail when investigating an issue.

| Env var | Purpose | Default |
|---|---|---|
| `PORTAL_LOG_LEVEL` | Verbosity: `debug` / `info` / `warn` / `error` | `info` |
| `PORTAL_LOG_ROTATE_SIZE` | Per-day size cap before overflow (`K`/`M`/`G` suffix, e.g. `500M`, `1G`) | `500M` |
| `PORTAL_LOG_RETENTION_DAYS` | Days of rotated logs to keep | `30` |

## Privacy Considerations

Portal persists pane scrollback to `~/.config/portal/state/` (override via
`PORTAL_STATE_DIR`) so it can rehydrate sessions after a reboot. Files are written
mode `0600`, directories `0700`.

- Same local-filesystem trust model as your shell history — anything visible in
  your terminal can end up in the saved state.
- **No encryption at rest.** If a pane displays secrets (tokens, credentials,
  diffs of sensitive files), they will be captured.
- **`portal.log` records config changes verbatim.** It does not contain pane
  scrollback, but config-mutation breadcrumbs and exec handoffs are logged as-is
  — a `xctl hooks set --on-resume "<cmd>"` command string, alias values, and
  project paths appear in the log. Redact manually if you share it in a bug report.
- **Mitigations:** for sensitive panes, run `tmux set-option -w history-limit 0`
  to prevent scrollback from accumulating, or `tmux clear-history` on demand
  (run before the next save, which lands at most ~30s later).
- v1 has no per-session opt-out; tmux-native workarounds above are the
  supported path.

## Uninstall

Two paths depending on whether you want to keep your saved state:

- **Just remove the binary** — `brew uninstall portal` or `rm $(which portal)`.
  The defensive `command -v portal` guard in the registered tmux hooks
  short-circuits when the binary is gone, so tmux keeps running normally. Your
  saved state is preserved; reinstalling Portal picks up where it left off.
- **Explicit teardown** — run `portal state cleanup` (kills the daemon and
  removes Portal's tmux hook entries), or `portal state cleanup --purge` to
  also wipe saved state under `~/.config/portal/state/`. Then uninstall the
  binary. Use `--purge` for a completely clean slate. Non-state config
  (`hooks.json`, `projects.json`, `aliases`) is preserved either way — remove
  manually if desired.

## License

MIT
