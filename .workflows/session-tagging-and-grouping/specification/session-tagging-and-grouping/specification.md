# Specification: Session Tagging and Grouping

## Specification

## Overview

Portal's `open` session list presents all live tmux sessions in a flat, alphabetical list. At the user's typical working load (~15–20 concurrent sessions) a flat list stops being legible. This feature adds an **on-demand grouped session list** with a toggle to cycle between viewing modes.

The grouping is powered by **tags**. Tags are the mechanism; the goal is the grouped, toggleable list. Grouping by directory/project comes "for free" (derivable from where a session lives); grouping by an arbitrary user dimension (Personal / Work / BusinessA) is what tags provide.

## Goal

Let the user slice the live session list multiple ways and aggregate sessions into logical groups, flipping between views with a single key.

## v1 Scope

v1 ships the **directory/project tag layer only**:

- A directory (equivalently, a project — "projects are just stored git-root directories") carries zero or more tags.
- Every session started in a directory **inherits** that directory's tags (a live lookup — no per-session tag storage in v1).
- The session list renders in three modes — **Flat → By Project → By Tag** — cycled with a single key.
- Tags are assigned/managed in the existing **projects edit modal** (TUI only).

**Effective tags of a session in v1 = its directory's tags.** (No union with per-session tags, no overrides — that's the deferred second layer.)

## Non-Goals (deferred)

These are explicitly out of v1 scope. v1 is purely additive and locks none of them out:

- **Per-session tags + `portal open --tag=`** (and `x --tag=`) — the eventual hybrid model's second layer: a per-session `@portal-tags` tmux option, settable at launch. Deferred. This layer is precisely what the directory-anchored v1 avoids: because `@portal-tags` is in-memory tmux server state, surviving a reboot would require capturing it into `sessions.json` (a modest, bounded addition to the `Session` record), reading it on daemon capture, and re-applying it on restore (interacting with the `@portal-restoring` window). The directory anchor needs none of this — directory tags ride the `projects.json` project record. That cost contrast is why v1 stops at the directory layer.
- **Live-grouped filtering** — keeping group headers visible while the `/` filter is active. Deferred as its own separate feature (would require Portal to own the filter wholesale). v1 flattens on filter.
- **Tag exclusion / hide-a-tag** — a filter layer on top of grouping. Deferred power-feature. The deferral is acceptable because the existing `/` filter covers basic narrowing in the meantime; revisit only if a concrete "tag I never want to see" pain appears.

## Guiding Invariant — Additive, No Regression

The feature is **purely additive**. Flat mode, the zero-tag state, and the all-tags-deleted state are byte-for-byte today's session list. Grouping appears only on opt-in (the toggle key). **By Project** delivers value with zero setup; **By Tag** fills in as the user tags directories.

## Tag Data Model & Persistence

### Storage

Tags are stored on the **project record** in `~/.config/portal/projects.json`. The existing `Project` record (`{path, name, last_used}`) gains a **`tags []string`** field.

- A directory carries **multiple tags** — this is what lets one project appear under several tag groups simultaneously.
- Reuses the existing JSON store machinery: `AtomicWrite` (temp file + rename) and `configFilePath` resolution. No new store, no new persistence pattern.

### Back-compatibility

Existing `projects.json` records predate the `tags` field. A missing `tags` field decodes to nil/empty — exactly the zero-tag state. No migration is required; un-tagged projects behave as today.

### Tags are implicit (no registry)

There is **no separate "create tag" step and no tag registry**. The set of tags that exists is the **union of `tags` across all project records**. Applying `work` to a second directory auto-joins the existing `work` group; removing the last `work` tag makes the group cease to exist. Tags come into and out of existence purely as a side effect of being applied to projects.

### Taggable surface — projects only

The projects edit modal is the **only** origin for tags, and it lists **known projects only**. Because every session creation upserts a project (`session.PrepareSession` → `store.Upsert(resolvedDir, projectName, "internal")` keyed by git-root), any directory opened in Portal at least once is a project and therefore taggable.

A directory **never opened in Portal** is not a project and cannot be pre-tagged. This is an accepted v1 boundary: **open a directory once, then tag it.** No bare-directory tagging in v1.

### Lifecycle — no orphan-tag cleanup needed

Tags live on the project record in `projects.json`, not in a separate store. They persist with the project and are removed when the project is deleted (projects-page `d`). There is no separate tag store to garbage-collect and no orphan-tag sweep (unlike hooks/markers). The `@portal-dir` session stamp (see *Session → Directory Resolution*) is ephemeral and dies with the session — nothing to GC there either.

## Session → Directory Resolution

Grouping a session — by project or (via inheritance) by tag — requires mapping each **live** session back to its directory. This is the one genuine piece of new mechanism the feature needs.

The session **name cannot** do this: a name is `{project}-{nanoid}` at birth, but the user renames sessions freely. A session knows its directory only via its panes, which roam.

### The stamp (fast path)

At session creation, Portal stamps the tmux session user-option **`@portal-dir = <resolvedDir>`**, where `<resolvedDir>` is the git-root already computed in `session.PrepareSession`.

- **Survives rename** — the option rides the session *object*, not its name.
- **Survives pane `cd`** — stamped once at create, never re-derived from live pane cwd.
- **Cheap to read** — the grouped render reads it in the same `list-sessions -F` pass that already fetches session names (append `#{@portal-dir}` to the format string). No `git rev-parse` per session per render.

The grouped render uses `@portal-dir` to look up the directory's `Project` record and its `tags`.

### The lazy stamp-on-render fallback (stamp absent)

`@portal-dir` is the fast path, not a guarantee. When the grouped render encounters a session with **no `@portal-dir`**, it resolves that session's directory from the **active pane's current path → git-root** and **stamps `@portal-dir` then and there** (lazy). After that first grouped render the session is on the fast path.

One mechanism covers **both** stamp-absence cases — no schema change, no restore-engine change, no first-boot backfill:

- **Post-reboot:** restored sessions return without the option (`@portal-dir` is in-memory tmux state, not persisted in `sessions.json`). The first grouped render re-derives and re-stamps. There is no need to persist the resolved directory into the session record.
- **Pre-existing live sessions on first ship:** sessions already running when the feature ships have no stamp; the same fallback stamps them on first render. They appear in **By Project** immediately — **no "restart to appear" gap**.

So deriving the directory live from the pane's `current_path` is **not rejected** — it is precisely this fallback, used only for the un-stamped minority (typically still sitting at their project directory), then cached via the lazy stamp. The drift/perf objections applied only to using live derivation as the *primary* path, which v1 does not.

### Path-keying canonicalisation (build-time requirement)

The dir→tags lookup keys on a directory path, so the **render-time lookup key must match the stored `Project.Path` exactly**. Both the stamped `@portal-dir` value and the fallback-derived git-root must be normalised to the same canonical form the project store uses, accounting for: symlinks, trailing slash, `~` expansion. Implementation must confirm the lookup key matches stored `Project.Path` for the same directory; a mismatch would silently drop a session out of its group.

## Grouping Semantics

### Modes

The session list cycles through three modes:

1. **Flat** — today's list, unchanged: all live sessions, flat, alphabetical.
2. **By Project** — a heading per directory; each session appears **once** under its directory.
3. **By Tag** — a heading per tag; a session appears **under each tag it has**; untagged sessions fall to a pinned **Untagged** bucket.

### The grouping-key problem (multi-membership)

A directory can carry multiple tags, so a session can belong to multiple tag groups. How a session renders depends on whether the grouping dimension is single- or multi-valued:

- **By Project — single-valued → Pattern A.** A session has exactly one directory, so it appears **once**. Matches the user's mental model and is the expected dominant mode.
- **By Tag — multi-valued → Pattern B.** A session appears **once under each tag it has** (the Linear/Jira/Trello/Notion group-by-label convention). This deliberately avoids inventing a "primary tag" concept — the only alternative, judged not worth the extra model and UX.

**Untagged bucket.** In By Tag mode, sessions whose directory has no tags collect under a single **Untagged** group, pinned last.

Accepted downside of Pattern B: a heavily-tagged list is longer than the flat list (sessions repeat). Mitigations beyond the Untagged catch-all (e.g. collapsible headers) are **deferred** — not built up front, revisited only if it bites.

### Ordering — static alphabetical, no recency

Portal has no recency tracking and does not hook zoxide frecency into this view. Ordering is static:

- **Within a group:** alphabetical by session name.
- **Group headings:** alphabetical.
- **Untagged:** pinned **last** (overrides alphabetical).

This matches today's static flat-list ordering, just aggregated. An MRU/recency ordering was explicitly declined.

## TUI Rendering & Toggle Behaviour

### Toggle key — `s`, single cycle

A single key **cycles** the mode: each press advances **Flat → By Project → By Tag → Flat**. A cycle is chosen over a "group by" menu because there are only three modes (fewer keystrokes, less chrome).

The toggle key is **`s`** ("switch view"), on the sessions page:

- Verified free on the sessions page — the browse-mode handler (`model.go:1583-1607`) handles only `? q k r n p x`, space, enter, esc; everything else falls through to the `bubbles/list`, which does not bind `s`.
- `g`/`G` were ruled out — they are bound by `bubbles/list` to GoToStart/GoToEnd (`model.go:635-636`).
- Minor accepted wrinkle: `s` already means "go to sessions" on the *projects* page (`s`/`x`). Same letter, page-dependent meaning — judged fine; it chains naturally (on projects, `s` → sessions; press `s` again → cycle views), and `x` remains the universal page-toggle.

### Group headers

Group headers are:

- **Dimmed** (styled distinct from session rows).
- **Non-selectable** — the cursor jumps session-to-session and **never lands on a header**.
- **Counted** — each header carries a count of its sessions, e.g. `Portal ··· 2`.

### Mode indication

- **Mode string lives in the title** (top), via the existing `SessionListTitle()`:
  - Flat → `Sessions`
  - By Project → `Sessions — by project`
  - By Tag → `Sessions — by tag`
- **Key hint lives in the footer** (Portal convention — keys at the bottom). Only the `s switch view` entry is added to the footer. The mode *state* lives in the title, off the crowded footer.

### Rendering stack

- **`bubbles/list`** drives the interactive picker (cursor, selection, filter, scroll, pagination) — unchanged.
- **`lipgloss`** styles the grouped look (dimmed headers + counts), layered into the existing `SessionDelegate` styles.

Both are already in use. The grouped appearance is achieved purely as lipgloss styling layered into `bubbles/list` rendering — **no new library, no rebuild.**

**`lipgloss/list` and `lipgloss/tree` were considered and rejected:** they are *static* renderers with no cursor/selection/filter/scroll; adopting them would mean hand-rolling all the interactivity `bubbles/list` provides for free. **Build constraint: the picker must not be routed through `lipgloss/tree`.**

## Mode Persistence & Empty States

### Remember last mode

The last-used grouping mode is **persisted** so Portal opens in the user's usual view ("if I always open in tag view I don't want to keep switching to it").

- **First-ever launch defaults to Flat** (zero surprise), and remembers thereafter.
- **Persistence target:** a small **prefs file** under `~/.config/portal/`, using the existing `configFilePath` + `AtomicWrite` pattern. UI state does not belong in domain stores like `projects.json`; the prefs file owns the last-used grouping mode. (This is an idiomatic implementation call, not a user-facing decision.)

### Empty states

- **By Tag with zero tags** — does **not** silently flatten. Render the plain (ungrouped) session list **with an explicit "No tags yet" message**, so the user sees they are in tag view, understands why there are no groups, and knows where to add them (the projects page). This is a degrade-to-flat **with a signpost**, not a silent one — it preserves the no-regression feel (the session list itself is unchanged) while keeping the mode legible.
- **Empty tags field in the projects edit modal** — shows a clear empty state ("no tags") rather than a blank.
- **By Project with nothing to group** — follows the same no-regression principle as By Tag:
  - **No live sessions:** renders exactly as flat mode would (an empty list). There is nothing to group; By Project never renders worse than flat, so no dedicated signpost is required. Unlike By Tag (which can be globally empty whenever zero tags exist anywhere), By Project is effectively always populated once any session is live, because every session resolves to a directory (via the `@portal-dir` stamp or the lazy fallback).
  - **Unresolvable directory:** a session whose `@portal-dir` is absent *and* whose lazy fallback cannot derive a git-root (e.g. a pane with no enclosing git repository) collects under a single pinned **Unknown** group at the end — mirroring the By-Tag **Untagged** bucket — so no session is ever silently dropped from the grouped view.

## Filter Composition

The existing `/` fuzzy filter (`bubbles/list`) continues to work **unchanged**, matching **session names** as it does today.

### Flatten-on-filter (v1)

While a filter is active, the list **flattens** to the matching sessions using the existing built-in filter; **group headers step aside**. Clearing the filter **restores** the grouped view. There is **no behaviour change** to filtering as it works today.

### Why not keep groups live while filtering

Keeping group headers visible during filtering was the user's first preference but is deferred — the cost is concentrated here and disproportionate to the rest of the feature:

- `bubbles/list` is a **flat list widget** with no concept of sections/headers. Its default filter **re-ranks matches into a relevance-sorted flat list** (scored by contiguity, start-of-string, post-separator matches — best-first, not alphabetical), which scrambles any grouped layout. The widget cannot preserve a structure it does not know exists.
- Keeping groups live during filtering would therefore require **Portal to own the filter wholesale** (custom input state, matching via `internal/fuzzy`, live re-group per keystroke, cursor/pagination management, `InitialFilter` re-wiring) and inherit a large interaction matrix (filter active + view-switch / preview / external-kill refresh / inside-tmux exclusion). This is the single biggest build-cost and bug-risk item in the area — bigger than the tagging itself.
- The payoff is small and transient: the difference shows only while actively typing; groups return the instant the filter clears, and filtering is usually "find one session fast," where a flat ranked hit-list is fine.

Live-grouped filtering is **deferred as its own separate, purely-additive feature** (see Non-Goals); nothing in v1 locks it out.

### Filter scope

- Filter scope stays **name-based**. The *tag* dimension is served by the **By Tag** view mode, not by the filter.

### Build note

Grouping must be a **render-layer** concern (or headers injected only when not filtering) so that the built-in filter only ever sees session items — this keeps flatten-on-filter trivial.

> Note: Portal's custom `SessionDelegate` means matched characters do **not** visually highlight today — the built-in filter ranks but does not highlight matches. v1 introduces no change here.

## Assigning & Managing Tags

### Surface — the projects edit modal

Tags are assigned and managed in the **existing projects edit modal**. A **Tags** field is added alongside Name and Aliases, behaving **exactly like the existing alias field** (`model.go:1427-1438`):

- Type a tag + **Enter** to add it.
- Highlight an entry + **`x`** to remove it.

This reuses an interaction the user already knows — zero new interaction to learn.

### Projects page only — not the sessions row

Tags are edited **from the projects page only**, never from a session row. Since v1 tags the *directory*, a sessions-row action would really mean "edit this session's project" (indirect) — deferred to keep v1 clean. (This ties back to the deferred per-session tag layer.)

### TUI only — no CLI in v1

There is **no `portal tags …` CLI** in v1. The projects page is the sole management surface. A CLI/scripting interface is a possible later addition.

### Implicit tags (recap)

Consistent with the data model: there is no "create tag" step or registry. The set of tags is the union of tags applied across all projects (see *Tag Data Model & Persistence*). Applying `work` to a second directory auto-joins the existing `work` group.

---

## Working Notes
