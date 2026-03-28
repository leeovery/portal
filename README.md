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

Portal is a CLI that runs at bare shell (before entering tmux) and provides an interactive TUI for picking, creating, and managing tmux sessions. It remembers your projects, resolves paths via aliases and zoxide, auto-detects git roots for new sessions, and automatically starts the tmux server when needed (great for post-reboot tmux-continuum/resurrect workflows).

After [shell integration](#shell-integration), you interact with Portal through two functions: **`x`** (session picker / opener) and **`xctl`** (subcommands like list, kill, alias). The function names are customizable — see `--cmd` below.

## Install

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

Register per-pane commands that re-execute automatically when a session is attached after a reboot. Must be run from inside a tmux pane.

```bash
xctl hooks set --on-resume "npm start"    # register a resume hook
xctl hooks rm --on-resume                 # remove the hook
xctl hooks list                           # list all hooks
```

Hooks fire via `tmux send-keys` when you attach/open a session. A volatile marker prevents duplicate execution within the same boot cycle — after a reboot the markers are gone and hooks re-fire.

### `xctl clean`

Remove stale projects whose directories no longer exist on disk, and prune hooks for panes that no longer exist.

```bash
xctl clean
```

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
| `/` | Filter mode (fuzzy search) |
| `R` | Rename session |
| `K` | Kill session |
| `q`/`Esc` | Quit |

The TUI has three views: session list, project picker, and file browser.

## Automatic Server Bootstrap

Portal automatically starts the tmux server if it isn't already running. This eliminates the need for LaunchAgents or other workarounds to keep tmux alive across reboots — especially useful with tmux-continuum/resurrect.

**How it works:**

- On any command that needs tmux (`open`, `list`, `attach`, `kill`), Portal checks for a running server and starts one if missing
- The **TUI** shows a brief "Starting tmux server..." loading screen while waiting for sessions to restore (1–6s)
- **CLI commands** print "Starting tmux server..." to stderr, then proceed normally once ready
- If the server is already running, there's zero overhead — commands execute immediately

Portal is plugin-agnostic: it doesn't depend on continuum or resurrect. It simply starts the server and waits briefly for any session restoration to complete.

Pair this with [resume hooks](#xctl-hooks) to automatically re-run pane commands (dev servers, editors, etc.) after a reboot.

## Configuration

Portal stores config in `~/.config/portal/`:

| File | Purpose | Env override |
|---|---|---|
| `aliases` | Path aliases (key=value, one per line) | `PORTAL_ALIASES_FILE` |
| `projects.json` | Remembered project directories | `PORTAL_PROJECTS_FILE` |
| `hooks.json` | Per-pane resume hooks (pane → event → command) | `PORTAL_HOOKS_FILE` |

Projects are auto-populated when you create new sessions and cleaned with `xctl clean`.

## License

MIT
