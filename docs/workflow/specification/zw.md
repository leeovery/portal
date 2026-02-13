---
topic: zw
status: superseded
superseded_by: portal
type: feature
date: 2026-01-31
sources:
  - name: cx-design
    status: incorporated
  - name: zellij-multi-directory
    status: incorporated
  - name: fzf-output-mode
    status: incorporated
  - name: git-root-and-completions
    status: incorporated
---

# Specification: ZW (Zellij Workspaces)

## Overview

### What is ZW

ZW (Zellij Workspaces) is a Go CLI that provides an interactive session picker for Zellij. It runs at bare shell (before entering Zellij) and offers a mobile-friendly TUI for managing Zellij sessions.

### The Problem

When SSH/Mosh-ing to a machine (e.g., from phone to Mac), it's tedious to:
- Remember which Zellij sessions exist
- Type session names correctly to attach
- Navigate to the right directory to start new sessions

Zellij's built-in session manager only works *inside* an existing session and is too information-dense for mobile screens.

### The Solution

A single command (`zw`) that:
1. Shows existing sessions (running and exited/resurrectable)
2. Allows quick attachment with arrow keys + Enter
3. Remembers project directories for starting new sessions
4. Works at bare shell, optimized for small screens

### Value Proposition

1. **Interactive picker at bare shell** - What Zellij's session-manager does, but *before* you're inside Zellij
2. **Mobile-friendly** - Clean, minimal interface vs. Zellij's dense built-in manager
3. **Project memory** - Quick-start new sessions in remembered directories
4. **One command** - `zw` does everything vs. `zellij ls` + `zellij attach <name>`

## Core Model

### Sessions as Workspaces

ZW treats Zellij sessions as **workspaces**. A workspace may span multiple directories - Zellij allows multiple panes in a session, each potentially in different directories.

### Sessions and Projects are Separate Concerns

- **Sessions** = Live data queried from Zellij (`zellij ls`)
- **Projects** = ZW's memory of directories used to start new sessions

ZW does not track which project a session belongs to. Select a session → attach. Select a project → start a new session there.

### No Directory Change Before Attach

When attaching to an existing session, ZW does not change directories. Zellij restores shell state on reattach - each pane resumes exactly where it was.

### Directory Change for New Sessions

When starting a **new** session, ZW changes to the selected project directory before creating the session. The sequence is:

1. Resolve directory to git root (if inside a git repository)
2. cd to resolved directory
3. Run `zellij attach -c <session-name>` (with optional layout flag)

This ensures the new session's default pane opens in the project root directory.

### Git Root Resolution

When a directory is selected for a new session (via `zw .`, `zw <path>`, or the file browser), ZW resolves it to the git repository root before proceeding.

**Implementation**: Run `git -C <selected-dir> rev-parse --show-toplevel`. If it succeeds, use the output as the directory. If it fails (exit code 128 — not a git repo), use the original directory as-is.

**Behavior**:
- Resolution is automatic and silent — no prompt or confirmation
- Non-git directories are used unchanged, with no warning
- One resolution function applied uniformly at all three entry points (`zw .`, `zw <path>`, file browser)

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
│       api-work       2 tabs         │
│       client-proj                   │
│                                     │
│           EXITED                    │
│                                     │
│       old-feature    (resurrect)    │
│                                     │
│    ─────────────────────────────    │
│    [n] new in project...            │
│                                     │
└─────────────────────────────────────┘
```

### Sections

1. **SESSIONS** - Running Zellij sessions
   - Shows session name
   - Shows attached indicator (`● attached`) if another client is connected
   - Optionally shows tab count (e.g., `2 tabs`)

2. **EXITED** - Resurrectable sessions (detached but recoverable from Zellij's cache)
   - Shows `(resurrect)` indicator
   - `Enter` on an exited session resurrects it (reattaches via `zellij attach <session-name>`)

3. **New session option** - `[n] new in project...`
   - Opens project picker to start a new session

### Sorting

**Sessions**: Displayed in the order returned by `zellij list-sessions`. No additional sorting applied.

**Projects**: Sorted by `last_used` timestamp, most recent first.

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Navigate list |
| `Enter` | Select (attach to session or open project picker) |
| `n` | Jump to "new session" option |
| `K` | Kill selected session |
| `/` | Enter filter mode |
| `q` / `Esc` | Quit |

**Kill confirmation**: Pressing `K` prompts for confirmation before killing the selected session: "Kill session 'myapp'? (y/n)"

### Filter Mode

The session list supports fuzzy filtering via a dedicated mode, activated by pressing `/`.

**Entering filter mode**: Press `/`. A filter input appears at the bottom of the list (e.g., `filter: _`). All subsequent keystrokes are treated as filter input — shortcut keys (`n`, `k`, `K`, etc.) lose their shortcut meaning and become typeable characters.

**While filtering**:
- Typing narrows the visible list by fuzzy-matching against session names
- `↑` / `↓` navigate the filtered results
- `Enter` selects the highlighted item (same as normal mode)
- `Backspace` deletes the last filter character; if the filter is already empty, exits filter mode
- `Esc` clears the filter and exits filter mode

**What gets filtered**: Running sessions and exited sessions. The `[n] new in project...` option is always visible and never filtered out.

**Outside filter mode**: All single-key shortcuts (`n`, `k`, `j`, `K`, `q`) work as documented in the keyboard shortcuts table. No filtering occurs.

### Session Info Display

For running sessions, ZW can query tab names via:
```bash
zellij --session <name> action query-tab-names
```

This allows showing tab count or tab names inline without entering the session.

### Empty States

**No sessions (running or exited):**
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

**Example**: Project "zw" produces sessions like `zw-x7k2m9`, `zw-a3b8p1`.

The suffix is a 6-character nanoid, ensuring uniqueness without user input. Users are never prompted for a session name.

### Renaming

Session renaming is available in **utility mode** (when ZW runs inside Zellij). The current session can be renamed using `zellij action rename-session <new-name>`.

**External renaming**: May be possible via `zellij --session <name> action rename-session <new-name>` from outside Zellij — to be verified during implementation.

**Note**: After renaming, the `ZELLIJ_SESSION_NAME` environment variable is not updated for existing panes (only new panes).

### New Session Flow

**New project (directory not in projects.json):**
```
Selected: ~/Code/myapp

Project name: [myapp] _
  (Enter to accept, or type a custom name)

Aliases (optional): _
  (Comma-separated, e.g. "app, ma". Enter to skip)

Layout: [default] ▾
  • default (single pane)
  • dev-setup
  • split-view
```

**Saved project:**
```
Layout: [default] ▾
  • default (single pane)
  • dev-setup
  • split-view
```

The session is created automatically with an auto-generated name (e.g., `myapp-x7k2m9`).

**Layout selection**: ZW presents existing Zellij layouts for the user to choose from. ZW does not create or manage layouts - that's handled by Zellij itself. If no custom layouts exist, ZW starts sessions with Zellij's default (single pane).

**No custom layouts**: If no custom layouts exist and the project is already saved, ZW skips all prompts — the session is created immediately upon project selection.

## Running Inside Zellij

### Environment Variables

Zellij sets these variables when inside a session:
- `ZELLIJ` - Indicates running inside Zellij
- `ZELLIJ_SESSION_NAME` - The current session's name (useful for utility mode operations like renaming or showing current session info)

### Detection

ZW detects if it's running inside an existing Zellij session via the `ZELLIJ` environment variable (set by Zellij when inside a session).

### Utility Mode

When running inside Zellij, ZW enters **utility mode** with restricted operations:

**Blocked:**
- Attaching to another session (prevents nesting) — applies to both the TUI and the `zw attach` CLI command
- Creating new sessions (prevents nesting) — applies to `zw .`, `zw <path>`, and `zw <alias>` CLI commands. ZW displays: "Cannot create sessions from inside Zellij. Exit this session first."

**Allowed:**
- Rename current session
- View other sessions (read-only)
- Kill other sessions
- Show current session info

### Utility Mode TUI

When running inside Zellij, the TUI displays with modifications:

```
┌─────────────────────────────────────┐
│                                     │
│      UTILITY MODE (in: cx-03)       │
│                                     │
│           SESSIONS                  │
│                                     │
│       api-work       2 tabs         │
│       client-proj                   │
│                                     │
│    ─────────────────────────────    │
│    [r] rename current session       │
│                                     │
└─────────────────────────────────────┘
```

**Visual differences:**
- Header shows "UTILITY MODE" with current session name
- Current session is not listed (you're already in it)
- `Enter` on a session shows info instead of attaching
- `[n] new in project...` is hidden (can't start nested sessions)
- `[r] rename current session` option added

**Session info display**: Pressing `Enter` on a session in utility mode shows an inline expansion below the session entry with tab names (queried via `zellij --session <name> action query-tab-names`). Pressing `Enter` again or selecting a different session collapses it. No separate screen or popup.

**Keyboard shortcuts in utility mode:**
| Key | Action |
|-----|--------|
| `r` | Rename current session (prompts for new name) |
| `K` | Kill selected session (other sessions only) |
| `q` / `Esc` | Exit ZW |

## Project Memory

### Remembered Directories

ZW maintains a list of directories where the user has previously started sessions. This enables quick access to frequently used project directories when starting new sessions.

### How Directories are Added

A directory is added to the remembered list when a new session is started there, regardless of entry point:
- File browser selection
- `zw .` (current directory)
- `zw <path>` (specified directory)
- `zw <alias>` (alias resolves to a directory already in `projects.json`, so no addition needed)

If the directory is not already in `projects.json`, it is added automatically.

### Project Naming

When a user starts a session in a **new directory** (not yet in `projects.json`), ZW presents a naming screen before proceeding to session creation:

- **Project name**: Text input, defaults to directory basename (after git root resolution)
- **Aliases**: Optional, user can add one or more short identifiers for quick access via `zw <alias>`

This is a dedicated screen within the TUI.

**Saved projects**: When starting a session in a directory already in `projects.json`, ZW skips the project naming screen and proceeds directly to session creation.

**Project names are independent of Zellij session names.** A project may have many sessions — each session's name is auto-generated from the project name (see Session Naming). The project name is a ZW display concept; it does not propagate to Zellij directly.

### Project Management

For saved projects, users can manage project details from the project picker via keyboard shortcut:
- **Rename** the project display name
- **Add or remove aliases**

These changes update `projects.json` only and do not affect any existing Zellij session names.

### Storage

Remembered directories are stored in `~/.config/zw/projects.json`.

### Usage in TUI

When selecting "new in project...", remembered directories appear first in the project picker, allowing quick selection before browsing to new locations.

### Project Picker Interaction

The project picker is a full-screen list shown when selecting `[n] new in project...` from the main TUI.

**Layout**: Remembered projects are listed by recency, with a `[browse for directory...]` option always visible at the bottom of the list.

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Navigate project list |
| `Enter` | Select project (proceeds to naming/layout flow) or open file browser if on browse option |
| `/` | Enter filter mode (same behavior as main session list) |
| `e` | Edit selected project (rename, manage aliases) |
| `x` | Remove selected project from remembered list (with confirmation) |
| `Esc` | Return to main session list |

**Edit mode**: Pressing `e` opens an inline edit for the selected project — cycling through editable fields (name, aliases) with `Tab`, confirming with `Enter`, cancelling with `Esc`.

### Stale Project Cleanup

**Automatic**: If a remembered directory no longer exists on disk, ZW removes it from the project list automatically when encountered.

**Manual**: While navigating the project list, users can manually remove a project from the remembered list via keyboard shortcut.

**Via `zw clean`**: Deletes all exited Zellij sessions (`zellij delete-session`) and removes projects whose directories no longer exist on disk. Prints each action taken (e.g., "Deleted exited session: old-feature", "Removed stale project: myapp (/Users/lee/Code/myapp)"). Non-interactive — no confirmation prompts.

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

All ZW data is stored in `~/.config/zw/`.

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
| `aliases` | No | Array of short identifiers for quick access via `zw <alias>` |
| `last_used` | Yes | ISO timestamp, used for sorting by recency |

**Aliases**: Must be unique across all projects. Enables quick session start: `zw app` opens the project picker for that project directly.

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
| `zw` | Launch the main TUI picker |
| `zw .` | Start new session in current directory |
| `zw <path>` | Start new session in specified directory |
| `zw <alias>` | Start new session for project with matching alias |
| `zw clean` | Remove exited sessions and stale projects, print what was removed (non-interactive) |
| `zw list` | Output running session names, one per line (for scripting/fzf) |
| `zw attach <name>` | Attach to session by exact name |
| `zw completion <shell>` | Output shell completion script (bash, zsh, fish) |
| `zw version` | Show version information |
| `zw help` | Show usage information |

**Quick-start shortcuts**: `zw .`, `zw <path>`, and `zw <alias>` all open the same naming/layout flow as selecting a directory via the project picker - they just skip navigation. The selected directory is added to remembered projects if not already present.

**Attach errors**: If `zw attach <name>` doesn't match any running or exited session, ZW displays: "No session found: {name}" and exits with a non-zero status code.

### Argument Resolution

When `zw` receives a positional argument (e.g., `zw myapp`):

1. **Path detection**: If the argument contains `/` or starts with `.`, treat it as a path
2. **Alias lookup**: Otherwise, check if it matches a project alias in `projects.json`
3. **Fallback to path**: If no alias match, treat as a relative path
4. **Validation**: If the resolved path doesn't exist, display error: "No project alias or directory found: {arg}"

### Design Philosophy

Most operations happen through the TUI. The CLI subcommands are minimal, providing only non-interactive utilities and standard help/version flags.

### Scripting & fzf Integration

`zw list` outputs session names one per line for piping to external tools:

```bash
# Quick attach with fzf
zw attach $(zw list | fzf)

# Scripting
for session in $(zw list); do
  echo "Session: $session"
done
```

This provides an alternative to the TUI for power users who prefer external pickers or scripting.

### Shell Completions

ZW provides shell completion scripts via Cobra's built-in generators.

```
zw completion bash
zw completion zsh
zw completion fish
```

Each outputs the completion script to stdout. Users source it in their shell config:

```bash
source <(zw completion zsh)
```

**Supported shells**: bash, zsh, fish only. No powershell — ZW wraps Zellij, which doesn't run on Windows.

## Distribution

### Target Platforms

- macOS (arm64, amd64)
- Linux (arm64, amd64)

### Installation Method

Distributed via Homebrew tap.

```bash
brew tap leeovery/tools
brew install zw
```

**Future exploration**: Publishing to Homebrew core (without requiring a personal tap) - to be explored post-implementation.

### Build & Release

[GoReleaser](https://goreleaser.com/) handles cross-platform builds and distribution.

**Release process**:
1. Run release script (generates version tag)
2. Push tag to GitHub
3. GitHub Actions workflow triggers GoReleaser
4. GoReleaser builds binaries and creates GitHub Release
5. GoReleaser auto-updates the Homebrew formula in `leeovery/homebrew-tools`

### Runtime Dependency

Zellij is a required dependency. The Homebrew formula declares Zellij as a dependency, ensuring it's installed automatically.

If Zellij is somehow missing at runtime, ZW displays: "ZW requires Zellij. Install with: brew install zellij"

## Zellij Integration

### Session Operations

ZW uses these Zellij CLI commands:

| Operation | Command |
|-----------|---------|
| Create/attach session | `zellij attach -c <session-name>` (creates if doesn't exist) |
| Create with layout | `zellij attach -c <session-name> --layout <layout-name>` |
| Attach to existing | `zellij attach <session-name>` |
| List sessions | `zellij list-sessions` |
| Kill session | `zellij kill-session <session-name>` |
| Delete exited session | `zellij delete-session <session-name>` |
| Query tab names | `zellij --session <name> action query-tab-names` |
| Rename session | `zellij action rename-session <new-name>` (from inside session) |

### Process Handoff

When launching Zellij (attach or create), ZW uses `exec` to replace its own process with Zellij. ZW's job is complete once the user selects a session — there are no post-attach actions. This avoids terminal state management and is the standard pattern for session pickers.

### Layout Discovery

ZW queries Zellij for its configuration directory to locate available layouts. Layouts are `.kdl` files in the `layouts/` subdirectory of Zellij's config.

**Display**: Layout names are shown without the `.kdl` extension (e.g., "dev-setup" not "dev-setup.kdl").

**No layouts available**: If no custom layouts exist, ZW skips the layout picker and creates sessions with Zellij's default (single pane). The new session flow shows "No custom layouts available."

### Session Discovery

ZW uses `zellij list-sessions` to discover both running and exited sessions. The output includes:
- Running sessions
- Exited sessions (labeled as "EXITED") - these are resurrectable

**Parsing note**: The command outputs with ANSI color codes. ZW must strip escape sequences when parsing the output.

**Attached status**: Running sessions may show the number of connected clients in the `zellij list-sessions` output. ZW displays `● attached` for sessions with one or more clients. Implementation should verify the exact output format, as it may vary by Zellij version.

## Dependencies

Prerequisites that must exist before implementation can begin:

### Required

None. ZW is a standalone tool with no blocking dependencies on other systems.

### Runtime Dependencies

| Dependency | Purpose |
|------------|---------|
| **Zellij** | ZW wraps Zellij - all session operations require Zellij to be installed |

**Note**: Zellij is a runtime dependency (must be present when ZW runs), not a build-time dependency. ZW can be built and tested independently; Zellij is declared as a Homebrew dependency for installation.

### Build Dependencies

Standard Go toolchain and libraries (Bubble Tea, etc.) - handled by `go.mod`.
