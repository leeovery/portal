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

  Discussion Map — Session Tagging and Grouping (6 subtopics — 1 converging · 1 exploring · 4 pending)

  ┌─ → Custom-grouping mechanism: tags (vs path-derived) [converging]
  ├─ ◐ Anchor: directory vs session (durability cost) [exploring]
  ├─ ○ Grouping axes (intrinsic dir/project + custom tags) [pending]
  ├─ ○ Grouping-key problem (flat tags have no single grouping key) [pending]
  ├─ ○ Grouped TUI rendering + toggle behaviour [pending]
  └─ ○ Assigning & managing group membership (UX) [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough
exploration to capture.*

---

## Custom-grouping mechanism

### Context

What produces the custom groups (Personal / Work / BusinessA)? Intrinsic
groupings (by directory/project) are free — derivable from where a session
lives. Only the custom dimension needs a mechanism.

### Journey

User landed on **tags**: "group by a tag, sessions auto-sort." Path-based
grouping (e.g. `~/Code/fabric/*` all together) was considered and kept as an
idea but judged too inflexible as the *custom* mechanism ("which segment?",
"doesn't work across the whole list"). Resolution: **path/directory grouping is
the free *intrinsic* toggle mode; tags are the *custom* mechanism.** The user
gets path-style grouping anyway (as the dir/project mode) without it having to
carry the flexible custom case.

### Decision (provisional)

Tags are the custom-grouping mechanism. Grouping = pick a tag dimension, sessions
cluster under it. Open sub-questions deferred to their own subtopics: the
**anchor** (what a tag attaches to) and the **grouping key** problem (flat
many-to-many tags have no single grouping key — see Open Threads).

---

## Anchor: what grouping data attaches to

### Context

User stated tags "need to be session based." But sessions have no durable
identity in Portal, so "session-based + durable" is the expensive path. This
subtopic decides what a tag actually hangs off.

### Feasibility finding (verified in code, session 001)

- `sessions.json` (`internal/state/schema.go`) keys each saved session **by
  `Name`** — there is **no session id/UUID**. Resurrection recreates sessions
  by name.
- Therefore Portal has **no stable session identity** today. Within a server
  lifetime tmux's `session_id` (`$3`) survives renames but is reassigned on
  reboot; the name survives reboot (resurrection restores it) but the user
  mutates names by habit. Neither is a durable key.
- **Durable session-based tags** would require introducing a Portal-stamped
  session id and threading it through create → daemon capture (schema bump) →
  restore → tag store. Real infrastructure touching the resurrection engine.

### The key distinction (assignment UX vs storage anchor)

The examples the user named — Personal / Work / BusinessA — are
**directory-stable classifications**: a directory's "Work-ness" is a property of
the place/project, not of a transient session. You would essentially never tag
two sessions in the *same directory* differently along these axes.

So "session based" may mean **"I assign the tag from the session row"** (UX), not
**"the tag is stored against the session"** (anchor). If the directory is the
storage anchor:

- Tags survive renames and reboots **for free** (directories are immortal; we
  look up live by the session's directory).
- Inheritance is automatic (every session in the dir gets the dir's tags).
- No schema bump, no session-identity infra.
- Cost: can't put two same-directory sessions in different custom groups — which
  the named use cases never need.

Open question to the user: is there a real case where two sessions in the **same
directory** must land in **different** custom groups? If no → directory anchor
(cheap, durable). If yes → pay for session identity (schema + resurrection work).

A wrinkle to resolve if directory-anchored: a live session's "directory" is
fuzzy (panes roam). Candidate: derive from the active pane's `current_path`, or
stamp the creation dir once at session create.

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- Nothing decided yet — discussion just opened.
