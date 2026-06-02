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

## Registration Redesign — "Ensure Exactly One"

`RegisterPortalHooks` is rebuilt so that, for every Portal-managed event, it converges that event's hook array to **exactly one** Portal entry carrying the current desired command body — reading per-event throughout.

### Per-event convergence algorithm

For each managed event, given the event's *eviction fingerprint(s)* and its *desired body*:

1. Read the event's entries via `ShowGlobalHooksForEvent(event)` → `ParseShowHooks`.
2. Collect the Portal-authored entries — those whose command body contains any of the event's eviction fingerprint(s). (User/other-plugin entries are not matched and are never touched.)
3. **Idempotent fast path:** if exactly one Portal-authored entry exists and its body already equals the desired body, do nothing — no unset, no append, no churn.
4. Otherwise converge: unset every Portal-authored entry via `UnsetGlobalHookAt` in **descending index order** (so a removal never shifts a not-yet-processed index), then `AppendGlobalHook(event, desiredBody)` exactly once.

This collapses any depth-N stack (including the live 139-deep ones) to a single entry, and migrates a stale legacy body to the current one, as an ordinary side effect of bootstrap step 2 — no separate cleanup pass.

### Per-event parameters

| Event(s) | Eviction fingerprint(s) | Desired body |
|---|---|---|
| `session-created`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out` | `portal state notify` | `notifyCommand` |
| `session-closed` | `portal state notify`, `portal state commit-now` | `commitNowCommand` |
| `client-attached`, `client-session-changed` | `portal state signal-hydrate` | `signalHydrateCommand` (the `--`-separated form) |

Notes on the table:

- **`session-closed`** lists both `portal state notify` and `portal state commit-now` as eviction fingerprints: this evicts a stale pre-fix `notifyCommand` left by an older binary *and* collapses any duplicate `commit-now` entries, converging to one `commitNowCommand`. (This replaces the historical session-closed special case — see "Migration-Helper Consolidation".)
- **Hydration events** match on `portal state signal-hydrate`, which catches both the legacy un-separated body and the current `--` form, converging to the current one. (This replaces the historical hydration special case — see "Migration-Helper Consolidation".)
- Eviction is scoped to each event's **own category fingerprint(s)**. A legacy cross-category entry (e.g. a stale `portal state migrate-rename` on `session-renamed` from a very old binary) is *not* reaped by registration — it remains the responsibility of the teardown/clean path. Registration's job is solely "ensure exactly one of *this* event's desired body."

### User-hook coexistence guarantee

The eviction predicate matches **only Portal-authored command bodies** (the `portalCommandSubstrings` substring discipline already used by the teardown path). A user-authored or other-plugin hook on the same event — including on `pane-focus-out` / `window-layout-changed` — is never matched and survives every registration untouched. This is a hard requirement: the original design deliberately chose `set-hook -ga` (append) over `-g` (replace) specifically to coexist with user `.tmux.conf` hooks, and the fix must preserve that coexistence.

## Migration-Helper Consolidation

The current code carries three distinct registration shapes, two of which exist only because append-if-absent could not self-heal:

- `RegisterHookIfAbsent` — append-if-absent dedup (the defective path).
- `migrateHydrationHooks` — one-shot eviction of legacy un-separated `signal-hydrate` bodies.
- `migrateSessionClosedHook` — exact-match eviction of stale `notifyCommand` on `session-closed`, then append `commitNowCommand`.

**Decision: fold all three into the single per-event ensure-exactly-one path; delete `migrateHydrationHooks` and `migrateSessionClosedHook`.**

The ensure-exactly-one algorithm with the per-event parameter table (see "Registration Redesign") already does everything the two helpers did:

- Hydration: matching on `portal state signal-hydrate` evicts the legacy un-separated body *and* any duplicate, converging to the `--` form — exactly `migrateHydrationHooks`' job, now intrinsic.
- session-closed: matching on `portal state notify` + `portal state commit-now` evicts the stale pre-fix notify *and* duplicate commit-nows, converging to one `commitNowCommand` — exactly `migrateSessionClosedHook`' job, now intrinsic.

This yields the investigation's stated goal: one declarative code path, net code removal, and nothing that ever has to be removed later (no run-once migration cruft).

### One behavioral change to record

`migrateSessionClosedHook` evicts the stale notify via **exact-string** match (against the historical `notifyCommand` literal), chosen so it could never remove a user-customised hook like `portal state notify --debug`. The unified path uses **substring** match (`portal state notify`), consistent with how the teardown path (`UnregisterPortalHooks`) already identifies Portal entries.

Consequence: a hypothetical user-authored hook whose body merely *contains* `portal state notify` on a Portal-managed event would now be treated as Portal-owned and evicted. This is assessed as acceptable — these are Portal-internal subcommands users do not hand-author, and the change makes the register and teardown predicates identical (one definition of "Portal-owned," no drift). The spec adopts the substring predicate uniformly.

### What is intentionally *not* consolidated

The legacy `portal state migrate-rename` substring stays in the teardown path's `portalCommandSubstrings` (so `portal hooks reset` still reaps stale migrate-rename entries from very old binaries). Registration does not install or converge migrate-rename — it is not a current Portal hook category and is left exactly as-is.

## Teardown Rewrite — `UnregisterPortalHooks`

`UnregisterPortalHooks` (consumed by `portal hooks reset` and any other teardown caller) shares the **identical** global-enumeration blind spot today: it reads once via the no-arg `show-hooks -g`, so on the two blind events it sees zero Portal entries on the 139-deep arrays and removes nothing. `portal hooks reset` therefore cannot currently undo this bug. This was independently reproduced (3 stacked entries → global enumeration shows 0 → per-index `set-hook -gu 'pane-focus-out[N]'` does clear them).

**The teardown path moves to the same per-event seam.** For each event in `portalEvents`, read that event's entries via `ShowGlobalHooksForEvent(event)`, collect the Portal-authored entries (`portalEntriesFor` / `portalCommandSubstrings` — unchanged), and remove them via `UnsetGlobalHookAt` in descending index order.

What stays unchanged:

- The eviction predicate (`portalCommandSubstrings`, including the legacy `portal state migrate-rename` substring for old-binary cleanup).
- The set of events scanned (`portalEvents` = save-trigger ∪ hydration events).
- Reverse-index removal, per-removal best-effort with `errors.Join` aggregation, and the `show-hooks failed: %w` error wrap on a read failure.

Only the **read** changes — from one global enumeration to a per-event enumeration loop. After this change, `portal hooks reset` reaps Portal entries at any depth on every managed event, including the two blind ones.

This is the second half of "delete `ShowGlobalHooks`": once both registration and teardown are on the per-event seam, the no-arg global read has no remaining caller and is removed.

## Acceptance Criteria

1. **No growth across bootstraps.** Running bootstrap step 2 (hook registration) N times (N ≥ 2) on a real tmux server leaves every Portal-managed event's hook array at **exactly one** Portal entry — specifically `pane-focus-out` and `window-layout-changed` stay at 1 and never grow.
2. **Existing stacks self-collapse.** An event pre-seeded with K stacked identical Portal entries (e.g. 139) collapses to exactly one entry after a single registration — no dedicated cleanup invocation required.
3. **Stale bodies migrate in place.** A legacy body (un-separated `signal-hydrate`; pre-fix `notifyCommand` on `session-closed`) is converged to the current desired body, leaving exactly one entry on that event.
4. **User hooks survive.** A co-resident user-authored / other-plugin hook on a managed event — including on the two blind events — is untouched by both registration and teardown.
5. **Teardown reaps at depth.** `UnregisterPortalHooks` (`portal hooks reset`) removes all Portal entries at any depth on every managed event, including `pane-focus-out` and `window-layout-changed` (Portal entry count → 0).
6. **Global read removed.** The no-arg `ShowGlobalHooks` is deleted; no production caller remains. All hook reads go through `ShowGlobalHooksForEvent(event)`.
7. **Idempotent and churn-free.** A registration against an already-converged table performs no unset and no append — no hook renumbering, no log churn.
8. **Cascade eliminated.** After the fix, a single tmux event that triggers a managed hook (e.g. a session-switch firing `pane-focus-out`) spawns exactly one `portal state notify`, not N.

---

## Working Notes
