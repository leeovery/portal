# Plan: State Notify Cascade on Binary Upgrade

## Phases

### Phase 1: Per-Event Hook Convergence
status: approved
approved_at: 2026-06-03

**Goal**: Eliminate the unbounded duplicate-hook cascade by moving every Portal hook read off the global no-arg `show-hooks -g` (blind to `pane-*` and geometry/rename `window-*` events in tmux 3.6b) and onto a per-event `show-hooks -g <event>` seam. Registration is rebuilt as declarative "ensure exactly one" per managed event (folding in and deleting `migrateHydrationHooks` and `migrateSessionClosedHook`); teardown is rewritten to read per-event so it reaps at any depth; and the defective `ShowGlobalHooks` seam is deleted once no caller remains. Existing 139-deep stacks self-collapse to one entry on the next bootstrap as an ordinary side effect — no dedicated cleanup migration.

**Why this order**: This is a single-root-cause bug — the global-enumeration blind spot — fixed by one architectural shift applied uniformly. There is no foundation to build before the fix; the new per-event seam is the shared primitive both the registration and teardown paths consume, and the no-arg read can only be deleted after both have migrated off it. The two code surfaces are too coupled (shared seam, shared deletion goal, neither alone reaches the intended end state) to warrant separate phases. Real-tmux regression guards are co-resident first-class deliverables, since the defect is a tmux-output-shape issue invisible to mock commanders.

**Acceptance**:
- [ ] A new `ShowGlobalHooksForEvent(event)` client seam runs `show-hooks -g <event>`, preserving the removed `ShowGlobalHooks` contract — the same trimming `Run` path and `failed to show global hooks: %w` wrap shape — so `ParseShowHooks` needs zero changes.
- [ ] `RegisterPortalHooks` converges each managed event to exactly one Portal entry carrying the current desired body, reading per-event throughout: collect Portal-authored entries by the event's eviction fingerprint(s), fast-path no-op when exactly one entry already equals the desired body, else unset all matches in descending index order and append once.
- [ ] The per-event parameter table is honoured: notify events match `portal state notify` → `notifyCommand`; `session-closed` matches the union of `portal state notify` + `portal state commit-now` → `commitNowCommand`; hydration events match `portal state signal-hydrate` → the `--`-separated `signalHydrateCommand`.
- [ ] `migrateHydrationHooks` and `migrateSessionClosedHook` are deleted, their behaviour subsumed by the unified path; eviction uses the substring predicate uniformly (the documented behavioural change from session-closed's prior exact-match).
- [ ] Running hook registration N≥2 times against a real tmux server leaves every managed event's array at exactly one Portal entry — specifically `pane-focus-out` and `window-layout-changed` stay at 1 and never grow.
- [ ] An event pre-seeded with K stacked identical Portal entries collapses to exactly one entry after a single registration, with no dedicated cleanup invocation; a stale legacy body (un-separated `signal-hydrate`; pre-fix `notifyCommand` on `session-closed`) converges to the current desired body.
- [ ] A registration against an already-converged table performs no unset and no append — no hook renumbering, and no eviction log line is emitted (the absence is the asserted signal). When evictions do occur, a single INFO line under the `bootstrap` component records the total collapsed via the existing `reaped` attr.
- [ ] `UnregisterPortalHooks` reads per-event via `ShowGlobalHooksForEvent`, collecting Portal entries via the unchanged `portalEntriesFor` / `portalCommandSubstrings` (still including the legacy `portal state migrate-rename` substring), and removes them in descending index order — reaping all Portal entries at any depth on every managed event including the two blind ones (Portal count → 0).
- [ ] Per-event read failures are best-effort and folded into the `errors.Join` aggregate on both paths (loop never short-circuits; each failed read emits the canonical `show-hooks failed` WARN with `error_class=unexpected`); per-index unset failures emit a WARN and continue.
- [ ] A co-resident user-authored / other-plugin hook on any managed event — including `pane-focus-out` and `window-layout-changed` — is matched by neither registration nor teardown and survives untouched.
- [ ] The no-arg `ShowGlobalHooks` method is deleted; no production caller remains and any test fixtures referencing it are migrated to the new seam.
- [ ] Real-tmux (`internal/tmuxtest`) integration tests cover: no-growth across N bootstraps on the blind events; the tmux 3.6b blind-spot reality (no-arg omits pane/geometry events, per-event includes them); self-heal of a K-deep stack with a co-resident user hook left intact; teardown-at-depth on the blind events with user hook intact; and idempotency/no-churn. The full `go test ./...` suite is green with no regressions.

#### Tasks
status: approved
approved_at: 2026-06-03

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| state-notify-cascade-on-binary-upgrade-1-1 | Add ShowGlobalHooksForEvent per-event read seam | read failure wraps `failed to show global hooks: %w`, output byte-identical to global form, event with zero entries |
| state-notify-cascade-on-binary-upgrade-1-2 | Rebuild RegisterPortalHooks as per-event "ensure exactly one" | idempotent fast-path no-op, K-deep stack collapse, stale-body in-place migration, session-closed union fingerprint count, user/other-plugin hook untouched, per-event read failure folded into errors.Join, per-index unset failure WARN-and-continue, single reaped INFO only when evictions occur |
| state-notify-cascade-on-binary-upgrade-1-3 | Delete migrateHydrationHooks and migrateSessionClosedHook and their dedicated paths | hydration `--` convergence still holds, session-closed→commit-now convergence still holds, substring predicate is the documented behavioural change, mock-commander tests referencing deleted helpers migrated/removed |
| state-notify-cascade-on-binary-upgrade-1-4 | Move UnregisterPortalHooks to the per-event read seam | migrate-rename substring retained in teardown predicate only, per-event read failure folded into errors.Join (no all-or-nothing abort), user hook on managed event survives |
| state-notify-cascade-on-binary-upgrade-1-5 | Delete the no-arg ShowGlobalHooks method and migrate its remaining fixtures | no production caller remains, showGlobalHooksOrWarn / ShowGlobalHooksOrWarn re-export removed, reboot_roundtrip_test.go reader re-pointed at per-event seam, full suite green |
| state-notify-cascade-on-binary-upgrade-1-6 | Real-tmux no-growth + blind-spot regression guards | blind events (pane-focus-out, window-layout-changed) stay at 1 across N≥2, no-arg omits pane/geometry events while per-event includes them |
| state-notify-cascade-on-binary-upgrade-1-7 | Real-tmux self-heal, teardown-at-depth, and idempotency/no-churn guards | K-deep stack collapses to 1 with co-resident user hook intact, teardown reaps at depth on blind events with user hook intact, second registration emits no unset/append and no reaped INFO |
