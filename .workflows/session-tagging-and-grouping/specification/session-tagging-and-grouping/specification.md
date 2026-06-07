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

- **Per-session tags + `portal open --tag=`** — the eventual hybrid model's second layer (per-session `@portal-tags` tmux option, captured/restored across reboot). Deferred.
- **Live-grouped filtering** — keeping group headers visible while the `/` filter is active. Deferred as its own separate feature (would require Portal to own the filter wholesale). v1 flattens on filter.
- **Tag exclusion / hide-a-tag** — a filter layer on top of grouping. Deferred power-feature.

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

---

## Working Notes
