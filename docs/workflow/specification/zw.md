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
