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

## Solution Strategy

Make Portal stop depending on tmux's global hook view entirely. The fix is a single architectural shift applied uniformly:

**Read hooks per-event, not globally.** Replace every `show-hooks -g` (global, no-arg) read with `show-hooks -g <event>` (per-event), used uniformly for *every* Portal-managed event. The global enumeration is the source of the blind spot; per-event reads are not blind (verified live: `show-hooks -g pane-focus-out` returns the entry that the no-arg form omits).

**Registration becomes declarative — "ensure exactly one."** For each Portal-managed event, read that event's entries, identify the Portal-authored ones for its category, and converge to exactly one entry carrying the current desired command body. Convergence = unset every matching Portal entry (reverse index order), then append one. The append-if-absent discipline is replaced by ensure-exactly-one.

**Cleanup is intrinsic, not bolted on.** Because registration now reads what's actually there per-event and converges to exactly one, the existing 139-deep stacks collapse to 1 as an ordinary side effect of the next bootstrap. There is **no dedicated run-once cleanup migration** — that would be permanent cruft that runs once then sits forever and can never be safely removed.

### Why uniform per-event (not just the two known-blind events)

Special-casing the blind set was explicitly rejected. The blind set is tmux-version-specific (observed in 3.6b); maintaining a hardcoded "these events are blind" list re-introduces the exact hidden-coupling assumption that caused this bug, and would silently regress if a future tmux version hides a different event. Uniform per-event reads remove the assumption entirely at negligible cost (one extra tmux invocation per event at bootstrap).

### Concrete mechanism

- **New tmux client seam:** `ShowGlobalHooksForEvent(event)` → runs `show-hooks -g <event>`. Output format is byte-identical to the global form (`pane-focus-out[0] run-shell "…"`), so the existing `ParseShowHooks` parser needs **zero changes**.
- **Delete `ShowGlobalHooks` (the no-arg global read).** It is the defect's single point of entry; with both registration and unregistration on the per-event seam, nothing should retain it. (Any remaining caller is migrated or the read is removed.)
- **Reuse existing, tested primitives:** the per-event eviction half already exists in `UnregisterPortalHooks` — `portalEntriesFor` + `containsAny(portalCommandSubstrings)` for Portal-only matching, reverse-index `UnsetGlobalHookAt` for removal, `AppendGlobalHook` for the single append.

---

## Working Notes
