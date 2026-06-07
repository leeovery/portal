---
status: complete
created: 2026-06-07
cycle: 1
phase: Plan Integrity Review
topic: Session Tagging and Grouping
---

# Review Tracking: Session Tagging and Grouping - Integrity

## Findings

### 1. Phase 2 builders (2-2, 2-3) are prioritised ahead of the SessionItem struct (2-1) they depend on

**Severity**: Important
**Plan Reference**: Phase 2 — tasks `session-tagging-and-grouping-2-2`, `session-tagging-and-grouping-2-3` (depend on `session-tagging-and-grouping-2-1`)
**Category**: Dependencies and Ordering
**Change Type**: update-task (dependency/priority metadata)

**Details**:
Tasks 2-2 (By Project builder) and 2-3 (By Tag builder) both *construct* enriched `SessionItem`s carrying the `GroupKey` / `GroupHeading` / `Tag` / `CatchAll` fields. Those fields do not exist on `SessionItem` until task 2-1 (`Extend SessionItem with group metadata`) adds them — confirmed against `internal/tui/session_item.go:30`, where `SessionItem` today wraps only a `tmux.Session`. So 2-2 and 2-3 have a genuine compile-time dependency on 2-1.

The plan inverts the order two ways:
1. **Priority inversion** — in tick, 2-2 and 2-3 are priority **1**, while 2-1 (and 2-4/2-5/2-6) are priority **2**. `tick ready` sorts by priority first, so once Phase 1 completes, 2-2 and 2-3 surface *before* 2-1. (They are only held back right now because they carry Phase-1 blockers; the moment those clear, the priority sort offers the builders first.)
2. **No explicit edge** — neither 2-2 nor 2-3 has a `blocked_by` edge to 2-1, so nothing corrects the priority inversion.

An implementer taking `tick ready` at face value would pick 2-2 (or 2-3) before 2-1 and hit a non-compiling build (the `GroupKey`/`GroupHeading`/`Tag`/`CatchAll` fields are undefined). 2-4, 2-5, 2-6 also consume 2-1's fields but are priority 2 and created after 2-1, so natural order already sequences them correctly — only the two priority-1 builders are mis-ordered.

The surgical fix is to add an explicit `blocked_by` edge from 2-2 and 2-3 to 2-1 (the build-order-correcting dependency). This makes 2-1 a hard predecessor regardless of the priority sort. Leaving the priorities as-is is acceptable once the edges exist, because a blocked task is never offered by `tick ready` until its blocker is `done`.

**Current** (task `session-tagging-and-grouping-2-2`, dependencies):
> blocked_by: `session-tagging-and-grouping-1-6` (Expose @portal-dir via ListSessions), `session-tagging-and-grouping-1-4` (Canonical directory path key)

**Proposed** (task `session-tagging-and-grouping-2-2`, dependencies):
> blocked_by: `session-tagging-and-grouping-2-1` (Extend SessionItem with group metadata), `session-tagging-and-grouping-1-6` (Expose @portal-dir via ListSessions), `session-tagging-and-grouping-1-4` (Canonical directory path key)
>
> Rationale: `buildByProject` returns `[]list.Item` of enriched `SessionItem`s and assigns `GroupKey`/`GroupHeading`; those fields are introduced by task 2-1, so 2-1 must complete first. The added edge overrides the priority-1 sort that would otherwise offer this builder before 2-1.

**Resolution**: Fixed
**Notes**: Added `tick dep add tick-4358f8 tick-0ccac8` (2-2 ← 2-1). Verified 2-2 now blocked_by [2-1, 1-6, 1-4].

---

### 2. By Tag builder (2-3) is missing its dependency on the dir→project lookup primitive (1-4)

**Severity**: Important
**Plan Reference**: Phase 2 — task `session-tagging-and-grouping-2-3` (depends on `session-tagging-and-grouping-1-4` and `session-tagging-and-grouping-2-1`)
**Category**: Dependencies and Ordering
**Change Type**: update-task (dependency/priority metadata)

**Details**:
Task 2-3's "Do" section resolves each session's project "via `project.MatchProjectByDir(projects, s.Dir)`" — and `MatchProjectByDir` is a deliverable of task 1-4 (`Canonical directory path key for dir→project lookup`). Its sibling builder 2-2 correctly declares `blocked_by` 1-4, but 2-3 declares only `blocked_by` 1-2 (`NormaliseTag`). This is a missing cross-phase data/capability edge: 2-3 cannot compile or run without `MatchProjectByDir`.

2-3 also constructs enriched `SessionItem`s (see Finding 1), so it additionally needs an edge to 2-1. Both missing edges should be added in the same update.

This is a genuine cross-phase dependency gap (criteria: "a cross-phase dependency is missing"), not an intra-phase natural-order case.

**Current** (task `session-tagging-and-grouping-2-3`, dependencies):
> blocked_by: `session-tagging-and-grouping-1-2` (Tag value normalisation helper)

**Proposed** (task `session-tagging-and-grouping-2-3`, dependencies):
> blocked_by: `session-tagging-and-grouping-2-1` (Extend SessionItem with group metadata), `session-tagging-and-grouping-1-4` (Canonical directory path key for dir→project lookup), `session-tagging-and-grouping-1-2` (Tag value normalisation helper)
>
> Rationale: `buildByTag` calls `project.MatchProjectByDir` (delivered by 1-4) to resolve each session's project, and constructs enriched `SessionItem`s whose `GroupKey`/`GroupHeading`/`Tag`/`CatchAll` fields are introduced by 2-1. Both are hard predecessors. The added 2-1 edge also overrides the priority-1 sort that would otherwise offer this builder before 2-1 (see Finding 1).

**Resolution**: Fixed
**Notes**: Added `tick dep add tick-dc8a90 tick-0ccac8` (2-3 ← 2-1) and `tick dep add tick-dc8a90 tick-5a49ee` (2-3 ← 1-4). Verified 2-3 now blocked_by [2-1, 1-4, 1-2].

---
