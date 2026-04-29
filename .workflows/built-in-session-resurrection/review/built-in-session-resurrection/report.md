# Implementation Review: Built-In Session Resurrection

**Plan**: built-in-session-resurrection
**QA Verdict**: Request Changes

## Summary

The 106-task implementation lands the full save/restore architecture cleanly: scaffolding, save daemon, skeleton restore, hydrate helper, hook lifecycle migration, bootstrap orchestrator, observability, and five subsequent analysis cycles. Code quality is consistently strong — small interfaces with DI, table-driven tests without `t.Parallel()`, clear doc-comments citing spec sections, and a thoughtful Pascal/camelCase split between reusable adapters and production-only wiring. The final 11-cycle phase structure with five analysis cycles produced a polished codebase: ~95% of tasks land Complete with zero blocking issues.

The blocking gaps are concentrated in **integration-test coverage**: tasks 5-8, 5-9, and 5-10 — three of the highest-value end-to-end regression guards the spec calls out — were either substantially descoped (5-8) or never implemented (5-9, 5-10). Phase 3's headline integration test (3-13) was also descoped from multi-session/ANSI/marker-clearance to a single-session minimal smoke. A separate observability gap (6-2) leaves the `migrate-rename` and `notify` subcommands logging via stderr instead of `*state.Logger`, and the `bootstrap.Logger` interface lacks the `Debug` method the spec implicitly requires for "Bootstrap events at DEBUG level only." Finally, 3-1's `ReadIndex` deliberately omits `ErrCorruptIndex` wrapping on permission errors, contradicting a plan acceptance criterion downstream warning classifiers depend on.

None of the blocking issues are in the **production code path** — the feature works end-to-end as designed; what is missing is the regression net that would catch a future drift. The implementation is ship-ready behind a manual-QA gate; it is not ship-ready behind an automated-CI gate of the strength the spec implies.

## QA Verification

### Specification Compliance

Implementation aligns with the spec's organizing principles ("Portal owns the full lifecycle", "single-writer architecture", "degrade locally, log, continue"). Notable deliberate deviations, all documented in code:

- **`Restore()` signature**: spec/plan suggested `error` only; implementation uses typed `(corrupt bool, err error)` tuple so the orchestrator cannot escalate non-corrupt restore failures to fatal. Cleaner contract than what was planned.
- **`@portal-restoring.Clear` failure classification**: plan body (task 5-2) says "soft + WARN", but spec § "Fatal Bootstrap Errors", CLAUDE.md, and the implementing test all say fatal. Implementation correctly follows spec; the plan body's "soft" wording is the outlier and should be reconciled in plan documentation.
- **`show-options -sv` → `-s`**: spec uses `-sv`, but `-v` (value-only) without an option name would emit values without their names, defeating prefix-based filtering. Implementation uses `-s` (name + value pairs) — spec wording should be amended.
- **`migrate-rename` hook deferred to v2**: spec § "v1 deferral (post-implementation note)" amended after implementation began; `cmd/state_migrate_rename.go` retained as the v2 endpoint, hook registration dropped from the bootstrap table.
- **Pre-write vs post-write log rotation**: plan says triggering write lands in `.old`; implementation rotates pre-write so triggering line lands in fresh `portal.log`. Both satisfy the ~2 MiB cap. Spec is ambiguous; implementation is internally consistent.

### Plan Completion

- [x] Phase 1 acceptance criteria met
- [x] Phase 2 acceptance criteria met
- [ ] **Phase 3 acceptance criteria**: integration test (task 3-13) covers structure round-trip but lacks multi-session, multi-window, multi-pane, ANSI scrollback, marker-clearance, env-round-trip, zoom-round-trip, and active-pane round-trip per the spec acceptance bullet at planning.md L96.
- [x] Phase 4 acceptance criteria met
- [ ] **Phase 5 acceptance criteria**: tasks 5-8, 5-9, 5-10 integration tests are the headline regression guards for the spec's "WaitForSessions removal" + "skeleton-restore + reattach" workflow. 5-8 substantially descoped; 5-9 and 5-10 entire test files never created.
- [ ] **Phase 6 acceptance criteria**: task 6-2 retrofit incomplete — `migrate-rename` and `notify` still write to stderr instead of `*state.Logger`, leaving two subsystems invisible to `portal.log` and to `portal state status` recent-warnings scanning.
- [x] Phases 7–11 (analysis cycles) acceptance criteria met
- [x] All 106 plan tasks marked complete in the manifest
- [x] No unintended scope creep (scope grew via spec amendment + `internal/warning` extraction, both well-justified)

### Code Quality

Excellent overall. Strong patterns observed throughout:

- **Small DI interfaces** consistently used: `ServerOptionWriter`, `ServerOptionLister`, `RestoringChecker`, `Restorer`, `FIFOSweeper`, `StaleCleaner`, `HookRegistrar`, `Server` — each is 1–2 methods.
- **`*state.Logger` nil-safe receiver pattern** trusted everywhere after task 7-4's guard removal; matches Go idioms for nil-safe value receivers.
- **Adapter split** between `internal/bootstrapadapter` (reusable Pascal-cased) and `cmd/bootstrap_production.go` (production-only camelCase) is well-documented and avoids import cycles.
- **Table-driven tests** dominate; `t.Parallel()` correctly avoided per project convention.
- **Doc-comments cite spec sections** at multiple load-bearing sites, anchoring decisions back to source-of-truth.

Minor concerns flagged below in Recommendations.

### Test Quality

Adequate at the unit level — every public function and most private helpers have focused, table-driven coverage with edge cases. Specific quality strengths:

- **Round-trip + drift coverage** on `internal/state` (capture/commit/scrollback/markers).
- **Argv-shape assertions** on every `tmux.Client` method.
- **Order-of-operations** asserted for the bootstrap orchestrator and hydrate helper.
- **Negative-space tests** lock in deliberate non-actions (timeout path does NOT unset marker; hook firing does NOT happen on timeout; etc.).

The integration-test gap dominates the test-quality assessment. The spec's headline "Validation Reference" (§ Scrollback Restore Mechanics) explicitly validates ANSI fidelity end-to-end, but no integration test verifies ANSI SGR survives save → kill-server → restore → attach → hydrate. The save-restore round-trip test shipped is a single-session, no-ANSI, no-marker-clearance smoke.

### Required Changes

1. **Implement task 5-9** (end-to-end reboot round-trip integration test). Required by spec/Phase 5 acceptance bullet at planning.md L164. The test file `cmd/bootstrap/reboot_roundtrip_test.go` does not exist. If full PTY-driven attach is too fragile, the plan's documented fallback (invoke `state.SignalHydrate` directly) is acceptable. Must verify structure, layout, zoom, CWDs, environment, hook firing, and ANSI scrollback in at least one base-index-drift configuration.

2. **Implement task 5-10** (`portal attach NAME` and `portal open` resolve `sessions.json`-only names integration test). Required by spec/Phase 5 acceptance bullet at planning.md L166. File `cmd/reattach_integration_test.go` does not exist. Five named test cases all absent.

3. **Expand task 5-8** (marker-suppression integration test). Add a probe `set-hook -ga` registered before the orchestrator runs that records structural events to a tempfile during the marker window; assert at least one event fires (non-vacuous) and that `sessions.json.saved_at` is not advanced. Add `//go:build integration` tag and `testing.Short()` skip.

4. **Expand task 3-13** (Phase 3 integration test) to cover the full plan acceptance: at minimum, two sessions × multi-window × multi-pane fixture; one zoomed pane; ANSI SGR byte-comparison; per-session environment round-trip; active-pane round-trip; marker clearance after `signal-hydrate` + helper dump.

5. **Fix task 6-2 logger migration**:
   - Open `*state.Logger` in `cmd/state_migrate_rename.go` (use `ComponentHooks`); replace stderr `fmt.Fprintf` calls with `Logger.Warn`/`Logger.Info`.
   - Open `*state.Logger` in `cmd/state_notify.go` (use `ComponentNotify`); add WARN on file-create failure per plan.
   - Add `Debug(component, format string, args ...any)` to `cmd/bootstrap/bootstrap.go` `Logger` interface; emit DEBUG on each step entry per spec § Observability "Bootstrap events at DEBUG level only".
   - Add `defer logger.Close()` to `cmd/state_signal_hydrate.go` and `cmd/state_hydrate.go` `RunE` bodies (production paths exec away, but tests + cmd interruption paths leak the fd).

6. **Fix task 3-1 `ReadIndex` permission-error wrapping**: change `cmd/.../internal/state/index_reader.go:42` to wrap the read-error branch with `ErrCorruptIndex` so downstream `errors.Is(err, ErrCorruptIndex)` classifiers (Phase 5 task 5-2 / Phase 6 task 6-9) correctly bucket permission errors as soft warnings. Alternatively, document the deliberate exclusion in the plan + spec and add tests/docs at the consumer sites that match the unwrapped read-error case explicitly.

7. **Reconcile plan body wording for task 5-2 `Restoring.Clear`**: plan body says "soft", spec/CLAUDE.md/test all say fatal. Implementation correctly follows spec — amend plan body so future reviewers don't flag this as drift.

## Recommendations

### Quick-fixes

1. `internal/tmux/hooks_register_test.go:545-547` — stale comment references "the migrate-rename call" that no longer exists.
2. `internal/tmux/hooks_register_test.go:474` — comment says "registers all 10" but assertion uses correct slice-len expression for 9.
3. `cmd/bootstrap/phase5_integration_test.go:42` — `markerProbeStub.wantValue` field set but never read; remove or use in the assertion block.
4. `internal/state/logger.go:120` — `parseLevel` accepts `"warning"` alias not documented in the level-constants block; align doc-comment.
5. `internal/state/logger.go:184-188` — rename-failure reopen swallows error silently while parallel branch emits diagnostic; restore symmetry.
6. `internal/state/markers_test.go:280-322` — sample paneKey `@portal-active-some-pane` reads like another Portal namespace; rename to `@my-plugin-foo` to communicate "unrelated".
7. `cmd/state_status.go` — add explicit `--json` flag absence test (1-line `Lookup("json") != nil` check) to guard against accidental future addition.
8. `cmd/state_cleanup.go` — inherits `SilenceErrors`/`SilenceUsage` from rootCmd rather than declaring locally; explicit local override would lock the contract.
9. `internal/state/schema.go:109` — `unsupported sessions.json version` error omits the expected version; add `(current: %d)` for diagnostics.
10. `internal/state/scrollback.go` — `xxhash.Sum64([]byte(out))` allocates copy; prefer `xxhash.Sum64String(out)` to avoid the allocation.
11. `internal/restore/session.go:9-10` — doc-comment says "preserving the spec's 'helper as initial process' invariant" but spec retired that phrasing post-task-8-3; reword to "helper-before-shell".
12. `cmd/bootstrap/bootstrap.go:147-148` — godoc says "the final return swallows it" but restoreErr is logged-and-discarded inline at the switch case; minor wording polish.
13. Multiple test files reference plan task IDs by number in comments; these will rot if tasks are renumbered. Prefer naming the spec section.
14. `cmd/state_signal_hydrate.go:60` — `signalHydrateRetryDelays` doc-comment could include cumulative totals per step (`// cumulative: 10, 30, 70, 150, 310, 500`) for the next reader.
15. `internal/state/markers.go:45-48` — `ServerOptionWriter` interface comment could cross-reference the post-7-6 consolidation so future readers understand why the seam is exactly this two-method pair.
16. `internal/tmuxtest/socket.go:158` — `_ = out` discard in `WaitForSession` is correct but easy to misread as a lint-silencer; add trailing comment "// intentionally discarded; has-session stderr is noisy during settle window".

### Ideas

17. `internal/restore/restore.go` and `cmd/state_hydrate.go` — two stray `state.SkeletonMarkerPrefix +` log-formatting concatenations remain outside `markers.go` (task 7-19's literal acceptance). Either introduce a `state.SkeletonMarkerName(paneKey)` helper or update plan acceptance.
18. `internal/tmux/hooks_unregister.go:24-28` — `portalCommandSubstrings` retains `"portal state migrate-rename"` for legacy upgrade cleanup despite task 7-2 path (b). Document sunset condition or open a follow-up to drop after migration window closes.
19. `cmd/state_cleanup.go:141-148` — `purgeStateDir` `EvalSymlinks` comparison rejects valid intermediate-symlink paths (e.g., `~/.config` symlinked, macOS `~/Library`). Test suite hides this via `canonicalTempDir`; real users would see "refusing to purge" and have to clean up manually. Suggest dropping the comparison or relaxing to leaf-symlink only (which `Lstat` already catches).
20. `cmd/bootstrap/bootstrap.go:198-209` — switch could explicitly validate `corrupt && !errors.Is(restoreErr, state.ErrCorruptIndex)` and Warn-log a stronger contract violation. Defense-in-depth.
21. `cmd/bootstrap/bootstrap.go:162-164` — `Run` mutates receiver to install noopLogger; method-local `logger := o.Logger; if logger == nil { logger = noopLogger{} }` would be race-safe.
22. `cmd/root.go` — `BootstrapDeps.ForceMemoise` reads as "opt-in to production behavior"; rename to `BypassMemoise` (inverted default) for more intuitive semantics.
23. `internal/restore/session.go:412-426` — `PredictLiveIndices` retained solely for orchestrator drift diagnostic. If drift WARN is judged low-value, consider removing both helper and warning.
24. `cmd/state_hydrate.go` — production-always nil-checks on `cfg.HandleTimeout` / `cfg.HandleFileMissing` are pure test affordances; document at the field comments.
25. `internal/tmuxtest/socket.go:118` — private `runRaw` collides case-only with public `RunRaw`. Rename to `execTmux` or `runBytes`.
26. Add a dedicated `socketArgs` unit test (table-driven) so the prefix shape is locked at unit scope rather than only via heavy integration suites.
27. `cmd/clean.go` — add the two spec-pinned regression tests from the plan: binary-missing-not-staleness-signal and projects.json-absence-not-staleness-signal.
28. `cmd/state_status.go` — missing fixture for `daemon.pid` permission-denied path.
29. `cmd/state_status.go` — `scanRecentWarnings` ignores `scanner.Err()`; truncated/oversized last line silently caps the count. A Debug-level log would aid diagnostics.
30. `internal/state/status.go` — `CollectStatus` always returns `nil` for error; either drop from signature or document a specific future condition.
31. `internal/state/logger.go` — `LogRotateThreshold` doc-comment could include a one-liner spec reference instead of the "exported so tests can reference" wording.
32. `internal/state/capture.go` — add a literal-string guard test asserting `captureFormat` contains `"#{window_layout}"` and not `"#{window_visible_layout}"`.
33. `cmd/state_daemon.go:25-34` — `daemonDeps.Dir` field name diverges from the planned `StateDir`; either rename or add a one-line comment.
34. `cmd/state_daemon_run_test.go:557-565` — promote the unreachable-error NOTE to a named `t.Skip`-guarded test for discoverability via `go test -v`.
35. `internal/restore/restore.go:153-170` — `warnOnPaneKeyDrift` takes `*SessionRestorer` solely to reach `PredictLiveIndices`; pass `*tmux.Client` directly to remove the indirection.
36. `cmd/bootstrap/noop.go` — no compile-time guard prevents future addition of `NoOpServer{}`/`NoOpRestoringMarker{}`. A short deny-list test would harden the policy.
37. `cmd/bootstrap/noop.go:46-50` — `NoOpFIFOSweeper` doc-comment claims it is the production-fallback when state-dir resolution fails, but production currently always wires `bootstrapadapter.FIFOSweeper`. Either drop the claim or add the symmetric fallback.
38. `cmd/bootstrap/bootstrap.go:80-93` — `FIFOSweeper.Logger` typed as concrete `*state.Logger` while orchestrator `Logger` is the interface; asymmetric.
39. `cmd/bootstrap_production.go:63-70` — `cleanStaleAdapter.CleanStale` silently swallows `ListAllPanes` error; a Warn-log would aid diagnosis.
40. Plan body for tasks 5-10, 6-2, 3-1 contradicts implementation choices (Restoring.Clear, ErrCorruptIndex wrapping, etc.) — sweep plan files to reconcile or annotate the deviations.
41. `internal/state/index_reader.go` — three-tuple `(Index, bool, error)` is unusual; consider typed `ReadResult{Index, Skip}` + `error` once Phase 5/6 call-site readability can be assessed.
42. Specification line 468 conflates per-pane `CreateFIFO` (defensive remove+mkfifo+chmod in arm phase) with `SweepOrphanFIFOs` (orphan removal in step 7). Reword to disambiguate.
43. Daemon-level test for "seed corrupt sessions.json → `deps.PrevIndex == nil` + WARN line" would close the seam between the `ReadIndex` contract test and its daemon consumer.

### Bugs

44. `main.go:22` — non-fatal branch `fmt.Fprintln(os.Stderr, err)` will print a bare `"\n"` for `ErrStatusUnhealthy` (sentinel with empty `Error()`); status command renders before returning so an empty trailing line follows. Cosmetic but visible.
45. `cmd/state_signal_hydrate.go:149` and `cmd/state_hydrate.go:360` — log fd not deferred-closed; leaks until process exit. Production paths exec away (acceptable), but interrupted paths and tests retain leaked fds.
46. `cmd/state_cleanup.go:141-148` — `purgeStateDir` rejects valid resolved-path purges when intermediate path components are symlinks (see Idea 19). Real users on macOS/symlinked-config setups would see false-positive refusals.
