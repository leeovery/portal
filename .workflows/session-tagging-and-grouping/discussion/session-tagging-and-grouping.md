# Discussion: Session Tagging and Grouping

## Context

Portal's `open` session list presents all live tmux sessions in flat
alphabetical order. The user routinely runs ~15–20 sessions at once; past a
certain count, a flat list stops being legible. The desire is to slice the list
different ways on demand and aggregate sessions into logical groups, flipping
between views with a toggle.

### Restated goal (session 001)

The user clarified that **tags were a candidate mechanism, not the goal**. The
actual want is: **a grouped session list with a toggle to switch between
grouping modes.** Mockup the user gave:

    Portal
      session-1
      session-2

    Agentic Workflows
      session-3
      session-4

    Tick
      ...

That example groups by project/directory. But the user also wants *other*
groupings — e.g. Personal, Work, BusinessA, BusinessB — which is what led to the
tags idea. Tags are still on the table but explicitly "open to ideas." So the
decision splits into two axes: **intrinsic groupings** (directory/project —
derivable for free from where a session lives) and **custom groupings**
(user-defined buckets that need some data behind them).

Discovery reframed an initial "three fixed grouping modes" idea (by directory,
by project, by custom buckets) toward a more general primitive: **tags**. Tag a
project and its sessions inherit that tag; optionally tag individual sessions
directly too. Grouping becomes "aggregate by tag", with directory/project
either derived facets or built-in tags over the same machinery.

The user confirmed this is **one cohesive feature** (work type: feature), not
several independently-shippable pieces — the tag model/persistence, the
project→session inheritance rule, the aggregated/grouped TUI view, and
assigning/managing tags only make sense delivered together.

### Code grounding (current state)

- A **project** is `{path, name, last_used}` in `~/.config/portal/projects.json`
  (persistent). A **session** is live tmux state `{name, windows, attached}`.
- A session name is `{project}-{nanoid}` at creation, but there is **no stored
  session→project link** — the only association is the name prefix convention.
- The `open` session list is live tmux sessions, flat alphabetical.

### Hard constraints surfaced early (session 001)

1. **Session names are NOT identity.** The user renames sessions freely to match
   what the session is doing. So we cannot use the session name — neither for
   identity nor for parsing the `{project}-` prefix to recover its origin. The
   name-prefix inheritance path is dead on arrival.
2. **Projects are rarely used; the real entry point is the directory.** The user
   normally starts via an alias (e.g. `xc portal`) which resolves through
   zoxide to a *directory*. Many sessions never touch a "project" record at all.
   This pushes the natural tag anchor toward the **directory**, not the project:
   a "project" is really just a named, tagged directory, and the directory is
   the one stable thing that survives renames and reboots.

### References

- Discovery session log: `.workflows/session-tagging-and-grouping/discovery/session-001.md`

## Discussion Map

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — Session Tagging and Grouping (5 subtopics — 1 exploring · 4 pending)

  ┌─ ◐ Custom-grouping mechanism: tags vs single-category vs path-derived [exploring]
  ├─ ○ Grouping axes (intrinsic dir/project + custom) [pending]
  ├─ ○ Anchor: what grouping data attaches to (dir; names mutable) [pending]
  ├─ ○ Grouped TUI rendering + toggle behaviour [pending]
  └─ ○ Assigning & managing group membership (UX) [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough
exploration to capture.*

---

## Summary

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- Nothing decided yet — discussion just opened.
