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

Portal is a CLI that runs at bare shell (before entering tmux) and provides an interactive TUI for picking, creating, and managing tmux sessions. It remembers your projects, resolves paths via aliases and zoxide, and auto-detects git roots for new sessions.

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

### `xctl clean`

Remove stale projects whose directories no longer exist on disk.

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

## Configuration

Portal stores config in `~/.config/portal/`:

| File | Purpose | Env override |
|---|---|---|
| `aliases` | Path aliases (key=value, one per line) | `PORTAL_ALIASES_FILE` |
| `projects.json` | Remembered project directories | `PORTAL_PROJECTS_FILE` |

Projects are auto-populated when you create new sessions and cleaned with `xctl clean`.

## License

MIT
