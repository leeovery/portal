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

  Discussion Map — Session Tagging and Grouping (6 subtopics — 2 decided · 1 converging · 1 exploring · 2 pending)

  ┌─ ✓ Custom-grouping mechanism: tags [decided]
  ├─ ✓ Anchor: hybrid — v1 ships directory/project layer ONLY [decided]
  ├─ ✓ Tag data model & persistence (projects.json + @portal-dir stamp) [decided]
  ├─ → Grouping-key problem (A: dir once · B: tag under each) [converging]
  ├─ ◐ Grouped TUI rendering + toggle behaviour [exploring]
  └─ ○ Assigning & managing tags (projects-page editing) [pending]

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

### Decision

Tags are the custom-grouping mechanism. Grouping = pick a tag dimension, sessions
cluster under it. Open sub-questions in their own subtopics: the **grouping key**
problem (multi-tag session → which group does it render under), and the data
model/persistence shape.

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

### Correction (session 001): "there is no BusinessA directory"

The user clarified their custom groups are **not** encoded in the filesystem —
there is no `~/businessA/` folder; BusinessA projects are scattered across
arbitrary paths (`~/Code/portal`, `~/Code/fabric`, …). Consequences:

- **Path-derived grouping (option C) is dead** — there is no shared path segment
  to group on.
- **Directory-anchored tags still work** — and this is the subtle point: a
  directory anchor does **not** require dirs to share a path. You tag each
  scattered directory `businessA` individually; they then group together
  regardless of where they live on disk. Directory-anchor ≠ path-derived.

### New option: tag at the tmux level (session user-option)

User asked: "can't we tag the tmux session somehow, at the tmux level?" Yes —
tmux **session user-options** (`@`-prefixed), which Portal already uses heavily
(`@portal-restoring`, `@portal-skeleton-*`). Store tags as e.g.
`@portal-tags "work,businessA"` on the session via the existing
`SetSessionOption` helper. Read them cheaply in one pass — `ListSessions`
already runs `list-sessions -F "#{session_name}|…"`; append `|#{@portal-tags}`
to read names+tags together for the grouped render.

Properties:

- **Genuinely per-session** and **survives rename** — the option attaches to the
  tmux session *object*, not its name. This sidesteps the mutable-name problem
  *and* the fuzzy-directory wrinkle (F2) entirely — no directory derivation
  needed.
- **The catch — reboot.** tmux options are in-memory server state; they die when
  the server dies. `sessions.json` currently saves `Session.Environment` but
  **not** session options. So to survive a reboot we must **capture
  `@portal-tags` into `sessions.json` and re-apply on restore** — a *modest,
  bounded* schema addition (add tags to the `Session` record; daemon reads the
  option on capture; restore re-sets it). Critically this is **far cheaper than
  inventing a session UUID** — tags travel *with* the session record that
  resurrection already keys by name, so no new identity scheme is needed.

### The fork (anchor), restated

1. **Directory-anchored** — store `{dir: [tags]}` in a json file. Tag once per
   place; every session there inherits. Zero resurrection change. Cost: can't
   distinguish two sessions in the same dir; assigning from one session row
   affects siblings in that dir.
2. **Session-option (`@portal-tags`)** — per-session granularity, survives
   rename, no directory derivation. Cost: tag each session individually; needs
   the modest capture/restore addition for reboot durability.
3. **Hybrid (matches original discovery framing)** — directory tags as the
   inherited base + per-session `@portal-tags` overrides for exceptions.
   Effective tags = dir ∪ session. Most flexible; most surface.

Leaning question to user: tag **once per place** (dir-anchored, ergonomic for
stable classifications), or tag **each session** (session-option, maximal
control)? YAGNI check on the hybrid before committing to both layers.

### Decision — hybrid, both layers

User chose **both**, explicitly. Two layers compose:

1. **Directory / project tags (inherited base).** Tag a directory once; every
   session started there inherits its tags. Key unification from the user:
   **"projects are just stored directories"** — so a *project tag* and a
   *directory tag* are the same concept. A project is a directory that also has a
   friendly name. Inheritance is a live lookup, durable for free (directories are
   immortal), no resurrection changes.
2. **Per-session tags (tmux `@portal-tags` option).** Set on the session object,
   survives rename, captured into `sessions.json` for reboot durability (the
   modest schema/capture/restore addition described above). Settable **at launch**
   via a new flag: **`portal open --tag=tag1,tag2`** (and presumably `x --tag=`),
   which stamps `@portal-tags` on the new session.

**Effective tags of a session = directory/project tags ∪ session tags** (union).
Whether a session tag can *subtract* an inherited tag is deferred as YAGNI —
union-only unless a real need surfaces.

### v1 scope cut (session 001) — directory/project layer only

User scoped a **first slice**: ship the **directory/project tag layer only**;
**defer** the per-session `@portal-tags` layer and `portal open --tag=`. Rationale:
"we're essentially tagging projects" — see how far pure directory tagging gets
before adding per-session ceremony. The hybrid remains the eventual shape; v1 is
just the inherited base of it. Effective tags in v1 = the directory's tags (no
union, no overrides).

### Parked sub-questions (to the data-model subtopic)

- **Where do directory tags live?** Natural fit: extend the `projects.json`
  `Project` record (`{path, name, last_used}`) with `tags []string`. But the
  user rarely creates projects — so does **tagging a bare (non-project) directory
  lazily create a project record**, or do directory tags need a store decoupled
  from projects? (Review F8 also flags path-keying sharp edges: symlinks,
  trailing slash, `~` expansion, canonicalisation.)
- **Reboot capture/restore** details for `@portal-tags` (schema bump, daemon
  capture, restore re-set). Interaction with `@portal-restoring` window (F4).
- **Grouping key** for multi-tag sessions (F1) — its own subtopic.

## Tag data model & persistence

### Context

Where do directory tags live, and how does a live session resolve to its
directory so it can be grouped / inherit tags?

### Confirmed code facts (session 001)

- `session.PrepareSession` (`internal/session/prepare.go`) resolves the input
  path to a **git root** (`git.Resolve`), derives `projectName =
  filepath.Base(resolvedDir)`, and **upserts a project on every session
  creation** (`store.Upsert(resolvedDir, projectName, "internal")`). Every Portal
  session creates/refreshes a project keyed by its git-root path.
- Validates the user's model: **projects are just stored git-root directories**;
  "start a session from a project" ≈ `cd <dir>` + start Portal.

### Direction

- **Store:** extend the `projects.json` `Project` record (`{path, name,
  last_used}`) with **`tags []string`**. A directory carries **multiple tags**
  (what lets one project appear under several tag groups). Reuses the existing
  JSON store + `AtomicWrite` + `configFilePath` machinery.
- **Editing UX:** surface tag editing on the **projects page** (already supports
  per-project editing / aliases). Detail deferred to the UX subtopic.

### The session→directory resolution problem (the one real piece of work)

Grouping *by project* (let alone by tag) requires mapping each **live** session
back to its directory. The name can't do it (`{project}-{nanoid}` at birth, but
the user renames). A session knows its directory only via its panes.

**Decision (user-confirmed): stamp `@portal-dir = <resolvedDir>` on the session
at creation** (value already in hand in `PrepareSession`). The grouped render
reads it in the same `list-sessions -F` pass and looks up the directory's tags.

- Survives **rename** (option rides the session object, not the name).
- Survives **pane `cd`** (stamped once at create, not derived from live cwd).
- Avoids `git rev-parse` per session per render (perf).
- **Reboot follow-on:** the option dies with the tmux server; restore must
  re-stamp `@portal-dir` when it rebuilds the session. Small, bounded, but
  required — else post-reboot sessions fall out of grouping. (Review F2/F4.)

Alternative (rejected as primary): derive the dir live from the active pane's
`current_path` each render — drifts when a pane `cd`s out of the project, and
costs a git-root resolution per session per render.

### Open (parked)

- Path-keying sharp edges for the dir→tags lookup: symlinks, trailing slash, `~`
  expansion, canonicalisation (review F8). Confirm the render-time lookup key
  matches stored `Project.Path` exactly.
- Does tagging a **bare directory not yet a project** need to work, or is "tag
  from the projects page" (lists known projects only) enough for v1? Leaning
  enough, since every session already upserts a project.

## Grouping-key problem (multi-tag → which group)

### Context

A directory can have multiple tags, so a session can belong to multiple tag
groups. When grouping is active, does a multi-tag session appear once or under
each tag?

### Direction (converging — standard-practice split)

- **Group by project/directory** is single-valued → **Pattern A**: each session
  appears **once**. Matches the user's mockup; expected dominant mode.
- **Group by tag** is multi-valued → **Pattern B** (Linear/Jira/Trello/Notion
  group-by-label convention): a session appears **under each tag it has**. Avoids
  inventing a "primary tag" concept (the only alternative, judged not worth the
  extra model + UX).

Honest downside of B: a heavily-tagged list grows longer than flat. Mitigations
deferred unless they bite: an "Untagged" catch-all, collapsible headers — not
built up front (review F7). Not yet explicitly user-confirmed; recommendation
stands pending objection.

## Summary

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- Nothing decided yet — discussion just opened.
