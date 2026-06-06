# Discussion: Session Tagging and Grouping

## Context

Portal's `open` session list presents all live tmux sessions in flat
alphabetical order. The user routinely runs ~15–20 sessions at once; past a
certain count, a flat list stops being legible. The desire is to slice the list
different ways on demand and aggregate sessions into logical groups, flipping
between views with a toggle.

Discovery reframed an initial "three fixed grouping modes" idea (by directory,
by project, by custom buckets) toward a more general primitive: **tags**. Tag a
project and its sessions inherit that tag; optionally tag individual sessions
directly too. Grouping becomes "aggregate by tag", with directory/project
either derived facets or built-in tags over the same machinery.

The user confirmed this is **one cohesive feature** (work type: feature), not
several independently-shippable pieces — the tag model/persistence, the
project→session inheritance rule, the aggregated/grouped TUI view, and
assigning/managing tags only make sense delivered together.

### References

- Discovery session log: `.workflows/session-tagging-and-grouping/discovery/session-001.md`

## Discussion Map

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — Session Tagging and Grouping (6 subtopics, 6 pending)

  ┌─ ○ Tag data model & persistence [pending]
  ├─ ○ Project→session tag inheritance [pending]
  ├─ ○ Per-session tags & overrides [pending]
  ├─ ○ Built-in vs derived facets (directory/project) [pending]
  ├─ ○ Grouped/aggregated TUI view & toggle [pending]
  └─ ○ Assigning & managing tags (UX) [pending]

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
