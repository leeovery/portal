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

### Phase 2: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| state-notify-cascade-on-binary-upgrade-2-1 | Make managedEvents the single source of truth for the Portal-managed event set | full-derivation vs parity-test fallback, preserve declaration order for stable output, retire saveTriggerEvents only if last production consumer, migrate-rename teardown-only fingerprint divergence preserved, stale doc comment corrected either way |
| state-notify-cascade-on-binary-upgrade-2-2 | Collapse the eight hand-rolled per-event dispatch RunFuncs onto perEventDispatch with optional fault injection | readErrFor/unsetErrFor fault maps, preserve setHookErrFor + no-arg-global-read fatal guard, sibling builder if signature churn too large, each migrated test still exercises its original fault (read error / per-index unset / CommandError) |
| state-notify-cascade-on-binary-upgrade-2-3 | De-duplicate hook command-body and fingerprint test literals within the tmux_test package | single test-package home for notify body + fingerprint substrings, production unexported-constant mirroring left untouched (exempt), real-tmux integration tests pass against shared literals |
| state-notify-cascade-on-binary-upgrade-2-4 | Fold recordingMigrationLogger onto the pre-existing recordingSlogHandler base (additive to new code only) | embed/wrap shared base, thin component/reaped accessors via projection, portal_saver_test.go / recordingSlogHandler unchanged, consuming migration/warn tests observe identical captured output |
| state-notify-cascade-on-binary-upgrade-2-5 | Fix the stale migrateHydrationHooks comment in reboot_roundtrip_test.go | comment-only edit (no test logic change), describe per-event ensure-exactly-one convergence, grep-confirm last stale reference, cmd/bootstrap package passes |

### Phase 3: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| state-notify-cascade-on-binary-upgrade-3-1 | Extract a shared per-event eviction helper used by both convergeEvent and UnregisterPortalHooks | parseEventEntries single-home collapse, descending-index best-effort unset loop shared, distinct error contracts preserved (convergeEvent best-effort vs UnregisterPortalHooks errors.Join), UnregisterPortalHooks keeps exported func(*Client) error signature for cmd/state_cleanup.go, migrate-rename teardown-only fingerprint divergence preserved, ParseShowHooks byte-identical, no new log component/attr, optional injected-logger teardown variant closes the recording-seam gap |
| state-notify-cascade-on-binary-upgrade-3-2 | Consolidate the test-side read-per-event → ParseShowHooks → count-by-fingerprint helper | single tmux_test-package canonical helper, countSignalHydrateEntries collapses to map-builder, two migration inline loops routed through it, must read via ShowGlobalHooksForEvent (no-arg read is blind/vacuous), verifyHydrationHookEntries in different package left as-is |
| state-notify-cascade-on-binary-upgrade-3-3 | Reuse the existing set-hook argv extractors instead of inline mock.Calls scanning | argv guards (set-hook/-ga/-gu index literals) live only in accessor co-located with setHookCalls/unsetHookCalls, cross-verb ordering + event-name-prefix split before "[", K-deep-collapse and append-follows-unset assertions unchanged in meaning |
| state-notify-cascade-on-binary-upgrade-3-4 | Rename test functions that reference deleted migration helpers | rename 9 TestMigrateHydrationHooks_* + TestConvergeSessionClosed_* to behaviour-describing TestRegisterPortalHooks_* names, no name references migrateHydrationHooks/migrateSessionClosedHook/convergeSessionClosed, bodies/assertions/accurate doc comments unchanged, go test -run TestRegisterPortalHooks discovers renamed functions |

### Phase 4: Analysis (Cycle 3)

**Goal**: Address findings from Analysis (Cycle 3).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| state-notify-cascade-on-binary-upgrade-4-1 | Make teardown reap the converged session-closed commit-now hook (close the fingerprint seam, AC #5) | derive portalCommandSubstrings from union of managedEvents fingerprints + explicit legacy migrate-rename addend (drift-proof, literal-add fallback), session-closed commit-now fingerprint now reaped to zero at depth, registration unchanged (no migrate-rename in managedEvents, session-closed keeps {notify,commit-now} set), teardown retains migrate-rename for stale legacy bodies on session-renamed, real-tmux teardown-at-depth test seeds stacked commitNowCommand on session-closed with non-vacuous pre-condition count + co-resident user hook survives, fingerprint-parity guard test mirroring TestPortalManagedEventSetParity |
| state-notify-cascade-on-binary-upgrade-4-2 | Consolidate the forked register/teardown test-harness dispatch + line-scoping helpers and propagate the no-arg-global-read fatal guard to teardown | teardown dispatch routed through perEventDispatchWithFaults (or shared extracted skeleton) so the no-arg-global-read t.Fatalf tripwire now covers teardown, single line-scoping primitive (parseSeededTableByEvent owner, linesForEvent thin lookup or removed) recognising both register `<event>[i] => '...'` and unregister `<event>[i] run-shell '...'` shapes via `<event>[` prefix, per-index unset fault injection preserved through unified unsetErrFor, simulated no-arg regression fails loudly, no t.Parallel introduced |
| state-notify-cascade-on-binary-upgrade-4-3 | Fix the stale doc comment in the six-event routing test (deleted test name + prior-spec heading) | comment-only edit (no test logic/sub-test/assertion change), file-level comment references TestRegisterPortalHooks_FreshTable not deleted TestRegisterPortalHooks_SessionClosedMigration, re-anchor spec-section pointer in both file-level comment and in-body t.Errorf (lines 99-104) to "Registration Redesign — Ensure Exactly One" (confirm exact heading against spec), go test ./internal/tmux/... still green |

### Phase 5: Analysis (Cycle 4)

**Goal**: Address findings from Analysis (Cycle 4).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| state-notify-cascade-on-binary-upgrade-5-1 | Route the three teardown tests' inline dispatch RunFuncs through perEventDispatchWithFaults | two fail-every-read sites use readErrForAllManagedEvents(sentinel) read-fault channel, single-event fold site uses map{"pane-focus-out": sentinel} with existing raw seeded table, all three inherit shared no-arg-global-read t.Fatalf tripwire, prune now-unused linesForEvent/imports without breaking other references, do NOT widen dispatchUnregisterHooks with a readErrFor param (direct perEventDispatchWithFaults call), test-only (no internal/tmux production change), behaviour-preserving assertions, no t.Parallel introduced |
