---
topic: state-notify-cascade-on-binary-upgrade
cycle: 1
total_proposed: 5
---
# Analysis Tasks: State Notify Cascade on Binary Upgrade (Cycle 1)

## Task 1: Make managedEvents the single source of truth for the Portal-managed event set
status: pending
severity: medium
sources: architecture

**Problem**: The convergence engine iterates `managedEvents` (a hand-written 9-entry table in `internal/tmux/hooks_register.go:45-55`), while teardown iterates `portalEvents`, derived as `slices.Concat(saveTriggerEvents, HydrationTriggerEvents)` (`internal/tmux/hooks_unregister.go:52`, sourced from `hooks_register.go:60-82`). These are two parallel, independently-authored enumerations of the same domain fact — which tmux events Portal manages — with the 8 non-session-closed event names duplicated as raw string literals across both. Nothing in production derives one set from the other, and no test asserts the two event-sets are equal. The failure mode is latent but real and is exactly the bug class this spec exists to eliminate: adding an event to `managedEvents` (registration installs a hook) without adding it to the save/hydration slices means teardown silently never reaps it (`portal hooks reset` leaves it stacked); the inverse leaves an event torn down but never converged. Additionally, the `saveTriggerEvents` doc comment (`hooks_register.go:57-59`) is now stale and actively misleading: it claims (1) the slice lists events `RegisterPortalHooks` registers a `portal state notify` hook on and processes in order, but `RegisterPortalHooks` no longer iterates `saveTriggerEvents` — it iterates `managedEvents`, and the slice's only remaining production consumer is `portalEvents`; (2) "Order is significant," which the spec establishes is no longer true for correctness; (3) the framing ignores that `session-closed` is in the list but registers `commit-now`, not `portal state notify`.
**Solution**: Derive `portalEvents` by projecting the `event` field out of `managedEvents` rather than re-concatenating `saveTriggerEvents` + `HydrationTriggerEvents`, making `managedEvents` the single source of truth for the event-set. If full unification removes `saveTriggerEvents` as a production consumer, retire the slice (or, if it still has a consumer, rewrite its now-stale doc comment). If full derivation is judged out of this bugfix's scope, the minimum acceptable alternative is a parity test asserting the set of `me.event` values in `managedEvents` equals `portalEvents`, so drift fails a test rather than a user's hook table — and the stale `saveTriggerEvents` comment must still be corrected regardless.
**Outcome**: There is exactly one production declaration of the Portal-managed event-set, and registration and teardown provably operate over the same events. Adding a future event to `managedEvents` automatically widens teardown coverage (or, in the parity-test variant, fails a test until the second slice is updated). No production doc comment claims a non-existent ordering contract or registration loop.
**Do**:
1. Read `internal/tmux/hooks_register.go:45-82` and `internal/tmux/hooks_unregister.go:39-52` to confirm the current `managedEvents`, `saveTriggerEvents`, `HydrationTriggerEvents`, and `portalEvents` shapes.
2. Preferred approach — full derivation:
   - Change `portalEvents` (`hooks_unregister.go:52`) to be built by projecting the `event` field out of every `managedEvents` entry (e.g. a small init/helper that maps `managedEvents` → `[]string{event...}`), preserving declaration order for stable output.
   - Remove the now-unused `saveTriggerEvents` slice if `portalEvents` was its last production consumer; otherwise rewrite its doc comment to state it is solely a teardown-scan building block, drop the "Order is significant" / "RegisterPortalHooks processes..." claims, and remove the `notify`-only framing that ignores `session-closed`/`commit-now`.
   - Update the `portalEvents` doc comment (`hooks_unregister.go:39-52`) to describe the new derivation from `managedEvents`.
3. Minimum-viable alternative if full derivation is descoped: keep both slices but add a unit test asserting `{me.event for me in managedEvents} == set(portalEvents)`, AND still rewrite the stale `saveTriggerEvents` doc comment per step 2.
4. DO NOT touch the registration/teardown fingerprint-set divergence: teardown's `portalCommandSubstrings` deliberately includes `portal state migrate-rename` while registration's `managedEvents` deliberately omits it. This is intentional, documented (`hooks_register.go:40-44`, `hooks_unregister.go:24-37`), and out of scope — it is orthogonal to the event-set and must be preserved exactly.
5. Run `go build ./...`, `go vet ./...`, and the full `internal/tmux` suite (including real-tmux integration tests).
**Acceptance Criteria**:
- `managedEvents` is the single production source for the Portal-managed event-set, OR a parity test fails on any divergence between `managedEvents`' event values and `portalEvents`.
- Registration and teardown demonstrably operate over the identical event set.
- The `migrate-rename` teardown-only fingerprint divergence is unchanged.
- No production doc comment asserts an ordering contract or a `saveTriggerEvents`/`notify`-only framing that no longer holds.
- `go build`, `go vet`, and the full `internal/tmux` suite pass (real-tmux integration tests included).
**Tests**:
- A unit test asserting the set of `managedEvents` event values equals `portalEvents` (in the derivation variant this becomes a tautology-guard against accidental future re-divergence; in the minimum-viable variant it is the primary drift tripwire).
- Existing teardown-at-depth and no-growth-across-bootstraps real-tmux guards continue to pass unchanged, confirming the derived/asserted event-set still covers every managed event.

## Task 2: Collapse the eight hand-rolled per-event dispatch RunFuncs onto perEventDispatch with optional fault injection
status: pending
severity: medium
sources: duplication

**Problem**: The convergence-path tests already share `perEventDispatch` (`internal/tmux/hooks_register_test.go:71`) and `parseSeededTableByEvent` (`hooks_register_test.go:102`) for the "answer `show-hooks -g <event>` per-event, fatal on a no-arg global read, dispatch `set-hook -ga`/`-gu`" shape. But 8 tests across three in-scope files hand-roll a bespoke `runFunc`/`RunFunc` that re-implements the identical 3-branch skeleton: `internal/tmux/hooks_register_test.go:546-564`, `:619-642`; `internal/tmux/hooks_register_warn_test.go:88-96`, `:129-145`, `:184-202`, `:246-258`; `internal/tmux/hooks_migration_test.go:329-349`, `:446-458`. The fatal-guard string `"convergence engine must read per-event, not the no-arg global show-hooks -g"` is copy-pasted verbatim across 5 of them. These copies exist only because the shared helper cannot inject a read error, a per-index unset error, or a `CommandError`, so each reproduces the whole skeleton to vary one branch — meaning the shared structure and its per-event-read invariant can drift independently per copy. `hooks_migration_test.go:319` already documents the copy as mirroring `TestRegisterPortalHooks_PerIndexUnsetFailureWarnsAndContinues`.
**Solution**: Extend `perEventDispatch` (or add one sibling builder) to accept optional per-event fault injection: `readErrFor map[string]error` and `unsetErrFor map[string]error` (it already takes `setHookErrFor`). The fatal-guard and per-event read/`set-hook` branches then live in one place; the 8 bespoke RunFuncs collapse to one-line helper calls passing the relevant fault maps. Tests needing a `CommandError` pass it through `readErrFor`.
**Outcome**: The per-event read/dispatch skeleton and its no-arg-global-read fatal guard exist in exactly one helper. The 8 previously-bespoke RunFuncs are one-line calls into that helper, varying only the injected fault. The per-event-read invariant can no longer drift between copies.
**Do**:
1. Read `perEventDispatch` (`hooks_register_test.go:65-96`) and `parseSeededTableByEvent` (`:98-116`) to confirm the current signature and branch structure.
2. Read each of the 8 hand-rolled RunFuncs to catalog exactly which branch each one varies: read error, per-index `set-hook -gu` (unset) error, or a `CommandError` surfaced through the read branch.
3. Extend the shared helper with optional fault-injection maps:
   - `readErrFor map[string]error` — when set and the key matches `args[2]` on a `show-hooks -g <event>` read, return that error (this is the channel for injecting a `CommandError`).
   - `unsetErrFor map[string]error` — when set and the key matches on a `set-hook -gu` call, return that error.
   - Preserve the existing `setHookErrFor` semantics and the no-arg-global-read `t.Fatalf` guard unchanged. If extending the existing signature would churn too many call sites, add one sibling builder that shares the inner closure and have `perEventDispatch` delegate to it with nil fault maps.
4. Replace each of the 8 bespoke RunFuncs (`hooks_register_test.go:546-564`, `:619-642`; `hooks_register_warn_test.go:88-96`, `:129-145`, `:184-202`, `:246-258`; `hooks_migration_test.go:329-349`, `:446-458`) with a one-line call into the helper passing the appropriate fault map. Remove the now-redundant copy-pasted fatal-guard strings and the `hooks_migration_test.go:319` "wraps the shared splitter directly..." comment if it no longer applies.
5. Run the full `internal/tmux` suite and confirm each migrated test still exercises the same fault path (read error / per-index unset error / `CommandError`) and still asserts the same observable behavior.
**Acceptance Criteria**:
- The per-event read/dispatch skeleton, including the no-arg-global-read fatal guard, is declared in exactly one helper.
- All 8 previously-bespoke RunFuncs are one-line calls into that helper.
- Each migrated test still injects and exercises its original fault (read error, per-index unset error, or `CommandError`) and asserts the same observable outcome as before.
- No verbatim copies of the `"convergence engine must read per-event..."` guard string remain outside the shared helper.
- The full `internal/tmux` suite passes.
**Tests**:
- The migrated tests themselves are the coverage: TestRegisterPortalHooks read-failure, per-index-unset-failure, and CommandError variants across the three files must continue to pass with identical assertions after collapsing onto the shared helper.
- No new test is required beyond confirming the helper's fault-injection branches are reached by the migrated call sites.

## Task 3: De-duplicate hook command-body and fingerprint test literals within the tmux_test package
status: pending
severity: low
sources: duplication

**Problem**: `expectedNotifyCommand` (`internal/tmux/hooks_register_test.go:31`) and `notifyCommandBody` (`internal/tmux/hooks_register_realtmux_test.go:54`) are byte-for-byte identical declarations of the notify run-shell body. Both live in package `tmux_test`. This is NOT the production/external-test import-boundary mirroring the topic note exempts (that exemption covers test copies of unexported PRODUCTION constants) — these are two test-side copies of the same string in the same package, authored independently in two files. The same shape applies to `notifyFingerprint`/`commitNowFingerprint`/`signalHydrateFingerprint` (`hooks_register_realtmux_test.go:41,45,49`), which restate substrings already embedded in the same package's full-body constants.
**Solution**: Keep a single test-package copy of each command-body literal and fingerprint substring (the `expected*` set in `hooks_register_test.go` is the natural home) and have `hooks_register_realtmux_test.go` reference those instead of redeclaring them.
**Outcome**: Each test-side command-body literal and fingerprint substring is declared once in the `tmux_test` package. The real-tmux test file references the shared declarations rather than maintaining parallel copies that can drift.
**Do**:
1. Read `hooks_register_test.go:27-46` (the `expected*` command-body constants) and `hooks_register_realtmux_test.go:41-54` (`notifyFingerprint`, `commitNowFingerprint`, `signalHydrateFingerprint`, `notifyCommandBody`) to confirm the exact overlaps.
2. Treat the `expected*` set in `hooks_register_test.go` as the single home. Remove `notifyCommandBody` (`realtmux_test.go:54`) and replace its usages with `expectedNotifyCommand`.
3. For the fingerprint substrings, either reference the substrings already embedded in the shared full-body constants or hoist a single shared fingerprint declaration into `hooks_register_test.go`; remove the redundant `realtmux_test.go:41,45,49` declarations and repoint their usages.
4. DO NOT touch the deliberate below-the-import-boundary mirroring of the production unexported constants — that mirroring is exempt per the topic note and must stay as-is.
5. Run the full `internal/tmux` suite including the real-tmux integration tests.
**Acceptance Criteria**:
- The notify command body has exactly one test-package declaration; `realtmux_test.go` references it.
- Each fingerprint substring is declared once in the test package (or sourced from the shared full-body constant) rather than independently restated.
- The production unexported-constant mirroring is untouched.
- The full `internal/tmux` suite, including real-tmux integration tests, passes.
**Tests**:
- The existing real-tmux integration tests in `hooks_register_realtmux_test.go` must continue to pass against the now-shared literals, confirming the referenced constants carry the same values the local copies did.

## Task 4: Fold recordingMigrationLogger onto the pre-existing recordingSlogHandler base (additive to new code only)
status: pending
severity: low
sources: duplication

**Problem**: `recordingMigrationLogger` (new, `internal/tmux/hooks_register_test.go:811-871`) is a near-duplicate of the pre-existing `recordingSlogHandler` (`internal/tmux/portal_saver_test.go:2629`, which the new `hooks_register_warn_test.go` tests already consume). Both are `slog.Handler` implementations in the same `tmux_test` package sharing identical shared/bound/owner + `WithAttrs`/`WithGroup` capture scaffolding; the only real difference is `recordingMigrationLogger` eagerly projects `component`/`reaped` into typed fields while `recordingSlogHandler` stores raw records. The implementation already uses `recordingSlogHandler` in `hooks_register_warn_test.go` and then introduced a parallel recorder for the sibling files.
**Solution**: Have `recordingMigrationLogger` embed or wrap the existing `recordingSlogHandler` (capture raw records once, expose `infos`/`reaped`/`warns` as thin accessors that filter/project the stored records) so the `WithAttrs`/`WithGroup`/owner scaffolding lives in exactly one type. Treat `recordingSlogHandler` as the shared base; the change is additive to the new code only.
**Outcome**: The `slog.Handler` capture scaffolding (`WithAttrs`/`WithGroup`/owner/shared) exists in exactly one type in the package. `recordingMigrationLogger` becomes a thin projection layer over the shared base, and the migration tests still see the same typed `component`/`reaped` accessors they do today.
**Do**:
1. Read `recordingMigrationLogger` (`hooks_register_test.go:811-871`) and `recordingSlogHandler` (`portal_saver_test.go:2629` onward) to map which scaffolding overlaps and which projection fields (`component`, `reaped`) are unique to the migration recorder.
2. Re-implement `recordingMigrationLogger` to embed or wrap `recordingSlogHandler`: let the base capture raw records, and expose `infos`/`reaped`/`warns` as thin accessors that filter/project the base's stored records.
3. Keep ALL changes additive to the new code (`hooks_register_test.go`). DO NOT edit `portal_saver_test.go` / `recordingSlogHandler` — that base is pre-existing out-of-scope code; only consume it, do not modify it.
4. Confirm the migration/warn tests that consume `recordingMigrationLogger` still read the same typed accessors and observe identical captured output.
5. Run the full `internal/tmux` suite.
**Acceptance Criteria**:
- The `WithAttrs`/`WithGroup`/owner/shared capture scaffolding is declared once (in `recordingSlogHandler`); `recordingMigrationLogger` embeds or wraps it.
- `portal_saver_test.go` / `recordingSlogHandler` are unchanged — the change is additive to the new code only.
- The typed `component`/`reaped` projections remain available to the consuming tests with identical behavior.
- The full `internal/tmux` suite passes.
**Tests**:
- The existing tests consuming `recordingMigrationLogger` (the migration/warn-path tests) must continue to pass with identical assertions, confirming the wrapped base captures the same records the standalone recorder did.

## Task 5: Fix the stale migrateHydrationHooks comment in reboot_roundtrip_test.go
status: pending
severity: low
sources: standards

**Problem**: The comment at `cmd/bootstrap/reboot_roundtrip_test.go:1202-1203` reads "HookRegistrar runs migrateHydrationHooks (Task 1-2) and registers the new `--`-separated signalHydrateCommand (Task 1-1) end-to-end." The spec's Migration-Helper Consolidation section explicitly deletes `migrateHydrationHooks` and folds its behavior into the unified per-event convergence path; the helper no longer exists anywhere in the codebase (verified by grep — only test comments reference it). The comment misdescribes the code path the test now exercises (`RegisterPortalHooks`' ensure-exactly-one convergence). The test logic itself is correct and verifies the right end-to-end behavior (`verifyHydrationHookEntries` asserts exactly one `--`-separated entry per hydration event); only the explanatory comment drifted. Other test files (`hooks_migration_test.go`, `hooks_register_warn_test.go`) already correctly note the helper was deleted; this one comment was missed.
**Solution**: Update the comment to describe the unified convergence path, e.g. "HookRegistrar runs RegisterPortalHooks' per-event ensure-exactly-one convergence, which evicts any stale un-separated signal-hydrate body and registers the `--`-separated signalHydrateCommand end-to-end."
**Outcome**: The comment accurately describes the per-event convergence path the test exercises and no longer references the deleted `migrateHydrationHooks` helper. The test body is unchanged.
**Do**:
1. Read `cmd/bootstrap/reboot_roundtrip_test.go:1195-1215` to confirm the stale comment and the surrounding `verifyHydrationHookEntries` assertions it describes.
2. Rewrite the comment at lines 1202-1203 to describe `RegisterPortalHooks`' per-event ensure-exactly-one convergence (eviction of any stale un-separated signal-hydrate body + registration of the `--`-separated `signalHydrateCommand`), removing the `migrateHydrationHooks`/Task-1-2 reference.
3. Make NO behavioral change to the test logic — comment-only edit.
4. Grep the codebase for any remaining `migrateHydrationHooks` references to confirm this is the last stale one; if others surface, note them but do not expand scope beyond comment corrections.
5. Run the `cmd/bootstrap` test package to confirm nothing broke.
**Acceptance Criteria**:
- The comment at `reboot_roundtrip_test.go:1202-1203` describes the unified `RegisterPortalHooks` convergence path and no longer references the deleted `migrateHydrationHooks`.
- No test logic changed.
- No remaining `migrateHydrationHooks` references exist outside intentional historical context.
- The `cmd/bootstrap` test package passes.
**Tests**:
- No new test required — this is a documentation-only correction. The existing `reboot_roundtrip_test.go` end-to-end assertions continue to pass unchanged, confirming the corrected comment describes the path the test still exercises.
