TASK: cli-verb-surface-redesign-4-8 — Fold stale-project pruning into the daemon's throttled automation

ACCEPTANCE CRITERIA:
- Filesystem-only classification (gone-dir → stale/pruned, permission-denied → retained) mirroring project.Store.CleanStale
- Slow (hourly-ish) throttled cadence like the existing hook cleanup, not per-tick
- Best-effort / non-fatal to the capture loop
- Runs only inside the live _portal-saver pane so the down-server false-orphan hazard does not apply
- No new log component (closed taxonomy)

STATUS: Complete

SPEC CONTEXT:
Spec §"clean deleted" (specification.md:329): "Stale-project pruning folds into the daemon's automation on a slow cadence (hourly-ish; hooks already prune on the idle tick). Mechanism/cadence is an implementation detail. Net effect: doctor reads healthy almost always because the automation keeps it that way; --fix is the manual trigger of the same repairs." The `clean` verb (which used to own the stale-project prune) is dissolved; the prune is now automated by the daemon, with `doctor --fix` (task 4-5) as the manual backstop. The classification must mirror project.Store.CleanStale (os.Stat: ErrNotExist → prune, permission-denied/other → retain).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_daemon.go:237-241 — projectCleanupInterval = 1 * time.Hour (hourly-ish slow cadence; comment explicitly contrasts the ~10s hookCleanupInterval)
  - cmd/state_daemon.go:462-501 — maybeRunProjectCleanup: nil-guard → throttle-check (time.Since(lastProjectCleanup) >= projectCleanupInterval) → deps.ProjectStore.CleanStale() → WARN-and-swallow on error → reset anchor AFTER body. Shape mirrors maybeRunHookCleanup exactly.
  - cmd/state_daemon.go:404-410 — call site: on tick()'s idle branch (!dirty && !gap), after maybeRunHookCleanup, before return; below the @portal-restoring early-return so it is skipped during a restore window and on capture-pending ticks.
  - cmd/state_daemon.go:55-69 — daemonDeps.ProjectStore + lastProjectCleanup fields with doc.
  - cmd/state_daemon.go:789-818 — RunE wiring: loadProjectStore() built once (same resolver foreground/doctor use), nil-on-failure with a single WARN, lastProjectCleanup anchored to daemon-start.
- Notes:
  - Classification is delegated to project.Store.CleanStale (store.go:191-237) rather than re-implemented, so gone-dir/permission-denied mirroring is guaranteed by construction. CleanStale's own permission-denied retention path is directly tested in internal/project/store_test.go:633 (TestCleanStale "retains project with permission denied", via os.Chmod(parentDir, 0o000)).
  - No new log component: deps.Logger is daemonLogger = log.For("daemon") (state_common.go:18). The gate's only emission is a WARN under the existing `daemon` component; CleanStale emits its own audit under the existing `projects` component. Closed taxonomy respected.
  - Down-server hazard correctly NOT guarded (state_daemon.go:485-489 comment): the daemon runs only inside the live _portal-saver pane, and a gone directory is unambiguously stale regardless of server state — unlike the hook prune whose zero-live-panes read is ambiguous. This is the correct reasoning and matches the AC.
  - No drift from plan.

TESTS:
- Status: Adequate
- Coverage (cmd/state_daemon_project_cleanup_test.go):
  - PrunesGoneDirOnceIntervalElapsed — gone-dir pruned + anchor advanced (filesystem classification + cadence).
  - RetainsLiveDirProject — live dir retained (classification retain-path).
  - DoesNotRunBeforeInterval — throttle gate proves not-per-tick; anchor untouched.
  - LogsWarnAndSwallowsCleanStaleError — best-effort/non-fatal; asserts "projects stale-cleanup failed" under component=daemon; anchor still advances after failure.
  - NilStoreNoOps — nil-store disables prune, no throttle mutation, no WARN.
  - TestStateDaemon_ProjectCleanupWiring (3 subtests) — startup wiring: store built from loadProjectStore, lastProjectCleanup anchored to start (within 2s), loadProjectStore failure → nil store + WARN, daemon not aborted.
- Notes:
  - Permission-denied retention is not simulated directly here (EACCES is hard to reproduce portably); the test comment acknowledges this and relies on the live-dir survivor plus the shared CleanStale being independently tested in the project package. Reasonable — the classification is not re-implemented in this task.
  - Gap (non-blocking): the hook-cleanup counterpart has TestDaemonTick_RunsHookCleanupOnIdleTick (state_daemon_run_test.go:607) proving tick()'s idle branch actually invokes the gate. There is no equivalent test proving tick() invokes maybeRunProjectCleanup — the gate is unit-tested in isolation and the deps field is wired, but if the tick():408 call line were removed the prune would silently never run and all existing tests would still pass. Small regression surface (the call site is present and correct), but the tick→gate wiring is asymmetrically uncovered vs the hook path.
  - Not over-tested: each test targets a distinct behaviour; no redundant assertions.

CODE QUALITY:
- Project conventions: Followed. Best-effort/WARN-and-swallow matches the daemon's "never crash on transient failure" contract; nil-store-disables mirrors the HookStore pattern; anchor-reset-after-body mirrors maybeRunHookCleanup; loadProjectStore resolves the same projects.json foreground/doctor use (env-inheritance rule). Tests obey the no-t.Parallel / package-level-state convention.
- SOLID principles: Good. Classification single-sourced in project.Store.CleanStale (not duplicated); the gate is a thin throttle+delegate.
- Complexity: Low. Gate is a 4-statement guard chain.
- Modern idioms: Yes.
- Readability: Good — arguably heavily commented, but consistent with this file's established density; the comments carry real load (hazard-guard rationale, closed-taxonomy note, cadence contrast).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/state_daemon_run_test.go — add a tick()-idle-branch test for the project prune mirroring TestDaemonTick_RunsHookCleanupOnIdleTick (state_daemon_run_test.go:607): seed a gone-dir project, elapse lastProjectCleanup, run tick() on an idle tick, assert the gone-dir project is pruned and no capture cycle ran. Closes the asymmetric coverage gap so a deleted tick():408 call site fails a test.
- [idea] cmd/state_daemon.go:449-501 — maybeRunHookCleanup and maybeRunProjectCleanup share the identical nil-guard → throttle-check → best-effort call → reset-anchor skeleton; consider extracting a shared throttled-gate helper (anchor *time.Time, interval, disabled, action closure, warnMsg). Decide whether the abstraction earns its keep for only two call sites with deliberately divergent doc comments (premature-abstraction risk).
