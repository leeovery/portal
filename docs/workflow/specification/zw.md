# Specification: ZW (Zellij Workspaces)

**Status**: Building specification
**Type**: feature
**Last Updated**: 2026-01-22

---

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

3. **New session option** - `[n] new in project...`
   - Opens project picker to start a new session

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Navigate list |
| `Enter` | Select (attach to session or open project picker) |
| `n` | Jump to "new session" option |
| `K` | Kill selected session |
| `q` / `Esc` | Quit |

### Session Info Display

For running sessions, ZW can query tab names via:
```bash
zellij --session <name> action query-tab-names
```

This allows showing tab count or tab names inline without entering the session.

## Session Naming

### Free-Form with Smart Defaults

Session names are user-chosen, not auto-generated.

**Default**: Directory basename (e.g., starting in `~/Code/myapp` suggests "myapp")

**Always prompt**: Even for previously saved projects, ZW prompts for the session name. Users may want a contextual name (e.g., "testing-workflow") rather than the project name.

### New Session Flow

```
Selected: ~/Code/myapp

Workspace name: [myapp] _
  (Enter to accept, or type a custom name)

Layout: [default] ▾
  • default (single pane)
  • dev-setup
  • split-view
```

**Layout selection**: ZW presents existing Zellij layouts for the user to choose from. ZW does not create or manage layouts - that's handled by Zellij itself. If no custom layouts exist, ZW starts sessions with Zellij's default (single pane).

### Renaming

Session renaming is supported. Workspaces evolve - a session started as "project-a" may become "comparison-testing".

## Running Inside Zellij

### Detection

ZW detects if it's running inside an existing Zellij session via the `ZELLIJ` environment variable (set by Zellij when inside a session).

### Utility Mode

When running inside Zellij, ZW enters **utility mode** with restricted operations:

**Blocked:**
- Attaching to another session (prevents nesting)

**Allowed:**
- Rename current session
- View other sessions (read-only)
- Kill other sessions
- Show current session info

## Project Memory

### Remembered Directories

ZW maintains a list of directories where the user has previously started sessions. This enables quick access to frequently used project directories when starting new sessions.

### How Directories are Added

When a user navigates to a new directory via the file browser and starts a session there, that directory is added to the remembered list.

### Storage

Remembered directories are stored in `~/.config/zw/projects.json`.

### Usage in TUI

When selecting "new in project...", remembered directories appear first in the project picker, allowing quick selection before browsing to new locations.

## File Browser

### Purpose

When starting a new session in a directory not in the remembered list, an interactive file browser allows navigating to and selecting the desired directory.

### Access

From the project picker (when creating a new session):
- Select the "browse..." option, or
- Press `/` to open the file browser directly

### Behavior

- Navigate directories using arrow keys
- Select a directory to start a new session there
- The selected directory is automatically added to remembered projects

## Configuration & Storage

### Location

All ZW data is stored in `~/.config/zw/`.

### Files

| File | Format | Purpose |
|------|--------|---------|
| `config` | Flat key=value | User configuration options |
| `projects.json` | JSON | Remembered project directories |

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
| `zw clean` | Remove exited/dead sessions (non-interactive) |
| `zw version` | Show version information |
| `zw help` | Show usage information |

### Design Philosophy

Most operations happen through the TUI. The CLI subcommands are minimal, providing only non-interactive utilities and standard help/version flags.
