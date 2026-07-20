---
scope: cli-verb-surface-redesign review remediation — resolver glob-pin divergence (blocking) plus non-blocking polish
cycle: 1
source: review
total_proposed: 6
gate_mode: auto
---
# Review Tasks: CLI Verb Surface Redesign (Cycle 1)

## Task 1: Sweep the surviving divergent silent-first-match glob branches out of the single-pin resolve paths
status: approved
severity: high
sources: report-8-2, report-2-1, report-2-3

**Problem**: Task 8-2 removed the silent-first-match glob branch (`if HasGlobMeta(query) { … return matches[0] }`) from `QueryResolver.Resolve`, but the identical branch survives unchanged in both single-pin paths: `ResolveSessionPin` (internal/resolver/query.go:295-298, collapse at :297) and `ResolveAliasPin` (query.go:347-355, collapse at :353). Both collapse a multi-match glob to `matches[0]`. They are production-dead only because the `os.Args`-based `isMultiTarget` gate diverts glob-bearing `-s`/`-a` values to the burst; if that argv assumption breaks (`openOwnArgs()` returns nil), a glob-bearing `-s 'api-*'` / `-a 'workflow-*'` silently forks to first-match. This violates 8-2 acceptance criterion 1 ("multi-match session glob never collapses to `matches[0]`") and 5 ("an os.Args-assumption break can no longer silently fork glob semantics"). Two unit tests actively enshrine the forbidden behaviour — query_test.go:593 ("glob expansion attaches the first match") and :896 ("key glob multi-match mints the first sorted match") — green precisely because the branches were left in place. The `ResolveSessionPin` doc comment (query.go:281-288) still documents a "first match at single-target arity" clause.
**Solution**: Eliminate the silent first-match from both single-pin paths symmetrically. Either (a) route the `ResolveSessionPin`/`ResolveAliasPin` glob branches through the shared all-match primitive (`expandSessionGlobAll` / the all-match helper feeding `ResolveSessionPinAll`/`ResolveAliasPinAll`), or (b) make a glob value reaching a single-pin an explicit loud error mirroring `Resolve`'s loud-miss. Do NOT reopen the already-correct `QueryResolver.Resolve` change.
**Outcome**: No single-pin resolve path can silently collapse a multi-match glob to the first match on any code path (including an os.Args-assumption break); the suite no longer enshrines the divergence; the doc comment reflects the exact-only (or all-match) contract.
**Do**:
1. In internal/resolver/query.go, remove the `if HasGlobMeta(query)` first-match block in `ResolveSessionPin` (:295-298) and the equivalent block in `ResolveAliasPin` (:347-355). Replace with either the shared all-match primitive or an explicit loud error — same approach in both.
2. Update the `ResolveSessionPin` doc comment (:281-288) to drop the "first match at single-target arity" clause and describe the new contract; adjust `ResolveAliasPin`'s doc similarly if it carries the same implication.
3. Retarget internal/resolver/query_test.go:593 and :896 to assert the new loud-miss/error (or all-match) behaviour instead of the first-match collapse.
4. Run the resolver package tests plus cmd/open tests to confirm nothing else depended on the removed branch.
**Acceptance Criteria**:
- Neither `ResolveSessionPin` nor `ResolveAliasPin` can return a `matches[0]` collapse for a multi-match glob.
- A glob-bearing `-s`/`-a` value reaching the single-pin path yields an all-match expansion or an explicit loud error (never silent first-match), independent of the `isMultiTarget`/os.Args gate.
- query_test.go:593 and :896 assert the new behaviour and no longer enshrine first-match.
- The `ResolveSessionPin` doc comment no longer claims first-match-at-single-target-arity behaviour.
- `QueryResolver.Resolve` is unchanged.
**Tests**:
- Retargeted query_test.go:593 / :896.
- A resolver-level test that a multi-match glob under `ResolveSessionPin` and `ResolveAliasPin` yields the loud miss/error (or full match set), mirroring `TestQueryResolver_Resolve_GlobFallsThroughToMiss`.

## Task 2: Single-source the domain-pin set so the exclusivity guard cannot miss a future pin
status: approved
severity: medium
sources: report-1-5, report-2-5

**Problem**: The open domain-pin flag set is enumerated in three places: the inline `anyPin := cmd.Flags().Changed("session") || …` at cmd/open.go:208-209, `anyOpenDomainPin(cmd)` at cmd/root.go:314-317, and the `pinDispatch` table (cmd/open.go:259-272). The inline at :208-209 duplicates `anyOpenDomainPin` verbatim, and because the pin names are hand-listed in multiple spots a future fifth pin can be added to `pinDispatch` yet silently missed by the exclusivity/`anyPin` guard.
**Solution**: Replace the inline `anyPin` at open.go:208-209 with a call to `anyOpenDomainPin(cmd)`, and derive both `anyOpenDomainPin` and the `pinDispatch` key set from a single shared pin-name list so a new pin is added in exactly one place.
**Outcome**: The pin set lives in one source consumed by the guard and the dispatch table, so a future pin cannot be silently omitted from the exclusivity guard.
**Do**:
1. Introduce a single canonical list of open domain-pin flag names (a package-level slice) if one does not already exist.
2. Rewrite `anyOpenDomainPin` (cmd/root.go:314-317) to iterate that list.
3. Replace the inline `anyPin` expression at cmd/open.go:208-209 with a call to `anyOpenDomainPin(cmd)`.
4. Where practical, derive the `pinDispatch` table keys (or a validation over them) from the same list so the two cannot drift.
**Acceptance Criteria**:
- cmd/open.go:208-209 no longer hand-lists pin flags; it calls `anyOpenDomainPin`.
- The pin-name set is declared once and consumed by both the guard and the dispatch table.
- The exclusivity guard's behaviour is unchanged for the existing four pins.
**Tests**:
- Existing pin-exclusivity tests still pass.
- A guard/unit test asserting every `pinDispatch` key is present in the shared pin-name list (drift guard), matching the existing guard-test style.

## Task 3: Fix two paths that report success while an operation silently failed
status: approved
severity: medium
sources: report-4-6, report-3-8

**Problem**: Two independent paths emit success-shaped output on a failure branch. (1) cmd/uninstall.go:136 `killSaver` relies on `HasSession`, which collapses all errors — including transient tmux faults — to `false`, so a transient probe failure is treated as "saver absent" while the uninstall message still claims removal. (2) cmd/open_burst_run.go:222-231 emits `LogBatchSummary` with `triggerAttached=true` before `connectTrigger` runs; on the rare trigger-own-connect-failure path the durable `portal.log` reports `opened N/N` counting a trigger that never attached.
**Solution**: (1) In `killSaver`, use the discriminating `HasSessionProbe` (internal/tmux/tmux.go:165) instead of `HasSession` so a transient probe fault is folded into the joined error rather than silently reported as "removed". (2) Emit a corrective WARN from the `connectTrigger` error path (or defer the `triggerAttached=true` count until after a successful connect) so the durable log does not claim an attach that failed.
**Outcome**: A transient tmux fault during uninstall surfaces as an error rather than a false "removed"; a trigger-connect failure leaves an honest durable log instead of an inflated `opened N/N`.
**Do**:
1. cmd/uninstall.go:136 — swap `HasSession` for `HasSessionProbe`; on a transient/error result, join the fault into the returned error and do not claim saver removal.
2. cmd/open_burst_run.go:222-231 — on the `connectTrigger` failure path, emit a corrective WARN under the `spawn` component (or restructure so the batch summary's trigger count reflects the actual attach outcome).
3. Add/adjust tests for both paths.
**Acceptance Criteria**:
- A transient tmux probe fault in `killSaver` produces an error, not a silent "saver absent" + "removed" message.
- On trigger-connect failure, `portal.log` does not report an `opened` count that includes the un-attached trigger, or a corrective WARN is emitted noting the trigger did not attach.
- Existing uninstall and open-burst tests still pass.
**Tests**:
- A `killSaver` test injecting a transient `HasSessionProbe` fault, asserting the joined error and no false removal claim.
- An open-burst test forcing `connectTrigger` failure, asserting the corrective WARN / honest count.

## Task 4: Doctor — correct count copy and close the test-coverage gaps
status: approved
severity: low
sources: report-4-1, report-4-3

**Problem**: `portal doctor` count diagnostics render ungrammatical copy ("1 sessions") at cmd/doctor.go:642, :438, :488. Two `checkStateDirSane` fail branches (existing-but-non-directory; unreadable stat, cmd/doctor.go:612-618) are uncovered, and there is no Execute-level test that a genuine stale hook/project over an otherwise-healthy runtime makes plain `portal doctor` return `ErrDoctorUnhealthy`.
**Solution**: Pluralise the count diagnostic copy correctly (singular when count == 1), and add the two missing `checkStateDirSane` fail-branch tests plus one Execute-level stale-entry → `ErrDoctorUnhealthy` test.
**Outcome**: Doctor output reads grammatically ("1 session"), and the previously-uncovered fail branches and the unhealthy Execute path are pinned by tests.
**Do**:
1. cmd/doctor.go:642,438,488 — pluralise the count copy (singular when count == 1); update the asserting test strings in cmd/doctor_test.go.
2. cmd/doctor_test.go — add subtests covering `checkStateDirSane` for (a) state-dir path exists but is not a directory, (b) stat is unreadable (cmd/doctor.go:612-618).
3. cmd/doctor_test.go — add one Execute-level test seeding a genuine stale hook/project over an otherwise-healthy runtime and asserting plain `portal doctor` returns `ErrDoctorUnhealthy`.
**Acceptance Criteria**:
- Count diagnostics render singular/plural correctly.
- Both `checkStateDirSane` fail branches are exercised by tests.
- An Execute-level test asserts `ErrDoctorUnhealthy` on a seeded stale hook/project.
**Tests**:
- The three additions above.

## Task 5: Close the test-coverage parity gaps across the redesigned surface
status: approved
severity: low
sources: report-2-5, report-2-6, report-2-2, report-4-8, report-3-3, report-3-2, report-6-3

**Problem**: QA flagged a cluster of symmetric test gaps where one spelling/branch is covered but its sibling is not: missing `--` (dash-dash) command spellings alongside the `-e` cases (cmd/open_test.go:2853 filter-threads-command, :325 attach-guard); no `TestOpenCommand_PathPin_Miss_HardFailsNoPicker` mirroring the `-s` pin-miss test; no `tick()`-idle-branch test for the project prune mirroring the hook-cleanup one (cmd/state_daemon_run_test.go); no engine-level subtest for a `Domain:"session"` (`-s`) surface (cmd/open_surfaces_test.go); argv-scan gaps (open_targets_guard_test.go:82 reverse mapping, open_targets_test.go:147 mid-list value between positionals); and no direct test that `open` flag completion excludes `--ack` and top-level completion excludes `state` (cmd/completion_test.go).
**Solution**: Add each missing symmetric test. These are independent test-only additions with no production-code change.
**Outcome**: Each covered branch/spelling has its documented sibling covered, removing the asymmetries QA identified.
**Do**:
1. cmd/open_test.go:2853 — add a `-f <text> -- <cmd>` case alongside the `-e` filter-threads-command case.
2. cmd/open_test.go:325 — add an attach-guard case using `open dev -- claude` alongside the `-e` case.
3. cmd/open_test.go — add `TestOpenCommand_PathPin_Miss_HardFailsNoPicker` mirroring the `-s` pin-miss test.
4. cmd/state_daemon_run_test.go — add a `tick()`-idle-branch test for the project prune mirroring `TestDaemonTick_RunsHookCleanupOnIdleTick`.
5. cmd/open_surfaces_test.go — add an engine-level subtest for a `Domain:"session"` (`-s`) surface.
6. cmd/open_targets_guard_test.go:82 — add a reverse assertion that every non-excluded `openTargetPins` key maps to a live `openCmd` flag; cmd/open_targets_test.go:147 — add a case with an excluded flag's value between two positionals (e.g. `["blog","-e","claude","api"]`).
7. cmd/completion_test.go — add a test asserting `open` flag completion excludes `--ack` and top-level completion excludes `state`.
**Acceptance Criteria**:
- Each of the seven sub-items above has a passing test.
- No production behaviour changes.
**Tests**:
- All additions listed under Do.

## Task 6: Small DRY / legibility cleanups (redundant and misleading code)
status: approved
severity: low
sources: report-2-3, report-5-2, report-6-4

**Problem**: Three low-churn maintainability nits: (1) `MatchSessions` (internal/resolver/glob.go:27, called at query.go:252,348) is reused to glob-match the alias-key namespace — a session-named helper applied to a non-session domain. (2) cmd/spawn_seams.go:59 `buildProductionSpawnSeams` calls `log.For("spawn")` a second time for the bundle's `Logger` instead of referencing the package-level `spawnLogger`. (3) cmd/completion.go:64-67 `completionAliasKeys` calls `store.Load()` a second time after `loadAliasStore` already loaded it.
**Solution**: Rename `MatchSessions` → `MatchGlob` across its definition and call sites (mechanical, no logic change); reference the package-level `spawnLogger` in `buildProductionSpawnSeams`; drop the redundant `store.Load()` in `completionAliasKeys` and return `store.Keys()` directly.
**Outcome**: The glob helper reads domain-agnostically, the cmd-layer spawn logger is single-sourced, and completion does not double-load the alias store.
**Do**:
1. internal/resolver/glob.go:27 + call sites (query.go:252,348 and any others) — rename `MatchSessions` to `MatchGlob`. Mechanical; no behaviour change. Independent of Task 1 — adapt to whichever call sites remain if both tasks are applied.
2. cmd/spawn_seams.go:59 — reference the package-level `spawnLogger` instead of a second `log.For("spawn")`.
3. cmd/completion.go:64-67 — remove the redundant `store.Load()` and return `store.Keys()` directly.
**Acceptance Criteria**:
- No `MatchSessions` identifier remains; `MatchGlob` compiles with all call sites updated.
- `buildProductionSpawnSeams` uses `spawnLogger`.
- `completionAliasKeys` loads the alias store once.
- No behaviour change; existing tests pass.
**Tests**:
- Existing resolver/spawn/completion tests pass unchanged (rename + single-source are behaviour-preserving).
