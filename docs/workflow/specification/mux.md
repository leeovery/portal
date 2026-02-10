---
topic: mux
status: concluded
type: feature
date: 2026-02-10
sources:
  - name: cx-design
    status: incorporated
  - name: zellij-multi-directory
    status: incorporated
  - name: fzf-output-mode
    status: incorporated
  - name: git-root-and-completions
    status: incorporated
  - name: zellij-to-tmux-migration
    status: incorporated
---

# Specification: mux

## Overview

### What is mux

mux is a Go CLI that provides an interactive session picker for tmux. It runs at bare shell (before entering tmux) and offers a mobile-friendly TUI for managing tmux sessions.

### The Problem

When SSH/Mosh-ing to a machine (e.g., from phone to Mac), it's tedious to:
- Remember which tmux sessions exist
- Type session names correctly to attach
- Navigate to the right directory to start new sessions

tmux's built-in session management is command-line only (`tmux ls`, `tmux attach -t <name>`) with no interactive picker.

### The Solution

A single command (`mux`) that:
1. Shows existing running sessions
2. Allows quick attachment with arrow keys + Enter
3. Remembers project directories for starting new sessions
4. Works at bare shell, optimized for small screens

### Value Proposition

1. **Interactive picker at bare shell** - An interactive session picker that works outside tmux
2. **Mobile-friendly** - Clean, minimal interface optimized for small screens
3. **Project memory** - Quick-start new sessions in remembered directories
4. **One command** - `mux` does everything vs. `tmux ls` + `tmux attach -t <name>`

## Core Model

### Sessions as Workspaces

mux treats tmux sessions as **workspaces**. A workspace may span multiple directories — tmux allows multiple windows and panes in a session, each potentially in different directories.

### Sessions and Projects are Separate Concerns

- **Sessions** = Live data queried from tmux (`tmux list-sessions`)
- **Projects** = mux's memory of directories used to start new sessions

mux does not track which project a session belongs to. Select a session → attach. Select a project → start a new session there.

### No Directory Change Before Attach

When attaching to an existing session, mux does not change directories. tmux restores shell state on reattach — each pane resumes exactly where it was.

### Directory Change for New Sessions

When starting a **new** session, mux passes the resolved directory to tmux via the `-c` flag. The sequence is:

1. Resolve directory to git root (if inside a git repository)
2. Run `tmux new-session -A -s <session-name> -c <resolved-dir>`

tmux's `-c` flag sets the working directory at session creation — no `cd` needed in mux's process.

### Git Root Resolution

When a directory is selected for a new session (via `mux .`, `mux <path>`, or the file browser), mux resolves it to the git repository root before proceeding.

**Implementation**: Run `git -C <selected-dir> rev-parse --show-toplevel`. If it succeeds, use the output as the directory. If it fails (exit code 128 — not a git repo), use the original directory as-is.

**Behavior**:
- Resolution is automatic and silent — no prompt or confirmation
- Non-git directories are used unchanged, with no warning
- One resolution function applied uniformly at all three entry points (`mux .`, `mux <path>`, file browser)

## TUI Design

### Technology

Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea) (terminal UI framework).

### Layout

Full-screen picker optimized for small screens (mobile SSH use case).

```
┌─────────────────────────────────────┐
│                                     │
│           SESSIONS                  │
│                                     │
│    >  cx-03          ● attached     │
│       api-work       2 windows      │
│       client-proj                   │
│                                     │
│    ─────────────────────────────    │
│    [n] new in project...            │
│                                     │
└─────────────────────────────────────┘
```

### Sections

1. **SESSIONS** - Running tmux sessions
   - Shows session name
   - Shows attached indicator (`● attached`) when `session_attached > 0`
   - Shows window count (e.g., `2 windows`)

2. **New session option** - `[n] new in project...`
   - Opens project picker to start a new session

### Sorting

**Sessions**: Displayed in the order returned by `tmux list-sessions`. No additional sorting applied.

**Projects**: Sorted by `last_used` timestamp, most recent first.

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Navigate list |
| `Enter` | Select (attach to session or open project picker) |
| `n` | Jump to "new session" option |
| `R` | Rename selected session (prompts for new name) |
| `K` | Kill selected session |
| `/` | Enter filter mode |
| `q` / `Esc` | Quit |

**Kill confirmation**: Pressing `K` prompts for confirmation before killing the selected session: "Kill session 'myapp'? (y/n)"

**Rename prompt**: Pressing `R` shows an inline text input pre-filled with the current session name. `Enter` confirms, `Esc` cancels.

### Filter Mode

The session list supports fuzzy filtering via a dedicated mode, activated by pressing `/`.

**Entering filter mode**: Press `/`. A filter input appears at the bottom of the list (e.g., `filter: _`). All subsequent keystrokes are treated as filter input — shortcut keys (`n`, `k`, `K`, etc.) lose their shortcut meaning and become typeable characters.

**While filtering**:
- Typing narrows the visible list by fuzzy-matching against session names
- `↑` / `↓` navigate the filtered results
- `Enter` selects the highlighted item (same as normal mode)
- `Backspace` deletes the last filter character; if the filter is already empty, exits filter mode
- `Esc` clears the filter and exits filter mode

**What gets filtered**: Running sessions only. The `[n] new in project...` option is always visible and never filtered out.

**Outside filter mode**: All single-key shortcuts (`n`, `k`, `j`, `K`, `q`) work as documented in the keyboard shortcuts table. No filtering occurs.

### Session Info Display

mux queries session metadata via tmux format strings:

```bash
tmux list-sessions -F '#{session_name}|#{session_windows}|#{session_attached}'
```

This provides session name, window count, and attached client count in a clean, structured format — no ANSI escape stripping needed.

For per-window detail (if needed):
```bash
tmux list-windows -t <name> -F '#{window_index}|#{window_name}|#{window_panes}'
```

### Empty States

**No sessions:**
```
┌─────────────────────────────────────┐
│                                     │
│           SESSIONS                  │
│                                     │
│       No active sessions            │
│                                     │
│    ─────────────────────────────    │
│    [n] new in project...            │
│                                     │
└─────────────────────────────────────┘
```

**No remembered projects (when opening project picker):**
```
Select a project:

  No saved projects yet.

  ─────────────────────────────
  browse for directory...
```

The file browser is always available to start sessions in new directories.

## Session Naming

### Auto-Generated from Project Name

Session names are auto-generated using the project name plus a short random suffix:

```
{project-name}-{nanoid}
```

**Example**: Project "mux" produces sessions like `mux-x7k2m9`, `mux-a3b8p1`.

The suffix is a 6-character nanoid, ensuring uniqueness without user input. Users are never prompted for a session name.

**Name sanitization**: tmux session names cannot contain periods (`.`) or colons (`:`). When generating a session name from a project name, mux replaces these characters with hyphens (`-`). For example, project "my.app" produces sessions like `my-app-x7k2m9`.

### Renaming

Session renaming is available both inside and outside tmux:

```bash
tmux rename-session -t <current-name> <new-name>
```

tmux's `rename-session` works from outside the session (unlike Zellij, which required being inside). This means renaming is available in all contexts — the main TUI and when running inside tmux.

### New Session Flow

**New project (directory not in projects.json):**
```
Selected: ~/Code/myapp

Project name: [myapp] _
  (Enter to accept, or type a custom name)

Aliases (optional): _
  (Comma-separated, e.g. "app, ma". Enter to skip)
```

The session is created immediately after naming — no layout selection step.

**Saved project:**

Session creation is immediate upon project selection — no prompts. The session name is auto-generated from the project name.

## Running Inside tmux

### Detection

mux detects if it's running inside an existing tmux session via the `TMUX` environment variable (set by tmux when inside a session).

### Behavior Inside tmux

When running inside tmux, the TUI is the same as outside — no restricted mode. The difference is the underlying operation:

- **Outside tmux**: `Enter` on a session → `exec tmux attach-session -t <name>`
- **Inside tmux**: `Enter` on a session → `tmux switch-client -t <name>`

tmux's `switch-client` switches the current client to a different session without nesting. This means all TUI operations work from inside tmux.

### Inside tmux: Session Actions

| Action | Command |
|--------|---------|
| Select existing session | `tmux switch-client -t <name>` |
| New session from project | `tmux new-session -d -s <name> -c <dir>` then `tmux switch-client -t <name>` |
| Kill session | `tmux kill-session -t <name>` (same as outside) |
| Rename session | `tmux rename-session -t <name> <new-name>` (same as outside) |

### TUI Differences When Inside tmux

- Current session is excluded from the session list (you're already in it)
- Header shows current session name for context (e.g., from `TMUX` env var parsing or `tmux display-message -p '#{session_name}'`)

### CLI Commands Inside tmux

CLI commands also use switch-client instead of attach:

- `mux .`, `mux <path>`, `mux <alias>` → create session detached, then switch-client
- `mux attach <name>` → switch-client to the named session

## Project Memory

### Remembered Directories

mux maintains a list of directories where the user has previously started sessions. This enables quick access to frequently used project directories when starting new sessions.

### How Directories are Added

A directory is added to the remembered list when a new session is started there, regardless of entry point:
- File browser selection
- `mux .` (current directory)
- `mux <path>` (specified directory)
- `mux <alias>` (alias resolves to a directory already in `projects.json`, so no addition needed)

If the directory is not already in `projects.json`, it is added automatically.

### Project Naming

When a user starts a session in a **new directory** (not yet in `projects.json`), mux presents a naming screen before proceeding to session creation:

- **Project name**: Text input, defaults to directory basename (after git root resolution)
- **Aliases**: Optional, user can add one or more short identifiers for quick access via `mux <alias>`

This is a dedicated screen within the TUI.

**Saved projects**: When starting a session in a directory already in `projects.json`, mux skips the project naming screen and proceeds directly to session creation.

**Project names are independent of tmux session names.** A project may have many sessions — each session's name is auto-generated from the project name (see Session Naming). The project name is a mux display concept; it does not propagate to tmux directly.

### Project Management

For saved projects, users can manage project details from the project picker via keyboard shortcut:
- **Rename** the project display name
- **Add or remove aliases**

These changes update `projects.json` only and do not affect any existing tmux session names.

### Storage

Remembered directories are stored in `~/.config/mux/projects.json`.

### Usage in TUI

When selecting "new in project...", remembered directories appear first in the project picker, allowing quick selection before browsing to new locations.

### Project Picker Interaction

The project picker is a full-screen list shown when selecting `[n] new in project...` from the main TUI.

**Layout**: Remembered projects are listed by recency, with a `[browse for directory...]` option always visible at the bottom of the list.

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Navigate project list |
| `Enter` | Select project (proceeds to naming flow for new projects, or creates session immediately for saved projects) or open file browser if on browse option |
| `/` | Enter filter mode (fuzzy-matches against project name and aliases) |
| `e` | Edit selected project (rename, manage aliases) |
| `x` | Remove selected project from remembered list (with confirmation) |
| `Esc` | Return to main session list |

**Edit mode**: Pressing `e` opens an inline edit for the selected project — cycling through editable fields (name, aliases) with `Tab`, confirming with `Enter`, cancelling with `Esc`.

### Stale Project Cleanup

**Automatic**: If a remembered directory no longer exists on disk, mux removes it from the project list automatically when encountered.

**Manual**: While navigating the project list, users can manually remove a project from the remembered list via the `x` keyboard shortcut.

**Via `mux clean`**: Removes projects whose directories no longer exist on disk. Prints each action taken (e.g., "Removed stale project: myapp (/Users/lee/Code/myapp)"). Non-interactive — no confirmation prompts.

## File Browser

### Purpose

When starting a new session in a directory not in the remembered list, an interactive file browser allows navigating to and selecting the desired directory.

### Access

From the project picker (when creating a new session):
- Select the "browse for directory..." option at the bottom of the project list

### Behavior

- **Content**: Shows directories only — files are not displayed
- **Starting directory**: Current working directory
- **Navigation**: Arrow keys to move through directory listing
- **Enter directory**: `Enter` or `→` descends into highlighted directory
- **Go up**: `Backspace` or `←` goes to parent directory
- **Select current directory**: `Enter` on `.` (current dir indicator) or dedicated shortcut (e.g., `Space`)
- **Cancel**: `Esc` returns to project picker without selection
- The selected directory is automatically added to remembered projects

## Configuration & Storage

### Location

All mux data is stored in `~/.config/mux/`.

### Files

| File | Format | Purpose |
|------|--------|---------|
| `config` | Flat key=value | User configuration options |
| `projects.json` | JSON | Remembered project directories |

### projects.json Structure

```json
{
  "projects": [
    {
      "path": "/Users/lee/Code/myapp",
      "name": "myapp",
      "aliases": ["app", "ma"],
      "last_used": "2026-01-22T10:30:00Z"
    }
  ]
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `path` | Yes | Absolute path to project directory |
| `name` | Yes | Display name (defaults to directory basename, can be customized) |
| `aliases` | No | Array of short identifiers for quick access via `mux <alias>` |
| `last_used` | Yes | ISO timestamp, used for sorting by recency |

**Aliases**: Must be unique across all projects. Enables quick session start: `mux app` opens the project picker for that project directly.

### Configuration Options

Configuration uses a simple flat format:

```
key=value
another_key=another_value
```

Specific configuration options will be determined during implementation based on what behaviors need to be user-configurable.

## CLI Interface

### Commands

| Command | Description |
|---------|-------------|
| `mux` | Launch the main TUI picker |
| `mux .` | Start new session in current directory |
| `mux <path>` | Start new session in specified directory |
| `mux <alias>` | Start new session for project with matching alias |
| `mux clean` | Remove stale projects (directories that no longer exist on disk), non-interactive |
| `mux list` | Output running session names, one per line (for scripting/fzf) |
| `mux attach <name>` | Attach to session by exact name |
| `mux completion <shell>` | Output shell completion script (bash, zsh, fish) |
| `mux version` | Show version information |
| `mux help` | Show usage information |

**Quick-start shortcuts**: `mux .`, `mux <path>`, and `mux <alias>` all open the same naming flow as selecting a directory via the project picker — they just skip navigation. The selected directory is added to remembered projects if not already present. For saved projects, session creation is immediate (no prompts).

**Attach errors**: If `mux attach <name>` doesn't match any running session, mux displays: "No session found: {name}" and exits with a non-zero status code.

### Argument Resolution

When `mux` receives a positional argument (e.g., `mux myapp`):

1. **Path detection**: If the argument contains `/` or starts with `.`, treat it as a path
2. **Alias lookup**: Otherwise, check if it matches a project alias in `projects.json`
3. **Fallback to path**: If no alias match, treat as a relative path
4. **Validation**: If the resolved path doesn't exist, display error: "No project alias or directory found: {arg}"

### Design Philosophy

Most operations happen through the TUI. The CLI subcommands are minimal, providing only non-interactive utilities and standard help/version flags.

### Scripting & fzf Integration

`mux list` outputs session names one per line for piping to external tools:

```bash
# Quick attach with fzf
mux attach $(mux list | fzf)

# Scripting
for session in $(mux list); do
  echo "Session: $session"
done
```

This provides an alternative to the TUI for power users who prefer external pickers or scripting.

**Inside tmux**: `mux list` includes all sessions (including the current one). The TUI excludes the current session for UX reasons, but the CLI output is always complete — callers can filter as needed.

### Shell Completions

mux provides shell completion scripts via Cobra's built-in generators.

```
mux completion bash
mux completion zsh
mux completion fish
```

Each outputs the completion script to stdout. Users source it in their shell config:

```bash
source <(mux completion zsh)
```

**Supported shells**: bash, zsh, fish only. No powershell — mux wraps tmux, which doesn't run on Windows.

## Distribution

### Target Platforms

- macOS (arm64, amd64)
- Linux (arm64, amd64)

### Installation Method

Distributed via Homebrew tap.

```bash
brew tap leeovery/tools
brew install mux
```

**Future exploration**: Publishing to Homebrew core (without requiring a personal tap) — to be explored post-implementation.

### Build & Release

[GoReleaser](https://goreleaser.com/) handles cross-platform builds and distribution.

**Release process**:
1. Run release script (generates version tag)
2. Push tag to GitHub
3. GitHub Actions workflow triggers GoReleaser
4. GoReleaser builds binaries and creates GitHub Release
5. GoReleaser auto-updates the Homebrew formula in `leeovery/homebrew-tools`

### Runtime Dependency

tmux is a required dependency. The Homebrew formula declares tmux as a dependency, ensuring it's installed automatically.

If tmux is somehow missing at runtime, mux displays: "mux requires tmux. Install with: brew install tmux"

## tmux Integration

### Session Operations

mux uses these tmux commands (verified against tmux 3.6a):

| Operation | Command |
|-----------|---------|
| Create or attach | `tmux new-session -A -s <name>` |
| Create with start dir | `tmux new-session -A -s <name> -c <dir>` |
| Create detached (for switch-client) | `tmux new-session -d -s <name> -c <dir>` |
| Attach to existing | `tmux attach-session -t <name>` |
| Switch client (inside tmux) | `tmux switch-client -t <name>` |
| List sessions | `tmux list-sessions -F '#{session_name}\|#{session_windows}\|#{session_attached}'` |
| Check session exists | `tmux has-session -t <name>` (exit 0 if exists, 1 if not) |
| Kill session | `tmux kill-session -t <name>` |
| Rename session | `tmux rename-session -t <name> <new-name>` |
| List windows | `tmux list-windows -t <name> -F '#{window_index}\|#{window_name}\|#{window_panes}'` |

### Process Handoff

When launching tmux from outside (bare shell), mux uses `exec` to replace its own process with tmux. mux's job is complete once the user selects a session — there are no post-attach actions. This avoids terminal state management and is the standard pattern for session pickers.

When running inside tmux, no `exec` is needed — `switch-client` is a tmux command sent to the server, not a process replacement. mux exits normally after issuing the tmux commands.

### Session Discovery

mux uses `tmux list-sessions` with format strings to discover running sessions. The `-F` flag provides structured, parseable output with no ANSI escape codes to strip.

Available metadata per session:
- `#{session_name}` — session name
- `#{session_windows}` — window count
- `#{session_attached}` — number of attached clients (0 = detached, 1+ = attached)
- `#{session_created}` — creation timestamp (unix epoch)

**No server running**: When no tmux server is running (no sessions exist), `tmux list-sessions` exits with a non-zero status and outputs an error. mux treats this as zero sessions — it shows the "No active sessions" empty state. This is not a fatal error.

## Dependencies

Prerequisites that must exist before implementation can begin:

### Required

None. mux is a standalone tool with no blocking dependencies on other systems.

### Runtime Dependencies

| Dependency | Purpose |
|------------|---------|
| **tmux** | mux wraps tmux — all session operations require tmux to be installed |

**Note**: tmux is a runtime dependency (must be present when mux runs), not a build-time dependency. mux can be built and tested independently; tmux is declared as a Homebrew dependency for installation.

### Build Dependencies

Standard Go toolchain and libraries (Bubble Tea, Cobra, etc.) — handled by `go.mod`.
