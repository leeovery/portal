# Specification: State Notify Cascade on Binary Upgrade

## Specification

## Problem Statement

Portal registers a `portal state notify` tmux global hook on six "save-trigger" events. Registration is meant to be idempotent — install exactly one entry per event, no matter how many times bootstrap runs.

On affected installs, two of those six events accumulate **unbounded duplicate copies** of the same hook. Each `portal open` / `x` / attach adds one more copy to each of the two events. Observed live: 139 stacked identical hooks on both `pane-focus-out` and `window-layout-changed`, while the four control events sat correctly at 1.

Because every stacked copy fires, a **single** human action that triggers one of those events (e.g. switching tmux session → `pane-focus-out`) detonates N back-to-back `portal state notify` fork+exec processes (~16 ms apart, ≈60 Hz). With 139 copies, one session-switch spawns 139 short-lived processes. Over one day this produced 11,000+ `state notify` invocations (~80 session-switches × 139).

### Symptoms

- Log spam: ~3 lifecycle-marker lines per spawned process; tens of thousands of lines/day.
- Write amplification on `portal.log` and on the `save.requested` marker that `state notify` touches, pressuring the daemon capture loop.
- fork+exec + tmux job-dispatch load funnelled through the single-threaded tmux server process — a strong (unproven) lead for the reported intermittent ~98% CPU peg on a core.
- The stack only ever **grows** (every bootstrap +1 per blind event); switching fires but never grows it.

## Root Cause

tmux 3.6b's `show-hooks -g` (with **no event argument**) does not enumerate global hooks for an entire class of events, even though those hooks are set and fire normally. The omitted class is all `pane-*` events and the geometry/rename `window-*` events (`window-layout-changed`, `window-pane-changed`, `window-renamed`, `window-resized`). Session-scoped events, `window-linked`/`window-unlinked`, `client-*`, and `alert-*` **are** enumerated.

Portal's idempotency check (`RegisterHookIfAbsent`) relies **solely** on the global `show-hooks -g` enumeration to decide whether a hook already exists. For the two blind events in Portal's save-trigger set (`pane-focus-out`, `window-layout-changed`), the existing entry is invisible to that read, so the check concludes "absent" and appends another copy on every bootstrap — unbounded.

The same blind spot **also breaks teardown**: `UnregisterPortalHooks` (used by `portal hooks reset`) reads through the identical global `show-hooks -g` path, sees zero Portal entries on the 139-deep arrays, and removes nothing. The bug currently cannot be undone through Portal's own reset path.

### Why it wasn't caught

- The flaw was baked in at design time: the original idempotency oracle assumed global `show-hooks -g` returns *all* Portal entries — true for the events it was reasoned about, false for pane/geometry-scoped events.
- Idempotency was only ever verified against events that `show-hooks -g` *does* enumerate (string-fixture commanders and tmux fixtures where the global read returns everything). The real tmux 3.6b global-enumeration blind spot was never modelled.
- No upper-bound assertion on hook-array length anywhere — the stacking is silent.

---

## Working Notes
