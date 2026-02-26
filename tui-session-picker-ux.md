# TUI Redesign Notes

## Problem

The current TUI (opened via `x` / `portal open`) has several UX issues:

- No section headings — flat list with no context about what items are
- No keybinding hints — shortcuts like `/` (filter), `R` (rename), `K` (kill) are undiscoverable
- Projects only accessible via "new in project..." sub-screen — not surfaced in main list
- `[n] new in project...` wording is confusing — implies "new session in current project"
- `n` key only moves cursor to the option, doesn't auto-select
- No context-sensitive shortcuts — available actions don't change based on highlighted item

## Proposed Design

### Single scrollable screen (replaces project picker sub-screen)

```
Sessions
  > my-app-x7k2    2 windows  ● attached
    api-server-m3p  1 window

Projects
    portal          ~/Code/portal
    dashboard       ~/Code/dashboard
    ...
    [b] browse for directory...
```

### Sections

- **Sessions** — running tmux sessions, sorted by tmux order. Enter = attach.
- **Projects** — saved/remembered projects, sorted by recency. Enter = create new session & attach immediately.
- **`[b] browse for directory...`** — at bottom of projects section, for directories not yet saved as projects.

### Shortcuts

- **`n`** — new session in current directory (the directory `x` was run from, equivalent to `x .`)
- **`/`** — enter filter mode
- **`b`** — jump to browse for directory
- **`q` / `Esc`** — quit

### Context-sensitive keybinding hints (bottom bar)

Hints change based on what's highlighted:

- **On a session**: `[enter] attach  [R] rename  [K] kill  [/] filter  [q] quit`
- **On a project**: `[enter] new session  [e] edit  [x] remove  [/] filter  [q] quit`
- **On browse**: `[enter] browse  [/] filter  [q] quit`

### Key decisions

- Projects surfaced directly in main list — no separate project picker sub-screen
- Selecting a project = immediate session creation + attach (no prompts)
- `n` auto-executes (creates session in cwd) rather than just moving cursor
- Session names remain auto-generated: `{project-name}-{nanoid}`
- The old `[n] new in project...` gateway is removed entirely

## Open questions

- Exact styling/layout of section headers and bottom bar
- Whether filter applies to both sections or just the active one
- Empty state messaging when no sessions or no projects exist
- How many projects to show before the list feels too long (or is scrolling fine?)
