# Implementation Review: CLI Verb Surface Redesign

**Plan**: cli-verb-surface-redesign
**QA Verdict**: Request Changes

## Summary

This is a strong, disciplined implementation. All 51 plan tasks across 12 phases (6 feature phases + 6 analysis cycles) were independently verified against their acceptance criteria and the specification; **50 are fully complete with adequate, well-balanced tests and no blocking issues**. The governing principle (split the surface by outcome), both axioms (absorb/net-N; attach-vs-mint), the domain-pin contract, the multi-target burst, the `doctor`/`uninstall` reshuffle, the `attach`/`spawn` retirement, the `hooks`→`hook` rename, `state` hiding, and tab completion are all implemented as specified, and the six analysis cycles left the codebase notably clean (single-sourced error strings, single-sourced governed emissions, byte-compat refactors, retargeted stale comments).

**One task falls short of its own acceptance criteria.** Task 8-2 removed the divergent silent-first-match glob branch from `QueryResolver.Resolve` (correct and complete), but the identical branch survives in the two single-pin resolve paths, `ResolveSessionPin` and `ResolveAliasPin`. Those branches are production-dead *today* — reachable only if the `os.Args`-based `isMultiTarget` diversion assumption breaks — so nothing is broken for users right now. But acceptance criteria 8-2.1 ("multi-match session glob never collapses to `matches[0]`") and 8-2.5 ("an `os.Args`-assumption break can no longer silently fork glob semantics") are not fully met, and two unit tests (`query_test.go:593` / `:896`) actively lock in the forbidden first-match behaviour. Three independent verifiers (Reports 8-2, 2-1, 2-3) converged on this. It warrants a small, scoped follow-up rather than reopening the `Resolve` change; if the maintainer judges the argv assumption unbreakable, it is a defensible ship-anyway.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across the full redesigned surface. Spot-checked deviations were all deliberate and sound:
- `doctor`'s daemon probe reads `ReadPIDFile`/`IsProcessAlive`/`ReadVersionFile` directly rather than the (now-deleted) `CollectStatus`/`StatusReport` — a later analysis-cycle refinement (Task 8-1) that the spec's read-only/low-cost intent endorses.
- `doctor` reads `sessions.json` via `state.ReadIndex` (absent → healthy pass, corrupt → fail) rather than a literal `HasLastSave` boolean — spec-consistent.
- The exact byte-pinned strings the spec calls out are preserved: the single-target miss string (`nothing resolved for '%s' — try -f %s`, U+2014 em-dash, single-sourced in `singleMissError`), the Projects-picker banner (`Pick a project to run`), and the command-on-attach usage error (single-sourced in `commandAttachOnlyMessage`).

The one gap is the 8-2 acceptance shortfall described above, which is a partial requirement miss (the `Resolve` fix is complete; the sibling pin paths were not swept).

### Plan Completion

- [x] Phase 1–12 acceptance criteria met (Phase 8 partial — see Required Changes)
- [x] All 51 tasks completed and verified
- [x] No scope creep — retained `internal/spawn` service, `spawn` log component, `@portal-spawn-*` markers, and `SplitNetN` are all explicitly in-scope-to-keep; relocations (SessionValidator, host-terminal seams, config/prefs loaders, AllPaneLister) preserved consumers rather than deleting

### Code Quality

No issues found beyond the 8-2 finding. Notable positives: the four copy-paste domain-pin arms collapsed to one table-driven helper (7-2); the two governed two-site emissions (`resolve` INFO line, `process:exec` marker) each single-sourced (7-8); the argv-order scan guarded against value-flag drift by a live `VisitAll` test (7-3). Residual DRY nits (pin set enumerated in three places; os.Stat stale-classification duplicated between doctor checks and the mutating pruners) are captured as non-blocking Ideas/Quick-fixes below.

### Test Quality

Tests adequately verify requirements throughout, with balanced coverage (verifiers flagged no over-testing beyond a handful of duplicate rows). The exception is Task 8-2: `query_test.go:593` and `:896` assert and enshrine the divergent first-match glob collapse that acceptance 8-2.1 forbids — they are green precisely because the sibling branches were left in place. These must be retargeted alongside the code fix. A scattering of test-symmetry gaps (missing `--` dash-dash spellings, a missing pin-miss test, missing engine-level and Execute-level cases) are non-blocking Quick-fixes.

### Required Changes

1. **Sweep the surviving divergent glob branches out of the single-pin resolve paths** (blocking; corroborated by Reports 8-2, 2-1, 2-3).
   - `internal/resolver/query.go:297` (`ResolveSessionPin`) and `internal/resolver/query.go:353` (`ResolveAliasPin`) still collapse a multi-match glob to `matches[0]` (silent first-match) — the exact branch Task 8-2 removed from `QueryResolver.Resolve`, left in the two pin variants. Production-dead only via the `os.Args`-based `isMultiTarget` gate; a broken argv assumption silently forks glob semantics for `-s 'api-*'` / `-a 'workflow-*'`. Route both through the shared `expandSessionGlobAll` / all-match primitive, or make a single-pin glob a loud error mirroring `Resolve`'s loud-miss.
   - Retarget the two tests that lock in the forbidden behaviour — `internal/resolver/query_test.go:593` ("glob expansion attaches the first match") and `:896` ("key glob multi-match mints the first sorted match") — to a loud miss/error or all-match expansion, together with the code fix, so the suite stops enshrining the divergence. The stale `ResolveSessionPin` doc comment (query.go:281-288) should drop its "first match at single-target arity" clause.

## Recommendations

### Do now

1. Stale doc/comment references (doc-staleness sweep):
   - `cmd/open_targets.go:27` — the `openTargetPins` doc block omits the "no bundled `-sf` value pins" contract; add a one-line note that bundled value shorthands are deliberately classified unknown-and-skipped (Report 3-2)
   - `cmd/open_burst_run.go:49-50` — the `OpenBurstDeps.Logger` comment "(Task 3-8 adds the batch summary)" is a now-stale forward-reference; reword to current behaviour (Report 3-6)
   - `cmd/reattach_integration_test.go:3` — header comment reads "Phase 5 task 5-10"; the task is 5-1 (Report 5-1)
   - `internal/spawn/recipe_test.go:170,174` — `TestRenderCommandString` uses retired `"attach"`/`"--spawn-ack"` sample tokens; replace with `open`/`--session`/`--ack` and update the coupled `want` string (Report 5-1)
   - `cmd/bare_root_test.go:26` — cobra mechanism comment cites `command.go:983`; the loop is at `:984` in cobra v1.10.2 (Report 6-5)
   - `CLAUDE.md:39` — the `doctor` entry says it "subsumes the retired `state status`" but omits its other two spec roles (replaces `clean` for repairs, folds in `spawn --detect`'s host-terminal line); append a short clause (Report 7-6)
2. `cmd/uninstall_test.go:109` — add an assertion that no `kill-session` call targets `_portal-bootstrap` (only `tmux.PortalSaverName`), making the "leaves the load-bearing anchor running" criterion explicit rather than implicit (Report 4-6)

### Quick-fixes

3. `cmd/open.go:208-209` — the inline `anyPin := cmd.Flags().Changed("session") || …` duplicates `anyOpenDomainPin(cmd)` (cmd/root.go:314-317); the pin set is now enumerated in three places (this inline, `anyOpenDomainPin`, and the `pinDispatch` table). Replace the inline with a call to `anyOpenDomainPin(cmd)` (Report 1-5)
4. Command `--` (dash-dash) spelling test parity:
   - `cmd/open_test.go:2853` — add a `-f <text> -- <cmd>` case alongside the `-e` case in the filter-threads-command test (Report 2-5)
   - `cmd/open_test.go:325` — add an attach-guard case using `open dev -- claude` alongside the `-e` case (Report 2-6)
5. `cmd/open_test.go` — add `TestOpenCommand_PathPin_Miss_HardFailsNoPicker` mirroring the `-s` pin miss test, closing the pin test-symmetry gap (Report 2-2)
6. `internal/resolver/query_test.go:97-116` — merge the two `Resolve` table rows exercising the identical zoxide-miss → `MissResult` path (Report 1-2)
7. `internal/resolver/query.go:252,348` + `internal/resolver/glob.go:27` — `MatchSessions` is reused to glob-match the alias-key namespace; rename to a domain-agnostic `MatchGlob` (mechanical cross-file rename, no logic change) (Report 2-3)
8. `open`-target argv-scan test hardening:
   - `cmd/open_targets_guard_test.go:82` — add a reverse assertion that every non-excluded `openTargetPins` key maps to a live `openCmd` flag, catching a stale entry after a future flag removal (Report 3-2)
   - `cmd/open_targets_test.go:147` — add a case with an excluded flag's value between two positionals (e.g. `["blog","-e","claude","api"]`) to pin mid-list value consumption (Report 3-2)
9. `cmd/open_surfaces_test.go` — add an engine-level subtest for a `Domain:"session"` (`-s`) surface (Report 3-3)
10. `doctor` test coverage:
    - `cmd/doctor_test.go` — cover the two uncovered `checkStateDirSane` fail branches (existing-but-non-directory; unreadable stat) at cmd/doctor.go:612-618 (Report 4-1)
    - `cmd/doctor_test.go` — add one Execute-level test seeding a genuine stale hook/project over an otherwise-healthy runtime and asserting plain `portal doctor` returns `ErrDoctorUnhealthy` (Report 4-3)
    - `cmd/doctor.go:642,438,488` — pluralise count diagnostic copy ("1 sessions" → "1 session"); also touches asserting test strings (Report 4-1)
11. `cmd/state_daemon_run_test.go` — add a `tick()`-idle-branch test for the project prune mirroring `TestDaemonTick_RunsHookCleanupOnIdleTick`, closing the asymmetric coverage gap (Report 4-8)
12. `cmd/spawn_seams.go:59` — `buildProductionSpawnSeams` calls `log.For("spawn")` a second time for the bundle's `Logger`; reference the package-level `spawnLogger` to single-source the cmd-layer binding (Report 5-2)
13. `cmd/hooks_test.go:825-846` — the "machine-generated hooks set via alias" subtest byte-duplicates the happy-path set test; fold its distinct acceptance intent into a comment or drop the duplicate assertion (Report 6-1)
14. completion coverage/redundancy:
    - `cmd/completion_test.go` — add a test asserting `open` flag completion excludes `--ack` and top-level completion excludes `state`, verifying the "hidden never appear in completion" edge directly rather than transitively (Report 6-3)
    - `cmd/completion.go:64-67` — `completionAliasKeys` calls `store.Load()` a second time after `loadAliasStore` already loaded it; drop the redundant re-read and return `store.Keys()` directly (Report 6-4)
15. `cmd/open_test.go:2102` — optionally add a bare `domain=alias` (or `domain=path`) resolve-line assertion to round out the resolve-log coverage (Report 1-4)
16. `internal/state/count_panes_test.go` / `internal/state/index_reader.go:60` — `CountPanes` lives in `index_reader.go` but its tests sit in a standalone `count_panes_test.go`; add `count_panes.go` or fold the tests into `index_reader_test.go` (Report 8-1)

### Ideas

17. `cmd/open.go:208-209` — derive `anyPin` from a single shared pin-name list also feeding the `pinDispatch` table, so a future fifth pin can't be silently missed by the exclusivity guard (decide whether the churn earns its keep for a 4→5 change) (Report 2-5)
18. Extract-shared-helper (duplication drift risk):
    - `cmd/doctor.go:465-491` (`checkStaleProjects`) & `internal/project/store.go:202-213` (`CleanStale`), plus `cmd/doctor.go:446-458` mirroring `hooks.Store.CleanStale` — extract a shared read-only staleness classifier consumed by both the check and the pruner to eliminate the divergence class (Report 4-3)
    - `cmd/state_daemon.go:449-501` — `maybeRunHookCleanup` and `maybeRunProjectCleanup` share an identical throttled-gate skeleton; consider a shared helper (weigh premature-abstraction for two call sites) (Report 4-8)
19. Governed-invariant drift guards:
    - `cmd/open.go:412,454` — a source-walking guard (in the spirit of the `internal/log` single-owner / keymap-dispatch guards) asserting the `resolve` INFO line and the `process:exec` marker each appear at exactly one call site (Report 7-8)
    - `cmd/doctor.go:502` — a guard test/lint failing if routine `doctor` regains a `filepath.WalkDir` over the state dir or opens `portal.log` (Report 8-1)
20. `cmd/uninstall.go:136` — `killSaver` relies on `HasSession`, which collapses all errors (including transient tmux faults) to `false`, so a transient probe failure is silently treated as "saver absent" while the message still claims removal; consider the discriminating `HasSessionProbe` (already at internal/tmux/tmux.go:165) and folding a transient fault into the joined error (Report 4-6)
21. `cmd/open_burst_run.go:222,226,231` — `LogBatchSummary` is emitted with `triggerAttached=true` before `connectTrigger` runs; on the rare trigger-own-connect-failure path `portal.log` reports `opened N/N` counting a trigger that did not attach. Consider a corrective WARN from the connect-error path to keep the durable log honest (Report 3-8)
22. `cmd/doctor_test.go:588` — the "_portal-bootstrap present but saver absent" edge is only implicitly covered by the "absent fails" subtest; name/comment the `_portal-bootstrap`-independence intent or add a one-line subtest (Report 4-2)
23. `cmd/retired_surface_test.go:123-135` — `TestRetiredSurface_AbsentFromCompletion` is coupled to the legacy `GenBashCompletion` (v1) emission form and would become a permanent no-op pass if completion migrates to V2; consider hardening it or accepting it as intentionally narrow given the child-registration test's redundancy (Report 5-3)
