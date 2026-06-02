# Implementation Review: Portal Observability Layer

**Plan**: portal-observability-layer
**QA Verdict**: Approve

## Summary

All 68 plan tasks across 11 phases were independently verified against their acceptance criteria, the specification, and the project's Go conventions. Every task is **Complete with zero blocking issues**; the build (`go build ./...`), `go vet ./...`, and the full test suite (`go test ./...`) all pass clean. The implementation is a faithful, high-quality realization of the spec: the `internal/log` slog foundation (swappable-handler indirection, per-record baselines, component-prefix text rendering), calendar-daily rotation with inode-identity reopen and size-cap valve, single-winner retention with per-deletion breadcrumbs, the four process-lifecycle markers + level-filter bypass, the state-mutation audit trail, the boundary-context-preservation sweep, the cycle-summary catalog, the saver/daemon lifecycle taxonomy, and the hydrate forensic trail are all present and behaviour-tested. The five analysis cycles (phases 7–11) landed genuine DRY/clarity cleanups (shared `log.Took`, `storelog.EmitCleanStaleSummary`, the consolidated `internal/logtest.Sink`, the `op=` attr fix, the `signal` component homing) without regressing behaviour. Test discipline is consistently strong — behaviour-focused assertions, real OS-error fixtures over mocks, structured-attr Kind assertions where stringified output could mask regressions, and standing guard tests (migration guard, drift tripwire, discard-construction guard, boundary contract). Verdict is **Approve**; the recommendations below are all non-blocking, with two latent `[bug]` items worth the user's attention because they fall *outside* every planned task's scope and are not tracked elsewhere.

## QA Verification

### Specification Compliance

Implementation aligns with the specification throughout. Notable, well-documented deviations — all sound engineering calls, none a spec violation:
- **`SweepLogsForClean(stateDir)`** instead of the spec/plan-sketched `SweepLogs(stateDir, retentionDays, gated)` — a narrower surface; the "same sweep function" invariant is preserved via the shared unexported `runRetentionSweepWithDays` (tasks 2-9, 8-4).
- **Projects store `project`=name / `path`=path / `value`=name** — task 7-6 resolved the original inverted attribution to match the closed-vocabulary definitions; this contradicts task 3-4's *literal* AC string (`project=<path>`) but is spec-faithful (see Idea below — warrants a human ack).
- **`fifo missing` hard-returns rather than "then exec"** — the spec's Hook-firing Rule-3 table frames it as a fall-through-to-exec, but a missing FIFO yields immediate ENOENT and a hard return; resolved as a genuine distinct exit path and flagged in-code (task 6-3).
- **`process_role` uses the live constant `hydrateTimeout`** where the spec wrote `signalTimeout` (task 6-2); text handler renders single-token attr values unquoted where the spec's illustrative lines show `raw="<v>"` quoted (tasks 2-11/2-12). These are spec-text/code naming drifts, not behavioural divergences.

### Plan Completion
- [x] All 11 phases' acceptance criteria met (Phase 1 foundation/migration; Phase 2 rotation/retention/invariants; Phase 3 state-mutation audit; Phase 4 boundary context; Phase 5 cycle summaries + lifecycle taxonomy; Phase 6 hydrate trail; Phases 7–11 analysis cleanups)
- [x] All 68 tasks completed and independently verified
- [x] No scope creep — where work exceeded a task's literal bounds (e.g. bootstrap step summaries also routed through `log.Took`; the second `syscall.Exec` site in `PathOpener.Open` also emits `process: exec`), it was spec-mandated by the same rule and is an improvement, not creep

### Code Quality

No issues found. Idiomatic Go 1.25 throughout: `atomic.Pointer[slog.Handler]` swap indirection, `errors.Is`/`errors.As` discrimination, `%w` wrapping that preserves `*os.PathError`/`*CommandError`/`*exec.ExitError` chains, the single-logging-owner invariant enforced by a source-walking guard test, and consistent component-bound `log.For(...)` package-init bindings. SOLID/DRY are well-served by the analysis-cycle extractions (`log.Took`, `storelog`, `internal/logtest.Sink`, `showGlobalHooksOrWarn`) without premature abstraction. The unbuffered-writer constraint, the deliberate not-chmod-same-day-segment choice, the inode-identity reopen rationale (tied to the 2026-05-28 incident), and the load-bearing log-ordering invariants (escalated-above-breadcrumb, self-eject → Close → exit) are all documented at the sites a maintainer would land.

### Test Quality

Tests adequately verify requirements. Coverage maps 1:1 to acceptance criteria with behaviour-focused assertions that would fail if the feature broke. Highlights: real-`exec`/real-tmux fixtures rather than synthetic errors on the auto-population paths; `cmd.Stderr`-nil invariant exercised end-to-end; structured `slog.KindDuration`/`KindInt64` assertions where substring rendering could mask a stringified-value regression; ordering proofs via sink snapshots taken at the exec/exit instant; live e2e integration tests for the daemon self-eject and reboot round-trip. Standing guard tests prevent drift (migration-guard for legacy symbols, `ResolveProcessRole` drift tripwire, discard-construction guard, the `SweepOrphanFIFOs` caller-vs-clean boundary contract). Minor test-balance notes (a few near-duplicate cases, a couple of direct-handler-vs-end-to-end gaps) are captured below as ideas, none material.

### Required Changes (if any)

None. No blocking issues were found in any task.

## Recommendations

### Bugs

1. **`portal state status` reads zero "Recent warnings" against real logs** (surfaced verifying tasks 1-10 / 3-x). `internal/state/status.go` (`scanRecentWarnings` / `logEntryQualifies`, `logFieldSeparator = " | "`) still parses the *legacy pipe-delimited* log format, and `cmd/state_status_test.go` + `internal/state/status_test.go` still seed pipe-format lines. Production now writes slog **text** format to `portal.log`, so the health check's warning scan silently matches nothing. This is a latent functional regression in the status command. It is **out of scope for every plan task** (the status *reader* is not the deleted logger and appears in no task's file list) and is **not tracked by any other task** — recommend a dedicated follow-up to migrate the status reader to the slog text format.

2. **File-browser alias creation emits no audit breadcrumb** (surfaced verifying task 3-5). `internal/ui/browser.go` `handleAliasSave` (the shared file-browser "save alias for highlighted dir" `a`-key flow, wired into `cmd/open.go`) is a *third* production alias-mutation site that still uses the un-audited two-step `store.Set(...)` + `store.Save()`; its `AliasSaver` interface exposes only `Load`/`Set`/`Save`. Aliases created this way leave no `aliases: set` breadcrumb, defeating the spec's "single place per file where the breadcrumb can't be forgotten" guarantee. Task 3-5 correctly instrumented the two callers its "Do" list named (`cmd/alias.go`, `internal/tui/model.go`), so this is outside its literal scope — recommend a follow-up threading this caller onto the audited `SetAndSave(name, path, "cli")` seam.

### Quick-fixes

3. **Stale `migration_guard_test.go` exclusion** (tasks 1-9, 1-10): `excludedFromGuard` at `internal/log/migration_guard_test.go:~28` still lists the now-deleted `internal/state/logger.go`; the entry is dead and the surrounding comment claims the legacy type "survives only in" that file, which is no longer true. Drop the entry and update the comment.

4. **Stale `CLAUDE.md` self-supervision doc** (task 5-10): `CLAUDE.md:103` still describes the old `self-supervision: saver-membership lost for N consecutive ticks, exiting` INFO line; the shipped behaviour is `daemon: self-eject ticks=N threshold=3` + `process: exit code=0` via `log.Close(0)`. Refresh the doc.

5. **Stale `saverLogger` docstring** (task 5-7): `internal/tmux/portal_saver.go:18-19` says "its sole consumer is the SIGKILL-escalation DEBUG breadcrumb" — after phases 5-7/5-8 the logger has many consumers. Update.

6. **Duplicated legacy-old literal** (task 2-4): the unit test declares `const legacyOld = "portal.log.old"` independently of production's `legacyOldName`; the test could reference the production const (same package).

### Ideas

7. **Compiler-check the closed component/op vocabularies** (tasks 2-13, 2-14, 5-11, 3-2, 7-3): the 15 component names and the closed `op`/`via` value spaces are enforced only by per-site string literals and convention — `main.go` hardcodes `"process"`, two packages repeat `log.For("signal")`, and stores pass free-string `op`/`via`. A typo would silently mis-route or emit an off-vocabulary value. Exporting `const`s from `internal/log` (e.g. `log.ComponentProcess`) and typed value sets would make the closed taxonomy compiler-checked. Cross-cutting; deferred deliberately by the analysis cycles.

8. **Human ack on the projects-store attr divergence** (tasks 3-4, 7-6): the implementation resolved the flagged ambiguity to `project`=name / `path`=path / `value`=name (spec-faithful, arguably superior), which contradicts task 3-4's literal AC string (`project=<path>`). It is documented in-source; recommend a deliberate sign-off that the divergence is accepted and the key-vs-value convention is consistent across the hooks/aliases/projects stores.

9. **Retention sweep has no terminal cycle-summary line** (task 8-1): the spec's cycle catalog lists "Retention sweep (rotated logs)" under `log-rotate`, but `internal/log/retention.go` emits only per-deletion `log-rotate: deleted` INFO breadcrumbs — no `<verb> complete ... took=T` summary. This is correctly out of scope for the `log.Took` extraction (no bookend to migrate), and the per-deletion breadcrumbs are the spec's documented shape, but the absence of a terminal summary is worth a deliberate look against the catalog.

10. **Reconcile spec text with shipped naming/rendering** (tasks 2-11/2-12, 6-2, 6-3): three spec-text-vs-code drifts merit a spec-hygiene pass — quoted `raw="<v>"` examples vs unquoted single-token rendering; the `signalTimeout` constant name vs live `hydrateTimeout`; and the Rule-3 `fifo missing` "then exec" framing vs its hard-return reality. All are documentation-only; behaviour is correct.

11. **Single-source the test-infra portal.log contract** (task 11-2): `internal/restoretest/logger.go` duplicates `internal/log`'s portal.log naming/symlink contract (name, `"2006-01-02"` layout, bare-relative target, temp-then-rename swing) with comments requiring "lockstep" but nothing enforcing it. A small shared leaf package (analogous to `tmuxout`/`tmuxerr`) exposing the day-file/symlink-target builders would give a single source of truth.

12. **Harden the `ListSessions` error swallow** (task 4-6): `internal/tmux/tmux.go` collapses *all* `list-sessions` errors to an empty-slice success while the new comment asserts "no server running"; a non-server error (malformed `-F`, tmux crash) is silently swallowed to zero sessions. Out of scope for the comment-only task 4-6, but a future Boundary-class-2 pass could discriminate server-absent from other failures.

13. **Minor robustness/cleanup items** raised across tasks, each low-value: `Close`'s `took` not routed through the `log.Took` single-source helper (1-4); `quoteIfMultiWord` doesn't escape embedded `=`/`"` (1-3); `claimNextSegment`'s unbounded retry loop lacks a sanity ceiling (2-6); duplicated segment-name parsing between `nextSegmentN` and `pastDayLogDate` (2-6); `claimSweepGate` swallows non-EEXIST open errors with no WARN (2-8); now-dead `Discard()`/`EagerSignalCore.Logger`/`migrationGuard` error-return surfaces (7-1, 5-11, 2-4); triplicated `[needs-info]` clean-stale rationale comments now that emission is centralized (3-3, 8-2).

14. **Minor test-coverage / test-balance ideas**, none material: a couple of near-duplicate test pairs could be consolidated (Close-before-Init in 1-4; self-eject cataloged-event in 5-10; remove-failure in 7-5); a few criteria are verified structurally rather than by a dedicated end-to-end test (zero-pane no-geometry-summary in 5-4; mid-stream io.Copy routing in 6-3; the destructive-contention regression in 11-2); and `bytes=N` assertions in 6-4 could add a `KindInt64` check for parity with the `took` Kind assertion.
