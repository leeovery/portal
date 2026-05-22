# Implementation Review: Killed Session Resurrects Within Tick Window

**Plan**: killed-session-resurrects-within-tick-window
**QA Verdict**: Comments Only

## Summary

All 17 plan tasks across three phases (1 implementation, 2 analysis cycles) are implemented, tested, and conform to spec. The synchronous `portal state commit-now` path eliminates the kill→resurrection race window with a single tmux-side seam (`session-closed`) covering every kill path; the cycle-1 / cycle-2 cleanups (state-package promotion of `TouchSaveRequested`, presume-set on `IsRestoring` query failure, struct-shape `CommitNowDeps`, descriptive sentinels, helper consolidations) tighten the design without changing observable behaviour. Acceptance criteria 1-15 of the Phase 1 acceptance list are satisfied. No blocking issues. Twenty non-blocking notes — mostly minor doc/cosmetic items plus one minor semantic note on `failCommitNow`'s wrap (`%w: %s: %v` doesn't place the cause in the error chain, only in the message text — doc-comment mildly inaccurate).

## QA Verification

### Specification Compliance

Implementation aligns with the specification. Key spec-mandated elements verified:

- `portal state commit-now` is a hidden sibling subcommand; uses `state.ReadIndex` → `state.CaptureStructure(client, nil, &prev)` → `state.Commit(dir, idx, false, logger)` exactly as the spec § Fix Approach prescribes.
- `@portal-restoring` short-circuit fires before any structural work, mirrors daemon's `tick()` entry guard, touches `save.requested` on exit per § save.requested Discipline.
- `IsRestoring` query failure presumes marker set (spec § @portal-restoring Defence risk priority — protect in-flight restore over prompt kill removal).
- Failure-path discipline (`failCommitNow`) emits ERROR + best-effort touch + non-zero exit + no panic; touch-on-failure failures preserve original non-zero exit (§ save.requested Touch Failure Handling).
- `session-closed` hook migration uses exact-string match (not substring) on the historical `notifyCommand` literal, descending-index unset, post-eviction append-if-absent — preserves user-customised hooks.
- Real-tmux re-entrancy gate (`TestCommitNowFromSessionClosedHook_NoDeadlockUnderRealTmux`) passes within 1.5s budget; spec § Hook Re-entrancy gate satisfied.
- `_portal-saver` self-kill safety covered for both timelines (marker-set byte-identity, marker-clear underscore filter).
- Daemon merge non-regression (semantic, not byte-equivalence per spec acceptance item 11) and six-event eventual consistency (registration-level + firing-side) verified.

### Plan Completion

- [x] Phase 1 acceptance criteria met (all 15 boxes from `planning.md` Phase 1).
- [x] Phase 2 (Analysis Cycle 1) tasks 2-1 through 2-7 completed.
- [x] Phase 3 (Analysis Cycle 2) tasks 3-1, 3-2, 3-3 completed.
- [x] All 17 tasks marked `done` in tick; all corresponding code/test artefacts present.
- [x] No scope creep — consumer-side (`internal/restore/*`) untouched per spec § Consumer-Side Untouched.

### Code Quality

No blocking issues. The implementation matches established cmd-package idioms (`*Deps` struct + per-field nil fallback, `t.Cleanup`-based mock restoration, no `t.Parallel`), uses modern Go error-handling primitives (`errors.Is` / `errors.Unwrap` / `%w`), and lives within sensible cyclomatic-complexity bounds. Doc-comments throughout cite spec sections and explain non-obvious design choices.

Minor architectural asymmetries flagged: `migrateSessionClosedHook` lacks the `if log == nil` guard sibling `migrateHydrationHooks` has (safe in practice — caller normalises); and `failCommitNow`'s `%w: %s: %v` wrap places the cause as text only, not in the `errors.Unwrap` chain — the doc-comment claim of "errors.Unwrap surfaces the underlying cause" is strictly inaccurate.

### Test Quality

Tests adequately verify the spec's Testing Requirements. Unit tests, integration tests, and regression tests are all present in the configurations the spec specifies. The real-tmux re-entrancy gate uses appropriate budget (1.5s with rationale), strong failure diagnostics, and the right skip discipline (`SkipIfNoTmux` / `StagePortalBinary`).

One non-blocking test-strength note: sub-test 2 of `TestCommitNowSymptom` (saver self-kill marker-clear) polls for a sessions.json shape identical to its pre-kill baseline; the test cannot distinguish "commit-now ran and filtered the saver" from "commit-now never ran." The underscore-prefix filter is unit-tested elsewhere, so coverage is not lost, but the integration assertion is weaker than its siblings.

### Required Changes

None.

## Recommendations

### Quick-fixes

1. `cmd/state_commit_now_test.go:80` — typo `restoringCals` → `restoringCalls` (field is unused; rename or delete).
2. `cmd/state_notify.go:50` — outer wrap `fmt.Errorf("touch save.requested: %w", err)` double-prefixes since `state.TouchSaveRequested` already prefixes its errors with `"touch save.requested: "`. Drop the outer wrap or change context (e.g. `"notify: %w"`).
3. `cmd/state_commit_now_symptom_integration_test.go:358` — comment "t.Setenv is scoped to the parent test" is misleading; `newSymptomFixture` receives the sub-test's `t`, so setenv is sub-test-scoped.
4. `cmd/state_commit_now_symptom_integration_test.go:440-447` — doc-comment on `runPortalSubprocess` says "A non-zero exit is treated as a fixture failure" — also used inside sub-test bodies where failure is assertion-level. Tweak to "test failure" for precision.
5. `cmd/state_commit_now_daemon_merge_integration_test.go` — two named subtests in `TestCommitNowDaemonMergeStability` share the same `present` map computed once outside. Per-subtest read would be safer if `ReadIndex` itself were a regression vector. Minor.

### Ideas

6. `failCommitNow` (`cmd/state_commit_now.go:255`) uses `fmt.Errorf("%w: %s: %v", errCommitNowFailed, stage, cause)`. The cause text is preserved in `err.Error()` but `errors.Unwrap(err)` returns `errCommitNowFailed`, not `cause`. Doc-comment at line 245 reads "errors.Unwrap surfaces the underlying cause" — strictly inaccurate. Options: (a) clarify doc that cause is in message text only, or (b) switch to Go 1.20+ multi-`%w` so `errors.Is(err, cause)` also holds (and strengthen T27b accordingly).
7. `migrateSessionClosedHook` (`internal/tmux/hooks_register.go:304`) lacks the `if log == nil` internal guard that `migrateHydrationHooks` has. Safe (sole caller normalises) but asymmetric. Add symmetric guard or precondition doc-line.
8. `migrateSessionClosedHook`'s ShowGlobalHooks failure emits WARN AND returns wrapped error, whereas `migrateHydrationHooks` only wraps+returns. Both defensible; converge or document the asymmetry deliberately.
9. Six-line nil-fallback ladder in `resolveCommitNowDeps` duplicates the shape across sibling `*Deps`. A generic `coalesce[T]` helper (Go 1.18+ generics) could DRY this if duplication grows.
10. `loadPrevIndex` duplicates the `(skip, err)` discrimination pattern used by other `state.ReadIndex` callers. Could become `state.ReadIndexOrZero(dir, logger, component)` if a second consumer emerges.
11. `cmd/state_commit_now_test.go` is 1,273 lines bundling 1-1/1-2/1-3 tests behind section banners. Splitting into per-task files would aid navigation.
12. Sub-test 2 of `TestCommitNowSymptom` shape is identical to pre-kill baseline — cannot prove `commit-now` actually ran. Tighten with mtime delta, log scan, or `save.requested` mtime assertion.
13. `dumpStateDirForNotifyTest` (`cmd/state_notify_six_event_eventual_consistency_test.go`) remains as a near-variant of the consolidated `dumpStateDir`. Cycle-3 duplication analysis deferred consolidation ("promote only if a third dumper appears"). Documented as deferred.
14. The `package cmd` unit-test `sessionNames(idx) []string` (`cmd/state_commit_now_test.go:153`) is the only remaining `sessionNames` variant after consolidation. A future rename (e.g. `sessionNamesSlice`) would clarify shape distinction.
15. `matchesShape` is local to symptom file; if a fourth integration file ever needs it, promote alongside `sessionNames` and `keysOf`.
16. `IsRestoring` default at `resolveCommitNowDeps:98` constructs fresh `tmux.DefaultClient()` per call; `NewClient` also returns `tmux.DefaultClient()`. Could share — `DefaultClient()` is cheap/stateless, harmless.
17. Firing-side test in `state_notify_six_event_eventual_consistency_test.go` polls every 25ms × 500ms (~20 stat calls). A single check immediately after `runStateNotify` returns would suffice — `notify` is synchronous. Loop is belt-and-braces; worth a one-liner justification or simplification.
18. `waitForSaveRequestedConsumed` uses `time.Ticker` with `<-ticker.C` after initial sample — first re-check happens after `daemonTickPollInterval` rather than immediately. Acceptable given 4s budget.
19. `runPortalSubprocess` swallows stdout/stderr unless command fails. Optional `-v` log of `cmd.CombinedOutput` under `testing.Verbose()` would aid future triage.
20. `TestStateCommitNow_TreatsIsRestoringErrorAsMarkerPresumedSet` could also assert `f.restoringCals == 1` to lock the contract that `IsRestoring` is queried exactly once per invocation.
21. `MigrationLogger` interface shape duplicates `BarrierLogger` and bootstrap `Logger`. Future consolidation possible but wider than this task's scope.
22. The four-event spec inventory ("TUI K, portal kill, Option-Q, M-q, external") is asserted by convergence reasoning rather than per-path exercise — intentional per plan (one external kill suffices because same hook fires for all); flagging for future readers.
23. Consider an injected debug-level log on `TouchSaveRequested` Chtimes failure for diagnostics on exotic filesystems where mtime updates fail silently.
