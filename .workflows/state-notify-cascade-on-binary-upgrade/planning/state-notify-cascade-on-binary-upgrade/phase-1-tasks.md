---
phase: 1
phase_name: Per-Event Hook Convergence
total: 7
---

## state-notify-cascade-on-binary-upgrade-1-1 | approved

### Task 1-1: Add ShowGlobalHooksForEvent per-event read seam

**Problem**: Portal's idempotency and teardown checks read the global hook table via `show-hooks -g` (no event argument). tmux 3.6b's no-arg `show-hooks -g` does not enumerate an entire class of events (`pane-*` and the geometry/rename `window-*` events: `window-layout-changed`, `window-pane-changed`, `window-renamed`, `window-resized`), even though those hooks are set and fire normally. The fix requires a per-event read that is not blind, but no such client method exists yet.

**Solution**: Add a new `*tmux.Client` method `ShowGlobalHooksForEvent(event string) (string, error)` that runs `show-hooks -g <event>`. It preserves the exact contract of the existing no-arg `ShowGlobalHooks`: same trimming `Run` path and the same `failed to show global hooks: %w` error wrap. Output is byte-identical in shape to the global form (`pane-focus-out[0] run-shell "…"`), so `ParseShowHooks` needs zero changes.

**Outcome**: A new client seam exists that, given an event name, returns the raw `show-hooks -g <event>` output verbatim through the trimming `Run` path, wrapping any commander error with `failed to show global hooks: %w`. The existing `ShowGlobalHooks` method is left in place for now (it is deleted in Task 1-5 once no caller remains).

**Do**:
- In `internal/tmux/tmux.go`, add a new method immediately after the existing `ShowGlobalHooks` (~L769):
  ```go
  // ShowGlobalHooksForEvent returns the raw output of "tmux show-hooks -g <event>".
  // Unlike the no-arg ShowGlobalHooks, the per-event form is not blind to the
  // pane-scoped / geometry-rename window-scoped events that tmux 3.6b omits from
  // global enumeration. The output shape is byte-identical to the global form
  // (e.g. `pane-focus-out[0] run-shell "…"`), so ParseShowHooks needs no change.
  func (c *Client) ShowGlobalHooksForEvent(event string) (string, error) {
      output, err := c.cmd.Run("show-hooks", "-g", event)
      if err != nil {
          return "", fmt.Errorf("failed to show global hooks: %w", err)
      }
      return output, nil
  }
  ```
- Use `c.cmd.Run` (the trimming variant), matching the removed method's contract — the spec notes trim-vs-no-trim is immaterial because `ParseShowHooks` trims each line itself.
- Do NOT delete `ShowGlobalHooks` in this task — Tasks 1-2 and 1-4 must migrate their callers off it first; deletion is Task 1-5.

**Acceptance Criteria**:
- [ ] `ShowGlobalHooksForEvent(event)` invokes the commander with exactly `["show-hooks", "-g", <event>]`.
- [ ] On commander success it returns the output verbatim with a nil error.
- [ ] On commander failure it returns `("", err)` where `err` wraps the underlying error with the literal prefix `failed to show global hooks: ` (assertable via `errors.Is` on the sentinel and substring containment).
- [ ] An event with zero entries (commander returns empty string, nil error) yields `("", nil)` — not an error.
- [ ] `go build -o portal .` and `go test ./internal/tmux/...` pass with the method added.

**Tests** (add to `internal/tmux/hooks_test.go`, a new `TestShowGlobalHooksForEvent` mirroring the structure of the existing `TestShowGlobalHooks`):
- `"it calls show-hooks -g <event> with the event as argv[2] and returns raw output"` — assert the recorded `mock.Calls[0]` equals `["show-hooks", "-g", "pane-focus-out"]` and output is returned verbatim (include leading/trailing whitespace to confirm no extra trimming at this layer beyond the commander's own).
- `"it returns empty string without error when output is empty"` — event with zero entries.
- `"it propagates commander error wrapped via %w with the failed to show global hooks prefix"` — `MockCommander{Err: sentinel}`, assert `errors.Is(err, sentinel)` and `strings.Contains(err.Error(), "failed to show global hooks")`.

**Edge Cases**:
- Read failure must wrap `failed to show global hooks: %w` (identical to the removed `ShowGlobalHooks` so downstream `show-hooks failed: %w` re-wrapping in the register/teardown paths is unchanged).
- Output byte-identical to the global form — no normalization is performed; the per-event read just scopes the enumeration.
- Event with zero entries returns `("", nil)`, which `ParseShowHooks` already handles (empty input → non-nil empty map).

**Context**:
> Spec § Concrete mechanism: "New tmux client seam: `ShowGlobalHooksForEvent(event)` → runs `show-hooks -g <event>`. Output format is byte-identical to the global form … so the existing `ParseShowHooks` parser needs zero changes." And: "`ShowGlobalHooksForEvent` preserves the removed method's contract — the same trimming `Run` path and the `failed to show global hooks: %w` error-wrap shape." The `Commander` interface exposes `Run` (trim) and `RunRaw` (verbatim); the seam uses `Run`, matching `ShowGlobalHooks`.

**Spec Reference**: `.workflows/state-notify-cascade-on-binary-upgrade/specification/state-notify-cascade-on-binary-upgrade/specification.md` §§ Solution Strategy, Concrete mechanism.

## state-notify-cascade-on-binary-upgrade-1-2 | approved

### Task 1-2: Rebuild RegisterPortalHooks as per-event "ensure exactly one"

**Problem**: `RegisterPortalHooks` currently dedupes via `RegisterHookIfAbsent`, which reads the global no-arg `show-hooks -g` table to decide whether a hook is "absent." On the two blind events (`pane-focus-out`, `window-layout-changed`) tmux 3.6b omits the existing entry from that global read, so the check always concludes "absent" and appends another copy — unbounded growth (live: 139 stacked copies). Every stacked copy fires, detonating N `portal state notify` processes per single tmux event.

**Solution**: Rebuild `RegisterPortalHooks` so that, for every Portal-managed event, it reads that event's entries via the new `ShowGlobalHooksForEvent(event)` seam and converges the event's hook array to exactly one Portal entry carrying the current desired body. Convergence: collect the Portal-authored entries for the event's eviction fingerprint(s); if exactly one already equals the desired body, do nothing (idempotent fast path); otherwise unset every matching entry in descending index order and append the desired body once. This collapses any depth-N stack to one entry and migrates stale legacy bodies in place, as an ordinary side effect of bootstrap step 2.

**Outcome**: After `RegisterPortalHooks` runs, every managed event holds exactly one Portal entry with the current desired body; running it again is a complete no-op (no unset, no append). The defective `RegisterHookIfAbsent` append-if-absent path is gone, replaced by the per-event ensure-exactly-one path. The `migrateHydrationHooks` / `migrateSessionClosedHook` helpers are folded in by this rewrite but physically removed in Task 1-3.

**Do**:
- In `internal/tmux/hooks_register.go`, build a per-event convergence engine. Define a managed-event table that pairs each event with its eviction fingerprint(s) and desired body, per the spec's parameter table:
  | Event(s) | Eviction fingerprint(s) | Desired body |
  |---|---|---|
  | `session-created`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out` | `portal state notify` | `notifyCommand` |
  | `session-closed` | `portal state notify`, `portal state commit-now` | `commitNowCommand` |
  | `client-attached`, `client-session-changed` | `portal state signal-hydrate` | `signalHydrateCommand` |
  Retain `saveTriggerEvents`, `HydrationTriggerEvents`, and the body constants `notifyCommand` / `commitNowCommand` / `signalHydrateCommand` (the body constants are already wrapped, guard-prefixed `run-shell` strings — do not change them). The `session-closed` event must be expressed with a two-element fingerprint set; the notify events and hydration events each carry a single fingerprint.
- Implement the per-event convergence algorithm (one helper, e.g. `convergeEvent(c *Client, logger *slog.Logger, event string, fingerprints []string, desiredBody string) (evicted int, err error)`):
  1. `raw, err := c.ShowGlobalHooksForEvent(event)`. On error: emit the canonical `show-hooks failed` WARN (`error`, `error_class="unexpected"`) and return `(0, fmt.Errorf("show-hooks failed: %w", err))`. (Reuse the existing WARN+wrap helper shape; the wrap string and WARN attrs must stay byte-identical to today's `showGlobalHooksOrWarn`.)
  2. Parse with `ParseShowHooks(raw)[event]`. Collect Portal-authored entries: an entry is Portal-authored iff `containsAny(entry.Command, fingerprints)` (the union across this event's fingerprints; see `containsAny` in `hooks_unregister.go`).
  3. **Idempotent fast path**: if exactly one Portal-authored entry exists AND its `Command` equals `desiredBody` (byte-for-byte string compare against the desired-body constant, post `ParseShowHooks` quote-stripping), return `(0, nil)` — no unset, no append.
  4. Otherwise converge: collect the Portal-authored entries' indices, sort descending, and `UnsetGlobalHookAt(event, idx)` each. A per-index unset failure is best-effort — emit a WARN carrying the underlying `error` and continue (do not abort, do not count it as evicted). Then `AppendGlobalHook(event, desiredBody)` exactly once; an append failure is returned (wrapped) so it folds into the aggregate. Return the count of successfully-unset entries.
- Rewrite `RegisterPortalHooks(c *Client, logger *slog.Logger) error` to iterate the managed-event table, calling `convergeEvent` for each event, accumulating per-event errors into an `errors.Join` aggregate (loop never short-circuits — every event is attempted) and summing the evicted counts.
  - After the loop, if the total evicted count > 0, emit a **single INFO** under the `bootstrap` component using the existing `reaped` attr: `logger.Info("<eviction summary message>", "reaped", total)`. If total == 0 (including the all-fast-path case), emit **no** eviction line. Per-event eviction detail may be emitted at DEBUG.
  - Keep the package-level `bootstrapLogger = log.For("bootstrap")` available; `RegisterPortalHooks` receives an injected `*slog.Logger` and tolerates nil via `log.OrDiscard`.
- Delete `RegisterHookIfAbsent`, `hookCategory`, `portalHookCategories`, `notifySubstring`, and `signalHydrateSubstring` if and only if they have no remaining callers after the rewrite — but note `migrateHydrationHooks` / `migrateSessionClosedHook` are still present until Task 1-3, so coordinate: this task may leave those two helpers physically present-but-unreferenced if removing them now would explode the diff, OR remove their call sites from `RegisterPortalHooks` and let Task 1-3 delete the dead functions. Prefer removing their call sites here (they are no longer invoked) and deleting the dead function bodies in Task 1-3. The build must stay green at the end of this task — Go does not error on an unreferenced unexported function, so leaving the two helper bodies temporarily is acceptable, but their `_test.go` fixtures that drive them through `RegisterPortalHooks` will change behaviour (see Tests).
- Migrate the mock-commander fixtures in `internal/tmux/hooks_register_test.go`, `hooks_register_six_event_routing_test.go`, and `hooks_register_warn_test.go` so their dispatch helpers answer the **per-event** read shape. Today `dispatchPortalHooks` / `dispatchShowHooks` match `len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g"` and return the WHOLE table for every call. After the change the production code calls `show-hooks -g <event>` (argv[2] = event), so the helpers must:
  - Recognize the per-event form (`args[0] == "show-hooks" && args[1] == "-g"` with `len(args) >= 3`) and return **only the lines for `args[2]`** (filter the seeded table by event), so the convergence engine sees the correct per-event entry count.
  - Continue to accept `set-hook -ga` (append) and `set-hook -gu` (unset) as before.

**Acceptance Criteria**:
- [ ] On a fresh hook table, `RegisterPortalHooks` appends exactly one entry per managed event with the correct desired body: `notifyCommand` on the six notify events, `commitNowCommand` on `session-closed`, `signalHydrateCommand` on the two hydration events. No `portal state migrate-rename` command is ever appended.
- [ ] **Idempotent fast path**: against a table already holding exactly one converged entry per event, `RegisterPortalHooks` issues zero `set-hook -ga` and zero `set-hook -gu` calls.
- [ ] **K-deep stack collapse**: with K identical Portal entries seeded on one event, convergence issues K `set-hook -gu` calls (descending index order) then one `set-hook -ga`, leaving exactly one entry.
- [ ] **Stale-body in-place migration**: a legacy un-separated `signal-hydrate` body on a hydration event, and a pre-fix `notifyCommand` on `session-closed`, are each evicted and replaced by the current desired body (count → 1).
- [ ] **session-closed union fingerprint**: an already-converged `session-closed` holding one `commitNowCommand` entry (which matches the `portal state commit-now` fingerprint, union count 1, body equals desired) takes the fast path — no unset, no append.
- [ ] **User/other-plugin hook untouched**: a co-resident entry whose body matches none of the event's fingerprints survives every registration (not unset, not counted).
- [ ] **Per-event read failure folded**: a `ShowGlobalHooksForEvent` failure on one event emits the `show-hooks failed` WARN (`error_class=unexpected`), folds into the `errors.Join` aggregate, and does NOT prevent the other events from converging.
- [ ] **Per-index unset failure**: a single `UnsetGlobalHookAt` failure emits a WARN, the loop continues to the next index, and the append still fires.
- [ ] **Single reaped INFO only on eviction**: a registration that evicts ≥1 entry emits exactly one INFO under `bootstrap` carrying the total via the `reaped` attr; a zero-eviction registration (including the all-fast-path case) emits no eviction INFO line.
- [ ] `go build -o portal .` passes; `go test ./internal/tmux/...` passes with migrated fixtures.

**Tests** (mock-commander unit tests in `internal/tmux/hooks_register_test.go`; the faithful real-tmux oracle lands in Tasks 1-6/1-7):
- `"it registers exactly one entry per managed event on a fresh table with the correct desired body"`
- `"it is a complete no-op against an already-converged table (zero unset, zero append)"`
- `"it collapses a K-deep stack on a single event to one entry via descending-index unsets then one append"`
- `"it migrates a stale un-separated signal-hydrate body in place to the -- form (count stays 1)"`
- `"it migrates a stale pre-fix notifyCommand on session-closed to commitNowCommand (count stays 1)"`
- `"it takes the fast path for an already-converged session-closed holding one commitNowCommand (union count 1)"`
- `"it leaves a co-resident user hook (no fingerprint match) untouched on a managed event"`
- `"it folds a per-event show-hooks failure into the errors.Join aggregate and still converges other events"` (fail the read for one event only)
- `"it emits a WARN and continues when one UnsetGlobalHookAt fails, then still appends"`
- `"it emits exactly one reaped INFO under the bootstrap component when evictions occur"` (use the existing recording-logger seam)
- `"it emits no eviction INFO line on a zero-eviction registration"`

**Edge Cases**:
- Idempotent fast-path no-op — exactly-one entry already equal to the desired body.
- K-deep stack collapse — descending index order so a removal never shifts an unprocessed index.
- Stale-body in-place migration — body differs from desired → converge (unset all + append one).
- session-closed union fingerprint count — Portal-authored counted across the union of `portal state notify` + `portal state commit-now`, not per-fingerprint, so a single `commitNowCommand` yields union count 1.
- User/other-plugin hook untouched — fingerprint match is the only eviction predicate; non-matching bodies survive.
- Per-event read failure folded into `errors.Join` — loop never short-circuits.
- Per-index unset failure → WARN and continue; the append still fires.
- Single `reaped` INFO only when evictions occur — its absence is the asserted signal for the idempotent case.

**Context**:
> Spec § Registration Redesign — "Ensure Exactly One": the four-step per-event convergence algorithm. § Per-event parameters table. § "Hook body shapes": each desired body is a `run-shell`-wrapped, guard-prefixed command; the fast-path equality check compares the **full wrapped body** byte-for-byte against the desired-body constant (the hydration constant contains the literal unexpanded `#{session_name}` token — tmux stores bodies verbatim and expands `#{…}` only at fire time, so the stored body equals the constant modulo `ParseShowHooks`' outer-quote stripping). § Logging, ordering & failure semantics: single INFO with the existing `reaped` attr under `bootstrap` on eviction; no line on zero-eviction; per-index unset failure is WARN-and-continue; per-event read failure is folded into `errors.Join` (canonical `show-hooks failed` WARN, `error_class=unexpected`); event/category order is no longer significant for correctness but a deterministic order may be retained for stable log/test output. § User-hook coexistence guarantee: eviction matches only Portal-authored bodies. Closed log taxonomy: use only the existing `reaped` attr and `bootstrap` component — invent no new attr/component.

**Spec Reference**: `.workflows/state-notify-cascade-on-binary-upgrade/specification/state-notify-cascade-on-binary-upgrade/specification.md` §§ Registration Redesign — "Ensure Exactly One", Per-event parameters, Hook body shapes, User-hook coexistence guarantee, Logging/ordering/failure semantics.

## state-notify-cascade-on-binary-upgrade-1-3 | approved

### Task 1-3: Delete migrateHydrationHooks and migrateSessionClosedHook and their dedicated paths

**Problem**: The codebase carries three registration shapes — `RegisterHookIfAbsent` (the defective append-if-absent path, removed in Task 1-2) plus two one-shot migration helpers, `migrateHydrationHooks` (evicts legacy un-separated `signal-hydrate` bodies) and `migrateSessionClosedHook` (exact-match evicts stale `notifyCommand` on `session-closed`, then appends `commitNowCommand`). The unified per-event ensure-exactly-one path from Task 1-2 already subsumes both. Leaving the two helpers in place is permanent cruft that can never be safely removed and re-introduces the global-read blind spot through their `showGlobalHooksOrWarn` calls.

**Solution**: Delete `migrateHydrationHooks`, `migrateSessionClosedHook`, and every identifier that existed only to support them, confirming their behaviour is fully covered by the Task 1-2 convergence path. Migrate or remove the mock-commander test fixtures that drove the deleted helpers through `RegisterPortalHooks`, re-expressing their assertions against the unified path (which evicts via **substring** match, the documented behavioural change from session-closed's prior exact-match).

**Outcome**: `hooks_register.go` contains a single declarative registration path with no migration-helper functions. The hydration `--` convergence and the `session-closed → commit-now` convergence both still hold under the unified path, verified by migrated tests. Net code removal; nothing remains that must be deleted later.

**Do**:
- In `internal/tmux/hooks_register.go`, delete:
  - `migrateHydrationHooks` (and its supporting `isStaleSignalHydrateEntry`, `staleSignalHydratePrefix`, `staleSignalHydrateMarker`).
  - `migrateSessionClosedHook` and the `sessionClosedEvent` constant if it is no longer referenced (the unified table may still want a `session-closed` literal — keep one source of truth; if the managed-event table from Task 1-2 references the event name, retain a single constant and drop the duplicate).
  - Any remaining `RegisterHookIfAbsent` / `hookCategory` / `portalHookCategories` / `notifySubstring` / `signalHydrateSubstring` declarations not already removed in Task 1-2.
  - The now-orphaned `showGlobalHooksOrWarn` helper IF Task 1-2's convergence path inlined its own WARN+wrap. If Task 1-2 reused `showGlobalHooksOrWarn`, retain it but switch its body to call `ShowGlobalHooksForEvent` — note it currently calls `ShowGlobalHooks`; that no-arg method is deleted in Task 1-5, so by the end of this task no production code path may call `c.ShowGlobalHooks()`. Verify with `grep "ShowGlobalHooks()" internal/tmux/*.go` (non-test): the only remaining production caller after this task must be in `hooks_unregister.go`, which Task 1-4 migrates.
- Update the `internal/tmux/hooks_migration_test.go` file (the dedicated `migrateHydrationHooks` suite). These tests assert helper-specific behaviour (eviction INFO message phrasing "evicted stale signal-hydrate hooks", per-helper structure). Re-express the still-valid invariants against the unified path or delete tests that only pinned the now-deleted helper's internal shape:
  - Keep the real-tmux behavioural invariants (stale `--` entry evicted, exactly one fixed entry per hydration event post-bootstrap, idempotent second bootstrap, user-hook-not-Portal-shape preserved) but adapt them: the unified eviction INFO message and the `reaped` attr now come from the single convergence summary, not a hydration-specific line. Assertions on the literal message "evicted stale signal-hydrate" must be retargeted to the unified summary message chosen in Task 1-2 (or relaxed to assert the `reaped` count only).
  - The `recordingLogger` capture helper in this file may be retained or consolidated with `recordingMigrationLogger` in `hooks_register_test.go` — do not duplicate; if both remain, leave a comment noting they capture the same shape.
  - Delete tests that assert the deleted helper is invoked as a distinct unit (e.g. `TestMigrateHydrationHooks_HydrationTriggerEventsSliceIsRespectedAtRuntime` asserting `set-hook -gu` covers every hydration event via the helper's per-event loop) only if the equivalent coverage now lives in the Task 1-2 convergence tests or the Task 1-6/1-7 real-tmux tests; otherwise re-express against the unified path.
- Update `internal/tmux/hooks_register_test.go`'s `TestRegisterPortalHooks_SessionClosedMigration` sub-tests: the unified path uses **substring** match (`portal state notify` + `portal state commit-now`) instead of session-closed's prior exact-string match. The sub-test `"it preserves a user-customised hook on session-closed that does not exact-match the Portal literals"` used `run-shell "portal state notify --debug"` and expected it preserved — under the substring predicate this body NOW contains `portal state notify` and IS treated as Portal-owned and evicted. Update that test to reflect the documented behavioural change: either (a) change the user hook to a body that does not contain any Portal fingerprint (so it survives), and add a separate explicit test documenting that a body containing `portal state notify` IS now evicted, OR (b) re-assert the eviction as the intended new behaviour. Prefer (a) plus a documenting test, matching the spec's framing.
- Update the mock dispatch helpers so the per-event reads return per-event-filtered output (this overlaps with Task 1-2; ensure consistency — there is one source of truth for the dispatch helper after both tasks).
- Run `go build -o portal .` and `go test ./internal/tmux/...` to confirm green after deletions and fixture migration.

**Acceptance Criteria**:
- [ ] `migrateHydrationHooks` and `migrateSessionClosedHook` no longer exist in `internal/tmux/hooks_register.go` (grep returns no definition).
- [ ] Hydration `--` convergence still holds: a stale un-separated `signal-hydrate` body converges to the `--` form, exactly one entry per hydration event (verified by a migrated test).
- [ ] session-closed → commit-now convergence still holds: a stale `notifyCommand` on `session-closed` converges to one `commitNowCommand` (verified by a migrated test).
- [ ] The substring predicate is exercised and documented as the behavioural change: a body containing `portal state notify` on a managed event is now treated as Portal-owned (evicted), and a test documents this.
- [ ] No production code path calls `c.ShowGlobalHooks()` except (temporarily) `hooks_unregister.go`, which Task 1-4 migrates.
- [ ] `go build -o portal .` and `go test ./internal/tmux/...` pass.

**Tests** (migrated / updated, in `internal/tmux/hooks_migration_test.go` and `hooks_register_test.go`):
- `"it converges a stale un-separated signal-hydrate body to the -- form with exactly one entry per hydration event"` (migrated from `TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed`)
- `"it is idempotent on a second bootstrap after hydration convergence (no eviction line)"` (migrated from `TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap`)
- `"it converges a stale notifyCommand on session-closed to commitNowCommand"` (migrated from the session-closed migration suite)
- `"it now evicts a body containing portal state notify on session-closed under the substring predicate"` (NEW — documents the behavioural change from exact-match)
- `"it leaves a body that matches no Portal fingerprint untouched on session-closed"` (replaces the old `--debug` preservation test with a genuinely non-matching body)

**Edge Cases**:
- Hydration `--` convergence still holds after the helper is folded in.
- session-closed → commit-now convergence still holds after the helper is folded in.
- Substring predicate is the documented behavioural change (a body merely containing `portal state notify` is now Portal-owned and evicted, where exact-match previously spared `portal state notify --debug`).
- Mock-commander tests referencing the deleted helpers are migrated or removed; no test drives a function that no longer exists.

**Context**:
> Spec § Migration-Helper Consolidation: "fold all three into the single per-event ensure-exactly-one path; delete `migrateHydrationHooks` and `migrateSessionClosedHook`." The ensure-exactly-one algorithm already does everything the two helpers did — hydration matching on `portal state signal-hydrate` evicts the legacy un-separated body and converges to the `--` form; session-closed matching on `portal state notify` + `portal state commit-now` evicts the stale pre-fix notify and converges to one `commitNowCommand`. § "One behavioral change to record": `migrateSessionClosedHook` used exact-string match; the unified path uses substring match (consistent with teardown). Consequence: a hypothetical user hook whose body merely contains `portal state notify` on a managed event is now treated as Portal-owned and evicted — assessed acceptable because these are Portal-internal subcommands users do not hand-author, and it makes register and teardown predicates identical. § "What is intentionally not consolidated": `portal state migrate-rename` stays in the teardown predicate only; registration must NOT install or converge migrate-rename (the registration fingerprint set and teardown `portalCommandSubstrings` set are intentionally different — do not unify them).

**Spec Reference**: `.workflows/state-notify-cascade-on-binary-upgrade/specification/state-notify-cascade-on-binary-upgrade/specification.md` §§ Migration-Helper Consolidation, One behavioral change to record, What is intentionally not consolidated.

## state-notify-cascade-on-binary-upgrade-1-4 | approved

### Task 1-4: Move UnregisterPortalHooks to the per-event read seam

**Problem**: `UnregisterPortalHooks` (consumed by `portal hooks reset` and any other teardown caller) reads once via the no-arg `show-hooks -g`. On the two blind events it sees zero Portal entries on the 139-deep arrays and removes nothing — so `portal hooks reset` currently cannot undo the bug. It shares the identical global-enumeration blind spot as registration.

**Solution**: Move the teardown read off the no-arg global enumeration onto the per-event `ShowGlobalHooksForEvent(event)` seam. For each event in `portalEvents`, read that event's entries, collect Portal-authored entries via the unchanged `portalEntriesFor` / `portalCommandSubstrings` (still including the legacy `portal state migrate-rename` substring), and remove them via `UnsetGlobalHookAt` in descending index order. The failure contract changes from single-read all-or-nothing to per-event fold-and-continue, matching the new register contract.

**Outcome**: `UnregisterPortalHooks` reaps all Portal entries at any depth on every managed event — including the two blind ones — leaving any co-resident user hook intact. A per-event read failure folds into the `errors.Join` aggregate rather than aborting the whole teardown. This is the second half of "delete `ShowGlobalHooks`": after this task, the no-arg global read has no production caller (Task 1-5 removes the method).

**Do**:
- In `internal/tmux/hooks_unregister.go`, rewrite `UnregisterPortalHooks(c *Client) error`:
  - Replace the single `raw, err := c.ShowGlobalHooks()` + one-shot abort with a per-event loop over `portalEvents`. For each event: `raw, err := c.ShowGlobalHooksForEvent(event)`; on error, emit the canonical `show-hooks failed` WARN (`error`, `error_class="unexpected"`) and fold `fmt.Errorf("show-hooks failed: %w", err)` (named with the event) into the `errs` aggregate, then `continue` to the next event — do NOT abort the loop.
  - For a successful read: `portal := portalEntriesFor(ParseShowHooks(raw)[event])`, sort descending by `Index`, and `UnsetGlobalHookAt(event, entry.Index)` each, folding per-removal failures into `errs` (unchanged leaf shape `unset hook on %s[%d]: %w`).
  - Return `errors.Join(errs...)` when non-empty, else nil.
  - `UnregisterPortalHooks` currently takes no logger. The teardown path needs a WARN sink for the per-event read failure. Either thread a logger param through (and update the `portal hooks reset` call site to pass `log.For("bootstrap")` or the appropriate component) OR bind a package-level logger (mirror `bootstrapLogger = log.For("bootstrap")` already in `hooks_register.go`). Prefer the package-level binding to avoid changing the exported signature and its callers — confirm the WARN routes under the same component the spec names (`bootstrap`); the spec says each failed read emits the canonical `show-hooks failed` WARN with `error_class=unexpected`.
- Leave `portalCommandSubstrings`, `portalEvents`, `portalEntriesFor`, and `containsAny` UNCHANGED. In particular, keep `portal state migrate-rename` in `portalCommandSubstrings` (teardown still reaps legacy migrate-rename entries from old binaries) — this is the intentional divergence from the registration fingerprint set.
- Migrate the mock-commander fixtures in `internal/tmux/hooks_unregister_test.go`. The dispatch helper `dispatchUnregisterHooks` matches `len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g"` and returns the WHOLE seeded table for every call. After the change the production code issues `show-hooks -g <event>` per event (argv[2] = event), so the helper must return **only the lines for `args[2]`**. Without this, every per-event read returns the full table and the per-event removal logic would attempt to unset entries on the wrong event. Update the helper to filter by event; the existing assertions (reverse index order, user entries left in place, migrate-rename reaped on session-renamed, cross-event ordering following `portalEvents`) should then hold against the per-event reads.
  - Update the `"propagates show-hooks -g failure without issuing any removal"` test: the new contract is fold-and-continue, not all-or-nothing abort. If ALL per-event reads fail, the aggregate error is returned and zero removals happen — re-express the test to fail every per-event read and assert the joined error wraps the sentinel and contains `show-hooks failed`, with zero `set-hook -gu` calls. Add a NEW test where ONE event's read fails and the others succeed, asserting the failing event folds into the aggregate while the other events' Portal entries are still reaped (the all-or-nothing behaviour is gone).
- Run `go build -o portal .` and `go test ./internal/tmux/...` to confirm green.

**Acceptance Criteria**:
- [ ] `UnregisterPortalHooks` reads per-event via `ShowGlobalHooksForEvent(event)` for every event in `portalEvents` — no call to `c.ShowGlobalHooks()` remains.
- [ ] Portal entries at any depth on every managed event (including `pane-focus-out` and `window-layout-changed`) are removed in descending index order; Portal count → 0.
- [ ] A co-resident user / other-plugin entry (no `portalCommandSubstrings` match) on a managed event survives.
- [ ] `portal state migrate-rename` remains in `portalCommandSubstrings` and a stale migrate-rename entry on `session-renamed` is still reaped.
- [ ] A per-event read failure folds into the `errors.Join` aggregate (the loop continues; other events are still torn down) and emits the canonical `show-hooks failed` WARN with `error_class=unexpected` — no all-or-nothing abort.
- [ ] Per-removal `UnsetGlobalHookAt` failures fold into the aggregate with the leaf shape `unset hook on <event>[<index>]: %w`, every removal still attempted.
- [ ] `go build -o portal .` and `go test ./internal/tmux/...` pass with migrated fixtures.

**Tests** (mock-commander unit tests in `internal/tmux/hooks_unregister_test.go`; real-tmux teardown-at-depth oracle is Task 1-7):
- `"it removes a single Portal entry from an otherwise-empty event array"` (migrated to per-event reads)
- `"it removes interleaved Portal entries in reverse index order and leaves user entries in place"` (migrated)
- `"it removes both Portal entries on session-renamed (notify and migrate-rename)"` (migrated — proves migrate-rename retained in teardown predicate)
- `"it ignores matching substrings on events outside portalEvents"` (migrated)
- `"it folds a single-event read failure into the aggregate and still reaps Portal entries on other events"` (NEW — the fold-and-continue contract; one event's read fails, others succeed)
- `"it returns the aggregate error and removes nothing when every per-event read fails"` (replaces the old all-or-nothing abort test)
- `"it attempts every removal even when one set-hook -gu call fails and returns a joined error naming every failed index"` (migrated)
- `"it is idempotent: a second run after a successful removal does nothing"` (migrated)

**Edge Cases**:
- migrate-rename substring retained in the teardown predicate only (registration omits it — see Task 1-3).
- Per-event read failure folded into `errors.Join` — no all-or-nothing abort; other events still torn down.
- User hook on a managed event survives (no `portalCommandSubstrings` match).
- Descending index order so a removal never shifts an unprocessed index.

**Context**:
> Spec § Teardown Rewrite — `UnregisterPortalHooks`: "The teardown path moves to the same per-event seam. For each event in `portalEvents`, read that event's entries via `ShowGlobalHooksForEvent(event)`, collect the Portal-authored entries (`portalEntriesFor` / `portalCommandSubstrings` — unchanged), and remove them via `UnsetGlobalHookAt` in descending index order." What stays unchanged: the eviction predicate (including the legacy `portal state migrate-rename` substring), the set of events scanned (`portalEvents`), reverse-index removal, per-removal best-effort with `errors.Join`, and the `show-hooks failed: %w` wrap. § Logging/failure semantics: "The teardown path follows the same fold-and-continue contract — a mid-loop failure leaves already-processed events torn down and folds the error rather than aborting. This is a deliberate change from today's single-read all-or-nothing abort." "Only the read changes — from one global enumeration to a per-event enumeration loop." This is the second half of "delete `ShowGlobalHooks`."

**Spec Reference**: `.workflows/state-notify-cascade-on-binary-upgrade/specification/state-notify-cascade-on-binary-upgrade/specification.md` §§ Teardown Rewrite — `UnregisterPortalHooks`, Logging/ordering/failure semantics (Per-event read failure).

## state-notify-cascade-on-binary-upgrade-1-5 | approved

### Task 1-5: Delete the no-arg ShowGlobalHooks method and migrate its remaining fixtures

**Problem**: The no-arg `Client.ShowGlobalHooks` (`show-hooks -g`) is the defect's single point of entry — it is blind to the pane/geometry events in tmux 3.6b. With registration (Task 1-2/1-3) and teardown (Task 1-4) both moved to the per-event seam, no production caller remains, but the method and its test fixtures still reference it. Leaving it invites a future caller to re-introduce the blind spot.

**Solution**: Delete the `ShowGlobalHooks` method from `internal/tmux/tmux.go`, remove the `ShowGlobalHooksOrWarn` test re-export (and the underlying `showGlobalHooksOrWarn` helper if it still calls the no-arg form — repoint it at `ShowGlobalHooksForEvent` if still used, or delete it), and migrate every remaining test fixture that reads via the no-arg method onto `ShowGlobalHooksForEvent`. The dedicated `TestShowGlobalHooks` unit test is replaced by the `TestShowGlobalHooksForEvent` test added in Task 1-1.

**Outcome**: `grep "ShowGlobalHooks\b" internal/ cmd/` (excluding `ShowGlobalHooksForEvent`) returns no production or test reference to the no-arg method. All hook reads — production and test — go through `ShowGlobalHooksForEvent(event)`. The full `go test ./...` suite is green.

**Do**:
- Confirm zero production callers remain before deleting: `grep -n "\.ShowGlobalHooks()" internal cmd --include=*.go` (excluding `_test.go`) must return nothing. Tasks 1-2/1-3/1-4 must have removed them all; if `showGlobalHooksOrWarn` (in `hooks_register.go`) still calls `c.ShowGlobalHooks()`, repoint it at `ShowGlobalHooksForEvent` or delete it depending on whether Task 1-2 kept it.
- Delete the `ShowGlobalHooks` method (`internal/tmux/tmux.go` ~L766–775).
- In `internal/tmux/export_test.go` (~L31–35), delete the `ShowGlobalHooksOrWarn` re-export var. If `showGlobalHooksOrWarn` was deleted in Task 1-3, also delete this re-export and any test that drove it directly (`TestShowGlobalHooksOrWarn_*` in `hooks_register_warn_test.go`). If `showGlobalHooksOrWarn` survives (repointed at the per-event seam), update the re-export and its tests to use a per-event signature.
- Delete `TestShowGlobalHooks` in `internal/tmux/hooks_test.go` (~L11–73) — its coverage is replaced by `TestShowGlobalHooksForEvent` (Task 1-1). Keep `TestAppendGlobalHook` and `TestUnsetGlobalHookAt` (unchanged primitives).
- Migrate the remaining test READERS that call `client.ShowGlobalHooks()` to assert post-state:
  - `internal/tmux/hooks_migration_test.go` helpers `countSignalHydrateEntries` (~L107) and the verification readers (~L170, ~L336): change each to read per-event via `client.ShowGlobalHooksForEvent(ev)` and parse that event's slice. (These may already have been touched in Task 1-3; ensure no `ShowGlobalHooks()` call remains.)
  - `cmd/bootstrap/reboot_roundtrip_test.go` `verifyHydrationHookEntries` (~L1285): re-point the reader at `client.ShowGlobalHooksForEvent(event)` inside its existing per-event loop (it already loops `tmux.HydrationTriggerEvents`), reading each event's table directly instead of reading the global table once and indexing into the parsed map.
- Update `hooks_register_warn_test.go` if it still references the deleted helper/re-export: the `showHooksWarnRecords` / `assertShowHooksWarnShape` infrastructure tests the WARN shape; retarget any test that drove `ShowGlobalHooksOrWarn` directly to drive the convergence path's read failure instead (or delete if Task 1-2's convergence tests already cover the `show-hooks failed` WARN shape under the per-event seam).
- Run `go build -o portal .` and the FULL `go test ./...` (not just the tmux package) — `cmd/bootstrap` and any other package that read via the no-arg method must be green.

**Acceptance Criteria**:
- [ ] The `ShowGlobalHooks` method is removed from `internal/tmux/tmux.go`.
- [ ] `grep -rn "ShowGlobalHooks\b" internal cmd --include=*.go` returns only `ShowGlobalHooksForEvent` references (no bare `ShowGlobalHooks`).
- [ ] The `ShowGlobalHooksOrWarn` re-export is removed (or repointed at the per-event seam if `showGlobalHooksOrWarn` survives) and no test drives the deleted form.
- [ ] `cmd/bootstrap/reboot_roundtrip_test.go`'s `verifyHydrationHookEntries` reads via `ShowGlobalHooksForEvent`.
- [ ] `go build -o portal .` passes and the full `go test ./...` suite is green with no regressions.

**Tests** (no new behaviour — this task is deletion + fixture re-pointing; correctness is the full-suite green plus):
- `"the full go test ./... suite passes after ShowGlobalHooks deletion"` (the suite is the assertion; confirm `cmd/bootstrap` round-trip and `internal/tmux` packages compile and pass)
- Confirm `TestShowGlobalHooksForEvent` (from Task 1-1) is the sole direct unit test of the read seam.

**Edge Cases**:
- No production caller remains — the deletion must be preceded by Tasks 1-2/1-3/1-4 having migrated every caller; verify with grep before deleting.
- `showGlobalHooksOrWarn` / `ShowGlobalHooksOrWarn` re-export removed (or repointed) — no dangling reference to a deleted symbol.
- `reboot_roundtrip_test.go` reader re-pointed at the per-event seam — it already iterates `HydrationTriggerEvents`, so the change is a per-event read inside the loop.
- Full suite green — packages outside `internal/tmux` (notably `cmd/bootstrap`) that read hooks must compile and pass.

**Context**:
> Spec § Concrete mechanism: "Delete `ShowGlobalHooks` (the no-arg global read). It is the defect's single point of entry; with both registration and unregistration on the per-event seam, nothing should retain it. The `Client.ShowGlobalHooks` method itself is removed once no production caller remains; any test fixtures referencing it are migrated to the new seam." § Acceptance Criteria item 6: "Global read removed. The no-arg `ShowGlobalHooks` is deleted; no production caller remains. All hook reads go through `ShowGlobalHooksForEvent(event)`."

**Spec Reference**: `.workflows/state-notify-cascade-on-binary-upgrade/specification/state-notify-cascade-on-binary-upgrade/specification.md` §§ Concrete mechanism, Acceptance Criteria (6).

## state-notify-cascade-on-binary-upgrade-1-6 | approved

### Task 1-6: Real-tmux no-growth + blind-spot regression guards

**Problem**: The defect is a tmux-output-**shape** issue — the global `show-hooks -g` omits a class of events. It is invisible to string-fixture / mock commanders, which return whatever the test author wrote. The mock-commander tests in Tasks 1-2/1-4 prove the convergence and teardown logic but cannot prove the blind spot is real or that the fix actually defeats it. Only a real tmux server is a faithful oracle.

**Solution**: Add two real-tmux integration tests using the `internal/tmuxtest` socket fixtures. (1) A no-growth test: run `RegisterPortalHooks` N≥2 times against a real tmux server and assert every Portal-managed event's array stays at exactly one Portal entry — specifically `pane-focus-out` and `window-layout-changed` stay at 1 and never grow. (2) A blind-spot regression guard: assert that no-arg `show-hooks -g` omits the pane-scoped and geometry/rename window-scoped events while `show-hooks -g <event>` includes them, documenting the tmux 3.6b reality the fix is built on.

**Outcome**: Two real-tmux tests that fail against the pre-fix binary (no-growth) / document the tmux behaviour (blind-spot) and pass after the fix. The no-growth test is the direct regression guard for the bug.

**Do**:
- Add a new test file `internal/tmux/hooks_register_realtmux_test.go` (package `tmux_test`), guarded by `tmuxtest.SkipIfNoTmux(t)`.
- **No-growth test** `TestRegisterPortalHooks_NoGrowthAcrossBootstraps`:
  - `ts := tmuxtest.New(t, "ptl-hooks-")`; `client := ts.Client()`; `client.EnsureServer()`.
  - Run `tmux.RegisterPortalHooks(client, nil)` N times (N=3 is sufficient; ≥2 required).
  - After each run, for every managed event (the six notify events, `session-closed`, and the two hydration events), read via `client.ShowGlobalHooksForEvent(event)`, parse, count entries whose body matches that event's Portal fingerprint, and assert the count is exactly 1.
  - Assert specifically and by name that `pane-focus-out` and `window-layout-changed` stay at 1 across all N runs (these are the events a global read cannot see — they are the regression target). Use the per-event read for the count so the assertion itself is not blind.
  - Optionally cross-check the desired body equals the expected constant on those two events.
- **Blind-spot regression guard** `TestShowHooksGlobalEnumeration_OmitsPaneAndGeometryEvents`:
  - Fresh `tmuxtest` server. Append a Portal-shape hook directly via `client.AppendGlobalHook(event, body)` on a set of events that includes at least: a pane-scoped event (`pane-focus-out`), a geometry/rename window event (`window-layout-changed`), and a control event known to be enumerated globally (e.g. `session-created`).
  - Read the no-arg global table via `ts.Run(t, "show-hooks", "-g")` (drive raw tmux through the socket, NOT a deleted client method) and parse it. Assert: the control event (`session-created`) IS present in the global enumeration, while `pane-focus-out` and `window-layout-changed` are ABSENT (the documented tmux 3.6b blind spot).
  - Read each of those events per-event via `client.ShowGlobalHooksForEvent(event)` and assert each returns the entry — proving the per-event seam is not blind.
  - If a future tmux version changes this behaviour the test will fail loudly, which is the intended early-warning signal; add a comment documenting that a failure here means the tmux blind-spot assumption changed (not necessarily a Portal bug), and that the per-event fix remains correct regardless.
- Tests must NOT use `t.Parallel()` (project convention). Use absolute-path socket fixtures via `tmuxtest.New` (already handles socket length caps and cleanup).
- Run `go test ./internal/tmux/...` (with tmux on PATH) to confirm both pass after the fix.

**Acceptance Criteria**:
- [ ] `TestRegisterPortalHooks_NoGrowthAcrossBootstraps` runs `RegisterPortalHooks` N≥2 times on a real tmux server and asserts every managed event stays at exactly one Portal entry, naming `pane-focus-out` and `window-layout-changed` explicitly.
- [ ] The no-growth test reads per-event (via `ShowGlobalHooksForEvent`) so the count assertion is itself not blind.
- [ ] `TestShowHooksGlobalEnumeration_OmitsPaneAndGeometryEvents` asserts no-arg `show-hooks -g` omits `pane-focus-out` and `window-layout-changed` while including a control event (`session-created`), and that `ShowGlobalHooksForEvent` returns each omitted event's entry.
- [ ] Both tests are guarded by `tmuxtest.SkipIfNoTmux(t)` and use the `internal/tmuxtest` socket fixtures.
- [ ] No `t.Parallel()`; tests pass under `go test ./internal/tmux/...` with tmux available.

**Tests**:
- `"it leaves pane-focus-out and window-layout-changed at exactly one entry across N≥2 bootstraps (no growth)"`
- `"it leaves every managed event at exactly one Portal entry across N≥2 bootstraps"`
- `"no-arg show-hooks -g omits pane-focus-out and window-layout-changed while including session-created"`
- `"show-hooks -g <event> returns the entry the global form omits"`

**Edge Cases**:
- Blind events (`pane-focus-out`, `window-layout-changed`) stay at 1 across N≥2 — the direct regression guard; fails against the pre-fix binary.
- No-arg global form omits pane/geometry events while the per-event form includes them — locks the tmux 3.6b reality and catches a future tmux behaviour change.
- The count assertion must read per-event, never via the (now-deleted) global method, so the test oracle is not itself blind.

**Context**:
> Spec § Testing Requirements: "The defect is a tmux-output-shape issue … invisible to string-fixture / mock commanders … The only faithful oracle is a real tmux server. Tests that would prove this fix MUST use the real-tmux socket fixtures (`internal/tmuxtest`)." Required test 1 (No-growth integration test): "Run hook registration N times against a real tmux server; assert every Portal hook array — and specifically `pane-focus-out` and `window-layout-changed`, the events a global read cannot see — stays at exactly 1. This is the direct regression guard for the bug; it fails today and passes after the fix." Required test 2 (Blind-spot regression guard): "assert that no-arg `show-hooks -g` omits the pane-scoped and geometry/rename window-scoped events while `show-hooks -g <event>` includes them." § Acceptance Criteria 1 (No growth across bootstraps), 8 (Cascade eliminated — a single managed event spawns exactly one `portal state notify`, the downstream consequence of holding the array at 1).

**Spec Reference**: `.workflows/state-notify-cascade-on-binary-upgrade/specification/state-notify-cascade-on-binary-upgrade/specification.md` §§ Testing Requirements (1, 2), Acceptance Criteria (1, 8).

## state-notify-cascade-on-binary-upgrade-1-7 | approved

### Task 1-7: Real-tmux self-heal, teardown-at-depth, and idempotency/no-churn guards

**Problem**: The mock-commander tests cannot faithfully reproduce the stacked-array collapse, the teardown-at-depth reap on blind events, or the no-churn idempotency — all depend on real `show-hooks` output shape and real `set-hook -gu` index semantics, which a mock would have to re-implement to be faithful. The self-heal of the live 139-deep stacks, the `portal hooks reset` reap on blind events, and the churn-free second registration must be proven against a real tmux server.

**Solution**: Add three real-tmux integration tests using `internal/tmuxtest`. (1) Self-heal: seed an event with K stacked Portal entries plus a co-resident user hook, run one `RegisterPortalHooks`, assert collapse to exactly one Portal entry with the user hook untouched. (2) Teardown-at-depth: with stacked Portal entries on the blind events plus a co-resident user hook, run `UnregisterPortalHooks`, assert all Portal entries reaped (Portal count → 0) with the user hook intact. (3) Idempotency / no-churn: after convergence, a second `RegisterPortalHooks` performs no unset and no append and emits no eviction INFO line.

**Outcome**: Three real-tmux tests proving self-collapse of K-deep stacks (including 139-deep), teardown reaping at depth on the blind events, and churn-free idempotency — the regression guards for Acceptance Criteria 2, 4, 5, and 7.

**Do**:
- Add to `internal/tmux/hooks_register_realtmux_test.go` (or a sibling real-tmux file), package `tmux_test`, each guarded by `tmuxtest.SkipIfNoTmux(t)` and using `tmuxtest.New`.
- **Self-heal test** `TestRegisterPortalHooks_SelfHealsKDeepStackLeavingUserHookIntact`:
  - Fresh server. Pick a blind event (`pane-focus-out`). Seed K stacked identical Portal entries: loop `K` times calling `client.AppendGlobalHook("pane-focus-out", notifyBody)` where `notifyBody` is the desired `notifyCommand` constant (K = a representative depth, e.g. 5; a comment may note the live incident was 139, and the test could optionally parametrize a larger K — keep wall-clock bounded). Also append ONE co-resident user hook on the same event with a body that contains no Portal fingerprint (e.g. `run-shell 'echo user pane-focus-out hook'`).
  - Run `tmux.RegisterPortalHooks(client, nil)` ONCE.
  - Read `pane-focus-out` per-event; assert exactly one Portal-fingerprinted entry remains, its body equals the desired `notifyCommand`, AND the user hook is still present (exactly one entry containing `echo user pane-focus-out hook`).
- **Teardown-at-depth test** `TestUnregisterPortalHooks_ReapsAtDepthOnBlindEventsLeavingUserHookIntact`:
  - Fresh server. On EACH blind event (`pane-focus-out` and `window-layout-changed`), seed K stacked Portal entries (e.g. via repeated `AppendGlobalHook` with the notify body) plus one co-resident user hook (non-Portal body).
  - Run `tmux.UnregisterPortalHooks(client)` once.
  - Read each blind event per-event; assert zero entries contain any `portalCommandSubstrings` fingerprint (Portal count → 0) AND the user hook survives on each event. This is the test that no-ops today (pre-fix teardown sees zero entries on the blind events via the global read) and passes after Task 1-4.
- **Idempotency / no-churn test** `TestRegisterPortalHooks_SecondRegistrationIsChurnFree`:
  - Fresh server. Run `RegisterPortalHooks` once to converge (use a recording logger capturing INFO/WARN — reuse the existing `recordingLogger`/`recordingMigrationLogger` capture seam, or `log.SetTestHandler`).
  - Capture the per-event entry indices after the first run (read each managed event, record `HookEntry.Index` values).
  - Run `RegisterPortalHooks` a SECOND time with a fresh recording logger.
  - Assert: (a) the per-event entry indices are UNCHANGED between the two runs (no renumbering → no unset+append occurred); (b) no eviction INFO line was emitted on the second run (the `reaped` summary's absence is the asserted signal); (c) no WARN was emitted. Because real tmux is the oracle, "no unset/append" is proven structurally by index stability rather than by counting mock calls.
- Tests must NOT use `t.Parallel()`. Keep K modest to bound wall-clock; a comment may reference the 139-deep live incident to justify the chosen K is representative of the same collapse path.
- Run `go test ./internal/tmux/...` (with tmux on PATH) to confirm all three pass after the fix.

**Acceptance Criteria**:
- [ ] Self-heal: a K-deep stack on `pane-focus-out` collapses to exactly one Portal entry (body == desired `notifyCommand`) after one `RegisterPortalHooks`, with a co-resident user hook left untouched.
- [ ] Teardown-at-depth: stacked Portal entries on BOTH blind events are fully reaped by `UnregisterPortalHooks` (Portal count → 0) with the co-resident user hook intact on each event.
- [ ] Idempotency / no-churn: a second `RegisterPortalHooks` leaves per-event entry indices unchanged (no unset/append) and emits no eviction INFO line and no WARN.
- [ ] All three tests use `internal/tmuxtest` real-tmux fixtures, are guarded by `SkipIfNoTmux`, and do not use `t.Parallel()`.
- [ ] `go test ./internal/tmux/...` passes with tmux available.

**Tests**:
- `"it collapses a K-deep pane-focus-out stack to one entry while leaving a co-resident user hook intact"`
- `"it reaps stacked Portal entries on pane-focus-out and window-layout-changed to zero while leaving user hooks intact"`
- `"a second registration leaves per-event indices unchanged and emits no eviction INFO and no WARN"`

**Edge Cases**:
- K-deep stack collapses to 1 with the co-resident user hook intact (self-heal on a blind event; the live incident was 139-deep on `pane-focus-out` and `window-layout-changed`).
- Teardown reaps at depth on both blind events with the user hook intact (the path that no-ops pre-fix).
- Second registration emits no unset/append (proven by index stability under real tmux) and no `reaped` INFO — the absence is the asserted churn-free signal.

**Context**:
> Spec § Testing Requirements: Required test 3 (Self-heal assertion): "Seed an event with K stacked Portal entries; run one registration; assert it collapses to exactly 1, and that a co-resident user-authored hook on the same event is left untouched." Required test 4 (Teardown-at-depth): "With stacked Portal entries on the blind events, assert `UnregisterPortalHooks` reaps them all (it no-ops today), leaving any co-resident user hook intact." Required test 5 (Idempotency / no-churn): "After convergence, a second registration produces no unset/append (e.g. hook indices unchanged, no eviction log line)." § Acceptance Criteria 2 (Existing stacks self-collapse — K stacked entries collapse to one after a single registration, no dedicated cleanup), 4 (User hooks survive both registration and teardown, including on the two blind events), 5 (Teardown reaps at depth on every managed event including the blind ones, Portal count → 0), 7 (Idempotent and churn-free — no renumbering, no log churn). The eviction line uses the existing `reaped` attr under the `bootstrap` component; its absence on the churn-free run is the asserted signal.

**Spec Reference**: `.workflows/state-notify-cascade-on-binary-upgrade/specification/state-notify-cascade-on-binary-upgrade/specification.md` §§ Testing Requirements (3, 4, 5), Acceptance Criteria (2, 4, 5, 7).
