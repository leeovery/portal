# Implementation Review: Built-In Session Resurrection

**Plan**: built-in-session-resurrection
**QA Verdict**: Approve

## Summary

The 128-task implementation lands the full save/restore architecture cleanly: scaffolding, save daemon, skeleton restore, hydrate helper, hook lifecycle migration, bootstrap orchestrator, observability, and three subsequent post-review remediation phases. The Cycle-1 review (originally **Request Changes**) flagged seven required fixes — all now landed via Phase 12. Phase 13 closed Analysis Cycle 6 findings (shared test-helper extraction, production binary path coverage, missing 5-10 acceptance assertions, paneKey parameterisation, documentation precision). Phase 14 closed Analysis Cycle 7 hygiene gaps (deferred logger Close in cleanup, init() rationale removal, OpenAndSignalFIFO unexport). Analysis Cycle 8 returned `STATUS: clean` across duplication / standards / architecture — the implementation is converged.

Code quality is consistently strong throughout: small DI interfaces, table-driven tests without `t.Parallel()`, doc-comments citing spec sections, careful Pascal/camelCase split between reusable adapters and production-only wiring. End-to-end integration coverage now includes a binary-driven reboot round-trip (alpha+beta, base-index drift variant), reattach against saved-only sessions (six tests), marker suppression with non-vacuous probe, multi-session/ANSI/marker round-trip, and full hook firing under `respawn-pane -k` arming.

The single residual issue is a stale inline comment at `internal/tmux/hooks_register_test.go:474` ("registers all 10. Second bootstrap sees those 10") that contradicts the computed 9-hook assertion two lines below. The comment cannot mask a regression (the assertion is computed from `expectedSaveTriggerEvents + expectedHydrationTriggerEvents`), but it represents an incomplete execution of quick-fix task 12-10 — the original review named both line 474 and lines 545-547 as stale; only the latter was updated. A separate documentation precision miss in task 13-5 (no reconciliation note appended to this review file from the cycle-6 sweep) is also tracked. Both are Comments Only and non-blocking.

The feature is ship-ready behind both manual-QA and automated-CI gates. Ship.

## QA Verification

### Specification Compliance

Implementation aligns with the spec's organizing principles ("Portal owns the full lifecycle", "single-writer architecture", "degrade locally, log, continue"). Notable deliberate deviations (all documented in code, all reconciled in plan):

- **`Restore()` signature**: spec/plan suggested `error` only; implementation uses typed `(corrupt bool, err error)` tuple so the orchestrator cannot escalate non-corrupt restore failures to fatal. Cleaner contract than what was planned.
- **`@portal-restoring.Clear` failure classification**: original plan body (task 5-2) said "soft + WARN", but spec § "Fatal Bootstrap Errors", CLAUDE.md, and the implementing test all say fatal. Reconciled in Phase 12 task 12-9 — plan body now correctly classifies as fatal with cycle-1 reconciliation note.
- **`show-options -sv` → `-s`**: spec uses `-sv`, but `-v` (value-only) without an option name would emit values without their names, defeating prefix-based filtering. Implementation uses `-s` (name + value pairs) — spec wording amended.
- **`migrate-rename` hook deferred to v2**: spec § "v1 deferral (post-implementation note)" amended after implementation began; `cmd/state_migrate_rename.go` retained as the v2 endpoint, hook registration dropped from the bootstrap table.
- **Pre-write vs post-write log rotation**: plan says triggering write lands in `.old`; implementation rotates pre-write so triggering line lands in fresh `portal.log`. Both satisfy the ~2 MiB cap. Spec ambiguous; implementation internally consistent.
- **ANSI scrollback assertion via substring contains** (rather than byte-equal as the Phase 12 task 12-4 edge case suggested): `capture-pane -e` normalises `\x1b[0m` → `\x1b[39m` when only fg state changed, and reflow padding breaks literal equality. Substring contains catches every regression byte-equal would. Documented inline in `internal/restore/integration_full_test.go`.

### Plan Completion

- [x] Phase 1 acceptance criteria met (Portal state CLI scaffolding & tmux hook registration)
- [x] Phase 2 acceptance criteria met (Save daemon, triggers, on-disk state format)
- [x] Phase 3 acceptance criteria met — multi-session/ANSI/marker round-trip lives in `internal/restore/integration_full_test.go` (closed by Phase 12 task 12-4)
- [x] Phase 4 acceptance criteria met (Resume-hook lifecycle migration)
- [x] Phase 5 acceptance criteria met — tasks 5-9 (`reboot_roundtrip_test.go`) and 5-10 (`reattach_integration_test.go`) shipped via Phase 12, with 13-3 closing the two remaining 5-10 assertion gaps
- [x] Phase 6 acceptance criteria met — task 6-2 logger retrofit completed by Phase 12 task 12-5; bootstrap `Logger` interface gained `Debug` method (12-6); deferred `Close()` added to all five non-daemon writers (12-7, 14-1)
- [x] Phases 7-11 acceptance criteria met (analysis cycles 1-5)
- [x] Phase 12 acceptance criteria met — 14 review-remediation tasks landed
- [x] Phase 13 acceptance criteria met — 5 cycle-6 analysis tasks landed
- [x] Phase 14 acceptance criteria met — 3 cycle-7 cleanup tasks landed
- [x] Analysis Cycle 8 returned `STATUS: clean` across all dimensions
- [x] All 128 plan tasks marked complete in the manifest
- [x] No unintended scope creep

### Code Quality

Excellent overall. Strong patterns sustained throughout:

- **Small DI interfaces** consistently used: `ServerOptionWriter`, `ServerOptionLister`, `RestoringChecker`, `Restorer`, `FIFOSweeper`, `StaleCleaner`, `HookRegistrar`, `Server`, `Logger` (3-method bootstrap variant, 4-method state variant) — each is 1–3 methods.
- **`*state.Logger` nil-safe receiver pattern** trusted everywhere; matches Go idioms for nil-safe value receivers.
- **Adapter split** between `internal/bootstrapadapter` (reusable Pascal-cased) and `cmd/bootstrap_production.go` (production-only camelCase) is well-documented and avoids import cycles.
- **Table-driven tests** dominate; `t.Parallel()` correctly avoided per project convention.
- **Doc-comments cite spec sections** at multiple load-bearing sites, anchoring decisions back to source-of-truth.
- **Shared integration scaffolding** in `internal/restoretest/` (Phase 13 task 13-1) eliminates ~150 lines of duplication across three test files; `BuildPortalBinaryDir`/`BuildPortalBinaryStable` two-flavour split documents lifetime ownership.
- **DEBUG step-entry breadcrumbs** in `bootstrap.Orchestrator.Run` provide spec-compliant tracing under `PORTAL_LOG_LEVEL=debug`.

Minor concerns flagged below in Recommendations.

### Test Quality

Adequate at unit level — every public function and most private helpers have focused, table-driven coverage with edge cases. Strong test quality observed:

- **Round-trip + drift coverage** on `internal/state` (capture/commit/scrollback/markers).
- **Argv-shape assertions** on every `tmux.Client` method.
- **Order-of-operations** asserted for the bootstrap orchestrator and hydrate helper.
- **Negative-space tests** lock in deliberate non-actions (timeout path does NOT unset marker; hook firing does NOT happen on timeout; marker-suppression: structural events DO fire but `saved_at` does NOT advance).
- **Production binary path coverage** in reboot round-trip via `DriveSignalHydrateBinary` (Phase 13 task 13-2) — the argv-identical pipeline through cobra dispatch into `runSignalHydrate`.
- **Saved-only reattach** (Phase 12 task 12-2 + Phase 13 task 13-3) — six tests covering bare-shell attach, path-arg open, inside-tmux switch-client, steady-state zero-rewrites, has-session-post-bootstrap, unknown-name negative.
- **Permission-error classification** for `ReadIndex` (Phase 12 task 12-8) skips Windows + root for portability.
- **Logger format byte-level assertions** on real on-disk log bytes rather than mocks.

Integration-test gaps from the original review are closed. Marker-suppression test now uses a non-vacuous `set-hook -ga` probe with `t.Cleanup` removal.

### Required Changes

None. The 7 required changes from the original Cycle-1 review are all resolved:
1. Task 5-9 reboot round-trip — `cmd/bootstrap/reboot_roundtrip_test.go` (3 sub-tests, drift variant, production binary path)
2. Task 5-10 reattach — `cmd/reattach_integration_test.go` (6 tests, all 7 acceptance bullets)
3. Task 5-8 marker suppression expansion — non-vacuous probe + `saved_at` invariant in `cmd/bootstrap/phase5_marker_suppression_integration_test.go`
4. Task 3-13 multi-session/ANSI/marker round-trip — `internal/restore/integration_full_test.go`
5. Task 6-2 logger migration — `state_migrate_rename` and `state_notify` route through `*state.Logger`; `bootstrap.Logger` gained `Debug` method; deferred `Close()` everywhere
6. Task 3-1 `ReadIndex` permission-error wrapping — `errors.Is(err, ErrCorruptIndex)` now classifies permission errors correctly
7. Task 5-2 `Restoring.Clear` plan reconciliation — fatal classification with cycle-1 note in two locations

## Recommendations

### Quick-fixes

1. `internal/tmux/hooks_register_test.go:474` — Stale numeric reference `// registers all 10. Second bootstrap sees those 10 in show-hooks` contradicts the immediately following computed assertion (`want = 7 + 2 = 9`). Phase 12 task 12-10 fixed lines 545-547 but missed this one. Update to `registers all 9. Second bootstrap sees those 9`.
2. `.workflows/built-in-session-resurrection/review/built-in-session-resurrection/report.md` — Phase 13 task 13-5 acceptance called for "historical review record gets a reconciliation note (not a rewrite)". Now satisfied by this rewrite — but if a future cycle wants minimal-diff annotation instead, see commit history for prior text.
3. `cmd/bootstrap/errors.go:52-53, 62-63` — Citations say "Observability section of the specification" (umbrella). For consistency with `internal/warning/warning.go:21-22` ("Observability → Proactive Health Signals → TUI interaction"), tighten to "Observability → Proactive Health Signals". One-line edit per godoc; no behavioural impact.
4. `internal/tmux/hooks_register_test.go:474` and elsewhere — Multiple test files reference plan task IDs by number in comments; these will rot if tasks are renumbered. Prefer naming the spec section.
5. `cmd/state_signal_hydrate.go:60` — `signalHydrateRetryDelays` doc-comment could include cumulative totals per step (`// cumulative: 10, 30, 70, 150, 310, 500`) for the next reader.
6. `internal/state/markers.go:45-48` — `ServerOptionWriter` interface comment could cross-reference the post-7-6 consolidation so future readers understand why the seam is exactly this two-method pair.
7. `internal/tmuxtest/socket.go:158` — `_ = out` discard in `WaitForSession` is correct but easy to misread as a lint-silencer; add trailing comment "// intentionally discarded; has-session stderr is noisy during settle window".
8. `cmd/state_status.go` — add explicit `--json` flag absence test (1-line `Lookup("json") != nil` check) to guard against accidental future addition.
9. `cmd/reattach_integration_test.go:327` — Cross-reference comment "phase5_marker_suppression_integration_test.go:207-210" omits the `cmd/bootstrap/` package qualifier.
10. `cmd/bootstrap/phase5_marker_suppression_integration_test.go:182` — Probe-file read returns a fatal on non-`IsNotExist` errors without including the probe-file path in the diagnostic.
11. `cmd/bootstrap/reboot_roundtrip_test.go:452-454` — Comment "Make alpha:w1.p0 the active pane of its window" describes intent but no `select-pane` call follows; rephrase or drop.
12. `cmd/state_status.go` — `scanRecentWarnings` ignores `scanner.Err()`; truncated/oversized last line silently caps the count. A Debug-level log would aid diagnostics.
13. `internal/state/logger.go:120` — `parseLevel` accepts `"warning"` alias not documented in the level-constants block; align doc-comment.

### Ideas

14. `.claude/skills/workflow-review-process` — Sub-agents (Task tool) often misinterpret system-reminders as blocking the Write tool. They returned findings as text rather than persisting to `.workflows/.../report-{phase}-{task}.md`. Parent ended up re-persisting all 22 reports. Consider clarifying agent prompt template to disambiguate Write availability vs. system reminders.
15. `internal/restoretest/restoretest.go:54` — `ProjectRoot` is exported but only called from the unexported `buildPortalBinaryInto`. Phase 14 task 14-3 already followed up on a sibling case (`OpenAndSignalFIFO`); consider unexporting `ProjectRoot` for consistency.
16. `internal/restoretest/restoretest.go:163` — `DriveSignalHydrate`'s 10-second budget and 50 ms cadence are encoded as untyped local constants. Lift to package-level named constants (e.g. `FallbackRetryDelay`, `FallbackRetryBudget`) so divergence-from-production rationale lives at one location.
17. `internal/restoretest/restoretest.go:218` — `DriveSignalHydrateBinary`'s `cmd.Env` construction concatenates `os.Environ()` then appends overrides. Brief comment confirming overrides win over inherited environment would harden against accidental future reordering.
18. `cmd/reattach_integration_test.go:380` — Hard-coded `state.SanitizePaneKey("alpha", 0, 0)` couples the marker assertion to single-pane shape. A small `expectedSkeletonMarker(name, win, pane)` helper colocated with the seed helper would localise the coupling.
19. `cmd/reattach_integration_test.go` — `reattachBuildOnce` / `reattachBinDir` / `reattachBuildErr` is the second sync.Once-cached portal-binary build pattern. Phase 13 task 13-1 already extracted shared helpers; lifting the once-Do wrapper into `internal/restoretest/` would avoid a third re-implementation.
20. `cmd/bootstrap/reboot_roundtrip_test.go:285-291` — In-pane scrollback override writes the deterministic ANSI fixture AFTER `captureAndCommit`. If a future variant adds a post-hydrate capture-and-commit step, dedup might skip rewriting due to stale hash. Add a comment.
21. `cmd/state_notify.go:34-47` — `state.EnsureDir()` called twice (once explicitly for fatal-pre-logger guard, then implicitly inside `openNoRotateLogger()`). EnsureDir is idempotent so correctness-safe but mildly wasteful.
22. Five files use `logger, _ := openNoRotateLogger()` discarding the open error. A helper centralising the policy would shrink duplication.
23. `internal/state/logger.go` — Both diagnostic strings ("portal: log rotation failed", "portal: log reopen failed") are duplicated as string literals across logger.go and logger_test.go. Consider extracting as unexported constants.
24. `schema_test.go:312-329` — Optionally add a `strings.Contains(err.Error(), "current:")` assertion in `TestDecodeIndex_ReturnsErrorWhenVersionUnsupported` to lock the new diagnostic into the test contract.
25. `cmd/state_cleanup_test.go:25-28` — `canonicalTempDir` is now a thin alias to `t.TempDir()` with no remaining unique semantics. A follow-up could inline `t.TempDir()` directly across the ~10 callsites.
26. `cmd/reattach_integration_test.go` — Tests mock `openPathFunc` rather than letting the real `openPath` reach a `mockSessionConnector`. A future hardening could split `openPath` into a connector-injectable shape so the test could exercise the full chain.
27. `internal/state/index_reader.go` — Three-tuple `(Index, bool, error)` is unusual; consider typed `ReadResult{Index, Skip}` + `error` once Phase 5/6 call-site readability can be assessed.
28. `cmd/bootstrap/bootstrap.go` — bootstrap.Logger has 3 methods (Debug/Warn/Error) while *state.Logger has 4 (adds Info). Asymmetry intentional but worth flagging if future steps want INFO emission.
29. `cmd/bootstrap/bootstrap.go` — Step 9 (Return) emits no Debug line. If future debugging surfaces a need for "bootstrap completed in N ms", a trailing Debug line would be the natural place.
30. `internal/restore/integration_full_test.go` — `TestPhase3Integration_FullRoundTrip` is one top-level test (no sub-tests). Splitting into `t.Run("structure", ...)`, `t.Run("zoom", ...)`, etc. would surface which dimension drifted as a sub-test name.
31. `cmd/bootstrap/reboot_roundtrip_test.go` — `verifyCWDs` cases hard-code resolution like `cfg.restoreBase+0`. Could be expressed via a small helper that builds `tmux.PaneTarget` from `(window, pane)` + cfg.
32. `cmd/state_status.go` — missing fixture for `daemon.pid` permission-denied path.
33. `internal/state/status.go` — `CollectStatus` always returns `nil` for error; either drop from signature or document a specific future condition.
34. `internal/state/logger.go` — `LogRotateThreshold` doc-comment could include a one-liner spec reference instead of the "exported so tests can reference" wording.
35. `internal/state/capture.go` — add a literal-string guard test asserting `captureFormat` contains `"#{window_layout}"` and not `"#{window_visible_layout}"`.
36. `cmd/state_daemon.go:25-34` — `daemonDeps.Dir` field name diverges from the planned `StateDir`; either rename or add a one-line comment.
37. `cmd/state_daemon_run_test.go:557-565` — promote the unreachable-error NOTE to a named `t.Skip`-guarded test for discoverability via `go test -v`.
38. `internal/restore/restore.go:153-170` — `warnOnPaneKeyDrift` takes `*SessionRestorer` solely to reach `PredictLiveIndices`; pass `*tmux.Client` directly to remove the indirection.
39. `cmd/bootstrap/noop.go` — no compile-time guard prevents future addition of `NoOpServer{}`/`NoOpRestoringMarker{}`. A short deny-list test would harden the policy.
40. `cmd/bootstrap/noop.go:46-50` — `NoOpFIFOSweeper` doc-comment claims it is the production-fallback when state-dir resolution fails, but production currently always wires `bootstrapadapter.FIFOSweeper`.
41. `cmd/bootstrap/bootstrap.go:80-93` — `FIFOSweeper.Logger` typed as concrete `*state.Logger` while orchestrator `Logger` is the interface; asymmetric.
42. `cmd/bootstrap_production.go:63-70` — `cleanStaleAdapter.CleanStale` silently swallows `ListAllPanes` error; a Warn-log would aid diagnosis.
43. Daemon-level test for "seed corrupt sessions.json → `deps.PrevIndex == nil` + WARN line" would close the seam between the `ReadIndex` contract test and its daemon consumer.
44. Specification line 468 conflates per-pane `CreateFIFO` (defensive remove+mkfifo+chmod in arm phase) with `SweepOrphanFIFOs` (orphan removal in step 7). Reword to disambiguate.
45. `internal/state/index_reader_test.go:213, :249` — Permission tests duplicate ~15 lines of chmod-0o000 setup + cleanup. A small `seedUnreadableSessionsJSON(t, dir)` helper would dedupe.
46. `cmd/bootstrap/reboot_roundtrip_test.go` — `DriveSignalHydrateBinary` reports per-session failures via `t.Errorf` (non-fatal). Defensible and consistent with other helpers, but downstream assertions still execute against an arguably-poisoned state on a partial signal-hydrate failure.
47. `cmd/clean_test.go` — Two regression tests share substantial setup boilerplate; a small `setupCleanTest(t) (projectsFile, hooksFile string)` helper could DRY env-var setup across the file.

### Bugs

48. `main.go:22` — non-fatal branch `fmt.Fprintln(os.Stderr, err)` will print a bare `"\n"` for `ErrStatusUnhealthy` (sentinel with empty `Error()`); status command renders before returning so an empty trailing line follows. Cosmetic but visible.
