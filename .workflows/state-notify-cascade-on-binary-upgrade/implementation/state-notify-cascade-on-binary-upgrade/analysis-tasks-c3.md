---
topic: state-notify-cascade-on-binary-upgrade
cycle: 3
total_proposed: 3
---
# Analysis Tasks: State Notify Cascade on Binary Upgrade (Cycle 3)

## Task 1: Make teardown reap the converged session-closed commit-now hook (close the fingerprint seam, AC #5)
status: pending
severity: medium
sources: architecture

**Problem**: Registration converges the `session-closed` event onto `commitNowCommand`
(`run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`), whose only Portal
fingerprint is `commitNowSubstring` = `"portal state commit-now"`. Teardown's
`portalCommandSubstrings` set in `internal/tmux/hooks_unregister.go:32-36` is
`{"portal state notify", "portal state signal-hydrate", "portal state migrate-rename"}` — it
omits `"portal state commit-now"`. Consequence: `UnregisterPortalHooks` / `portal hooks reset`
reads `session-closed` per-event, finds the converged commit-now entry, classifies it as
non-Portal (no substring matches), and leaves it installed. A Portal-authored hook on a managed
event survives teardown, directly contradicting Acceptance Criterion 5 ("removes all Portal
entries at any depth on every managed event"). The work derived `portalEvents` from
`managedEvents` so the two paths provably scan the identical event-set, and
`TestPortalManagedEventSetParity` guards it — but that parity is over *events*, not over
*fingerprints*. Registration's per-event fingerprint set (which carries `commitNowSubstring` for
session-closed via the two-element set at `hooks_register.go:47`) and teardown's flat
`portalCommandSubstrings` diverge in a way that makes one registered category unreachable by
teardown. The existing real-tmux teardown-at-depth test
(`TestUnregisterPortalHooks_ReapsAtDepthOnBlindEventsLeavingUserHookIntact` in
`hooks_register_realtmux_test.go:375`) asserts post-teardown that zero entries match
`"portal state commit-now"` (its `teardownFingerprints` slice at lines 409-414 includes it), but
it only ever seeds `notify` bodies on the two blind events (`pane-focus-out`,
`window-layout-changed`) and never seeds a commit-now body on `session-closed`, so that assertion
is vacuously green and the gap is uncovered. The commit-now registration predates this work unit
(killed-session-resurrects-within-tick-window fix), but this work unit rewrote teardown and
re-grounded the event-set on `managedEvents` without closing the corresponding fingerprint gap,
so closing it here is in-scope per AC #5.

**Solution**: Close the fingerprint seam so teardown reaps every category registration installs,
in a drift-proof way that mirrors how `managedEventNames()` makes the event-set drift-proof.
Derive teardown's fingerprint set from the UNION of every `managedEvents` entry's `fingerprints`
PLUS the legacy `"portal state migrate-rename"` substring (which teardown intentionally retains
and registration intentionally omits). A future hook category added to `managedEvents` then
automatically widens teardown coverage, the same way adding an event widens `portalEvents`. The
simpler literal-add alternative (just append `commitNowSubstring` to the `portalCommandSubstrings`
literal) is acceptable as a fallback if the union derivation proves awkward, but the union
approach is strongly preferred for drift-proofing. Preserve the two intentional asymmetries:
registration must still NOT install or converge `migrate-rename` (it stays absent from every
`managedEvents` fingerprint set), and teardown must still retain the `migrate-rename` substring
(it is not in `managedEvents`, so the union must explicitly add it).

**Outcome**: `UnregisterPortalHooks` / `portal hooks reset` reaps the converged `session-closed`
commit-now hook at any depth, restoring AC #5. Teardown's fingerprint set is derived from
`managedEvents` (plus the explicit legacy `migrate-rename` addend) so it cannot drift from
registration's installed categories. The real-tmux teardown-at-depth test seeds a stacked
commit-now body on `session-closed` and asserts it reaps to zero, so the AC-5 assertion is no
longer vacuous. The intentional registration-vs-teardown `migrate-rename` asymmetry is preserved
and documented.

**Do**:
1. In `internal/tmux/hooks_unregister.go`, replace the hand-authored
   `portalCommandSubstrings` literal (lines 32-36) with a value derived from the production
   `managedEvents` table. Build the union of every `managedEvents[i].fingerprints` element
   (de-duplicated, order-stable to match the existing teardown removal-order assertions if any
   depend on it), then explicitly append the legacy `"portal state migrate-rename"` substring
   that teardown retains but registration omits. Express the legacy addend as a named constant
   (e.g. reuse or introduce a `migrateRenameSubstring = "portal state migrate-rename"` const)
   rather than a bare literal, so it is greppable and self-documenting.
2. Add a small unexported helper (e.g. `teardownFingerprints()` analogous to
   `managedEventNames()`) in either `hooks_register.go` (alongside `managedEventNames`, since it
   reads `managedEvents`) or `hooks_unregister.go`, that returns the union-plus-legacy slice.
   Initialise `portalCommandSubstrings` from it (`var portalCommandSubstrings = teardownFingerprints()`).
3. Update the doc comment on `portalCommandSubstrings` to explain it is now DERIVED from
   `managedEvents` fingerprints (so adding a category to `managedEvents` automatically widens
   teardown coverage), plus the explicitly-retained legacy `migrate-rename` substring that
   registration never installs. Preserve the existing rationale block explaining WHY
   `migrate-rename` is retained (older binaries shipped the inert hook on `session-renamed`).
4. Do NOT touch `managedEvents` in `hooks_register.go` — registration must continue to NOT
   install/converge `migrate-rename` (it stays absent from every fingerprint set), and the
   `session-closed` two-element `{notifySubstring, commitNowSubstring}` set is already correct.
5. Extend the real-tmux teardown-at-depth test
   `TestUnregisterPortalHooks_ReapsAtDepthOnBlindEventsLeavingUserHookIntact`
   (`hooks_register_realtmux_test.go:375`) so the AC-5 assertion is no longer vacuous: seed a
   K-deep stack of `commitNowCommand` (the `portal state commit-now` body) on `session-closed`
   (plus a co-resident non-Portal user hook on that event), add a pre-condition sanity assert
   that the seeded commit-now count equals the stack depth, then after the single
   `UnregisterPortalHooks` call assert the `session-closed` commit-now count reaps to zero while
   the user hook on `session-closed` survives. Reuse the existing `teardownFingerprints` slice
   and `countPortalEntriesForEvent` helper so the `"portal state commit-now"` fingerprint is now
   exercised against a real seeded body. If `session-closed` is not currently in the test's
   event list, add it (or add a focused sibling test) so the commit-now body is actually present
   when the `"portal state commit-now"` assertion runs.
6. Verify the intentional asymmetry is still preserved by the existing tests: registration must
   not converge `migrate-rename` (no `managedEvents` entry carries it), and teardown must still
   match a stale `migrate-rename` body on `session-renamed`.
7. `go build -o portal .` and `go test ./internal/tmux/...` (including the real-tmux integration
   tests where tmux is available) and `go vet ./internal/tmux/...` must pass.

**Acceptance Criteria**:
- `portalCommandSubstrings` includes `"portal state commit-now"` (directly or via the derived
  union), so `UnregisterPortalHooks` reaps the converged `session-closed` commit-now entry.
- The teardown fingerprint set is derived from the union of `managedEvents` fingerprints plus the
  explicitly-retained legacy `"portal state migrate-rename"` substring — not an independently
  hand-authored literal — so a future category added to `managedEvents` automatically widens
  teardown coverage.
- Registration is unchanged: `managedEvents` carries no `migrate-rename` fingerprint on any
  event, and `session-closed` retains its `{notifySubstring, commitNowSubstring}` two-element set.
- Teardown still retains `"portal state migrate-rename"` so it reaps stale legacy entries from
  older binaries on `session-renamed`.
- The real-tmux teardown-at-depth test seeds a stacked `commitNowCommand` on `session-closed` and
  asserts it reaps to zero with a non-vacuous pre-condition count check; the co-resident user hook
  on `session-closed` survives.
- `go build`, `go test ./internal/tmux/...`, and `go vet ./internal/tmux/...` pass.

**Tests**:
- Extend `TestUnregisterPortalHooks_ReapsAtDepthOnBlindEventsLeavingUserHookIntact` (or add a
  focused sibling) to seed K stacked `commitNowCommand` entries on `session-closed` plus a user
  hook, assert the seeded commit-now count == K as a pre-condition (so a green pass cannot be
  vacuous), run one `UnregisterPortalHooks`, and assert the `session-closed` commit-now count → 0
  while the user hook survives at count 1.
- A unit test asserting the derived `portalCommandSubstrings` contains every `managedEvents`
  fingerprint (including `commitNowSubstring`) plus `"portal state migrate-rename"`, mirroring
  the spirit of `TestPortalManagedEventSetParity` but over fingerprints — guards against future
  fingerprint drift between registration and teardown.
- Confirm existing `TestPortalManagedEventSetParity` and the `migrate-rename`-retention teardown
  test(s) still pass unchanged, proving the intentional asymmetry is preserved.

## Task 2: Consolidate the forked register/teardown test-harness dispatch + line-scoping helpers and propagate the no-arg-global-read fatal guard to teardown
status: pending
severity: low
sources: duplication

**Problem**: The registration tests own `perEventDispatch` / `perEventDispatchWithFaults`
(`internal/tmux/hooks_register_test.go:95-151`) and the teardown tests own
`dispatchUnregisterHooks` (`internal/tmux/hooks_unregister_test.go:18-35`). Both are
`MockCommander` `RunFunc` builders modeling the same tmux contract for the same per-event fix:
answer a per-event `show-hooks -g <event>` read by returning only the queried event's lines, and
answer `set-hook -gu <event>[idx]` with an optional per-target injected error. They were authored
independently across the register/teardown task boundary and have already begun to drift:
`perEventDispatchWithFaults` carries a load-bearing `t.Fatalf` guard
(`hooks_register_test.go:128-131`) that fails if the no-arg global `show-hooks -g` is ever issued
— the exact invariant this whole fix exists to enforce — while `dispatchUnregisterHooks` has NO
such guard, so a teardown regression that reverted to the blind no-arg global read would pass
silently under its mock. Separately, `parseSeededTableByEvent`
(`hooks_register_test.go:186-200`, a map-builder) and `linesForEvent`
(`hooks_unregister_test.go:41-51`, a single-event filter) independently re-implement the same
primitive — scope a whole-table show-hooks fixture to one event's lines, keyed on the `<event>[`
prefix. Two slightly-different shapes that must stay in agreement about how an event line is
recognised. The shared production seam both exercise (`evictPortalEntries` +
`ShowGlobalHooksForEvent`) is unified; only the test harness is forked.

**Solution**: Collapse the forked test-harness pair into single `tmux_test`-package primitives.
Have the teardown dispatcher reuse the register-side per-event read/unset skeleton — including the
no-arg-global-read `t.Fatalf` tripwire — so teardown inherits the same "must read per-event"
guard. Either call `perEventDispatchWithFaults(t, table, nil, nil, unsetErrFor)` from the teardown
tests, or extract the per-event read/unset skeleton (with the no-arg-global fatal guard) into one
shared helper both sides build on. Pick one of `parseSeededTableByEvent` / `linesForEvent` as the
single line-scoping primitive ("scope a whole-table show-hooks fixture to one event's lines") and
express the other in terms of it. The unified line-scoping must recognise both the register
fixture shape (`<event>[i] => '...'`) and the unregister fixture shape
(`<event>[i] run-shell '...'`) by matching on the `<event>[` prefix, which both already do.

**Outcome**: One per-event mock-dispatch builder and one whole-table-to-per-event line-scoping
primitive serve both the register and teardown test files. The teardown dispatcher carries the
no-arg-global-read `t.Fatalf` tripwire, so a teardown regression to the blind global read fails
loudly instead of passing silently — the safety invariant now covers both paths. There is exactly
one definition of "an event line begins with `<event>[`".

**Do**:
1. Decide the consolidation shape: prefer calling the existing
   `perEventDispatchWithFaults(t, table, nil, nil, unsetErrFor)` from the teardown tests
   (`dispatchUnregisterHooks` needs only the show-hooks read + `set-hook -gu` legs, a strict
   subset of what `perEventDispatchWithFaults` already provides). If the teardown call sites need
   a thinner ergonomic wrapper, keep a `dispatchUnregisterHooks` shim that delegates to
   `perEventDispatchWithFaults` rather than re-implementing the leg dispatch and missing the
   guard.
2. Ensure the shared dispatcher's no-arg-global-read `t.Fatalf` guard
   (`hooks_register_test.go:128-131`) now executes on the teardown path too — this is the
   load-bearing point of the consolidation. After the change, a teardown mock that ever sees a
   no-arg `show-hooks -g` (len(args) < 3) must fail the test.
3. Pick one line-scoping primitive. Recommended: keep `parseSeededTableByEvent` (the map-builder)
   as the single owner, and reimplement `linesForEvent(showOutput, event)` as a thin lookup
   (`parseSeededTableByEvent(showOutput)[event]`) — or delete `linesForEvent` and have the
   teardown tests use the map directly. Whichever is kept must recognise both fixture body shapes
   by matching on the `<event>[` prefix (the index bracket), so no caller's fixture stops parsing.
4. Update all teardown test call sites (`hooks_unregister_test.go`) to use the consolidated
   helpers; remove the now-dead `dispatchUnregisterHooks` / `linesForEvent` bodies (or reduce
   them to delegating shims).
5. Confirm `go test ./internal/tmux/...` passes and that the teardown tests still inject per-index
   unset faults correctly through the unified `unsetErrFor` channel.

**Acceptance Criteria**:
- The teardown tests drive their per-event mock dispatch through the same skeleton as the
  register tests (via `perEventDispatchWithFaults` or a shared extracted helper), and that
  skeleton's no-arg-global-read `t.Fatalf` guard executes on the teardown path.
- A simulated teardown regression to a no-arg `show-hooks -g` read fails the teardown test loudly
  (the tripwire now covers both paths).
- Exactly one whole-table-to-per-event line-scoping primitive exists in the `tmux_test` package;
  the other is expressed in terms of it or removed.
- The unified line-scoping recognises both the register (`<event>[i] => '...'`) and unregister
  (`<event>[i] run-shell '...'`) fixture shapes via the `<event>[` prefix.
- Per-index unset fault injection still works for the teardown tests through the shared
  `unsetErrFor` channel.
- `go test ./internal/tmux/...` and `go vet ./internal/tmux/...` pass; no `t.Parallel()`
  introduced.

**Tests**:
- Existing teardown unit tests in `hooks_unregister_test.go` continue to pass after rewiring onto
  the shared dispatch + line-scoping helpers (including the per-index unset-fault cases).
- A guard assertion (or an existing register-side test reused on the teardown path) confirming a
  no-arg `show-hooks -g` issued under the consolidated teardown dispatcher triggers the
  `t.Fatalf` tripwire — proving the invariant now covers teardown.
- Existing register-side tests (`perEventDispatch` / `perEventDispatchWithFaults` consumers)
  continue to pass unchanged after the line-scoping primitive is unified.

## Task 3: Fix the stale doc comment in the six-event routing test (deleted test name + prior-spec heading)
status: pending
severity: low
sources: standards

**Problem**: The file-level rationale comment in
`internal/tmux/hooks_register_six_event_routing_test.go:22-23` claims "The existing
TestRegisterPortalHooks_SessionClosedMigration's nonSessionClosedSaveTriggerEvents loop already
asserts each of the six events resolves to notifyCommand". No test named
`TestRegisterPortalHooks_SessionClosedMigration` exists anywhere in the codebase — under this
spec's Migration-Helper Consolidation, `migrateSessionClosedHook` was folded into the unified
convergence engine and the corresponding test was renamed/restructured. The current session-closed
tests are `TestRegisterPortalHooks_SessionClosedUnionFastPath` /
`*SessionClosedSubstringEvictsPortalStateNotifyBody` / `*SessionClosedNonMatchingUserHookSurvives`,
and the fresh-table six-event coverage now lives in `TestRegisterPortalHooks_FreshTable`. The same
comment block (and an in-body `t.Errorf` message at lines 99-104) points reviewers at spec section
"Hook Registration Migration → Registration Strategy", which is a heading from a different/prior
spec, not this work unit's spec (whose heading is "Registration Redesign — Ensure Exactly One").
The project constraint requires comments to accurately describe the code they sit on; this comment
misdirects a reader to a non-existent test as the stated justification for the gate's existence.
The test body and assertions themselves are correct and conform to the spec.

**Solution**: Update the stale references so the comment accurately describes the current code and
spec. Replace the `TestRegisterPortalHooks_SessionClosedMigration` reference with the current
covering test (`TestRegisterPortalHooks_FreshTable`, which asserts each non-session-closed
save-trigger event resolves to the notify command on a fresh table). Re-anchor the
"Hook Registration Migration → Registration Strategy" section pointer (both in the file-level
comment and in the `t.Errorf` message at lines 99-104) to this work unit's spec heading
"Registration Redesign — Ensure Exactly One" (or the current per-event parameter-table section).

**Outcome**: The six-event routing test's rationale comment and assertion messages cite a test
that actually exists (`TestRegisterPortalHooks_FreshTable`) and the current spec section heading,
so a reviewer following the justification is no longer misdirected. No behavioural change.

**Do**:
1. In `internal/tmux/hooks_register_six_event_routing_test.go`, edit the file-level comment
   (around lines 22-23) to reference `TestRegisterPortalHooks_FreshTable` (the current covering
   test) instead of the deleted `TestRegisterPortalHooks_SessionClosedMigration`.
2. Re-anchor the spec section pointer in the same comment block from "Hook Registration Migration
   → Registration Strategy" to this work unit's spec heading "Registration Redesign — Ensure
   Exactly One" (confirm the exact current heading against the specification file before editing).
3. Update the matching spec-section reference in the in-body `t.Errorf` message at lines 99-104 so
   the failure message points reviewers at the current heading rather than the prior spec's.
4. Make no changes to the test logic, sub-test structure, or assertions — they are correct.
5. `go build` and `go test ./internal/tmux/...` and `go vet ./internal/tmux/...` must still pass
   (comment-only change).

**Acceptance Criteria**:
- The file-level comment references `TestRegisterPortalHooks_FreshTable` (a test that exists), not
  `TestRegisterPortalHooks_SessionClosedMigration` (which does not).
- The spec-section pointer in both the file-level comment and the in-body `t.Errorf` message names
  this work unit's current spec heading, not the prior spec's "Hook Registration Migration →
  Registration Strategy".
- No test logic, sub-test names, or assertions change.
- `go build`, `go test ./internal/tmux/...`, and `go vet ./internal/tmux/...` pass.

**Tests**:
- `go test ./internal/tmux/...` passes (comment-only change; existing six-event routing
  assertions continue to pass unchanged) — confirms the edit did not disturb the test body.
