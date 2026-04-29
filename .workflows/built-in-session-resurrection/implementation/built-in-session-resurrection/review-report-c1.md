---
scope: built-in-session-resurrection
cycle: 1
source: review
total_findings: 53
deduplicated_findings: 53
proposed_tasks: 14
---
# Review Report: built-in-session-resurrection (Cycle 1)

## Summary

QA verdict is **Request Changes** with 7 blocking items and 46 non-blocking recommendations. Blocking issues cluster in integration-test coverage (tasks 5-8/5-9/5-10/3-13 descoped or never implemented), the task 6-2 logger-migration retrofit (two subcommands still use stderr; bootstrap Logger lacks Debug; hydrate paths leak log fds), an `ErrCorruptIndex` wrapping gap in `ReadIndex`, and a plan-body wording reconciliation. Production code paths function as designed; the gap is the regression net. The 46 recommendations split into 16 quick-fixes, 27 ideas, and 3 bugs — roughly 7 high-value items are promoted; the rest are cosmetic or speculative and remain in the inbox.

## Discarded Findings

- **Quick-fix 2** (hooks_register_test.go L474 comment "registers all 10") — pure comment polish; not user-visible.
- **Quick-fix 3** (markerProbeStub.wantValue unused field) — dead-field cleanup; trivial.
- **Quick-fix 4** (`parseLevel` "warning" alias doc-comment) — doc-only nit.
- **Quick-fix 6** (markers_test.go sample paneKey rename) — readability nit only.
- **Quick-fix 7** (state_status.go --json absence test) — speculative regression guard.
- **Quick-fix 8** (state_cleanup.go SilenceErrors local override) — defensive lock with no observed regression.
- **Quick-fix 11** (restore/session.go doc-comment "helper as initial process" wording) — doc-only.
- **Quick-fix 12** (bootstrap.go L147 godoc wording polish) — doc-only.
- **Quick-fix 13** (test files reference plan task IDs) — convention nudge; broad sweep with low payoff.
- **Quick-fix 14** (signalHydrateRetryDelays cumulative comment) — doc-only.
- **Quick-fix 15** (ServerOptionWriter cross-reference comment) — doc-only.
- **Quick-fix 16** (tmuxtest/socket.go `_ = out` trailing comment) — doc-only.
- **Idea 17** (SkeletonMarkerName helper) — already covered as accepted exception in task 7-19; either deviation is acceptable per QA.
- **Idea 18** (drop "portal state migrate-rename" substring sunset) — premature; v2 migration window still relevant.
- **Idea 20** (defense-in-depth `corrupt && !errors.Is` validation) — speculative.
- **Idea 21** (Run-receiver mutation race) — Run is single-call; theoretical race only.
- **Idea 22** (BootstrapDeps.ForceMemoise → BypassMemoise rename) — naming preference; no defect.
- **Idea 23** (drop PredictLiveIndices / drift WARN) — judgment call without telemetry signal.
- **Idea 24** (state_hydrate.go nil-check field comments) — doc-only.
- **Idea 25** (tmuxtest socket.go runRaw rename) — case-collision with public API; cosmetic.
- **Idea 26** (socketArgs unit test) — already locked at integration scope.
- **Idea 28** (state_status daemon.pid permission-denied fixture) — narrow fixture gap; low payoff.
- **Idea 30** (CollectStatus always-nil error signature) — doc / signature polish.
- **Idea 31** (LogRotateThreshold doc-comment spec ref) — doc-only.
- **Idea 32** (capture.go literal-string guard test) — narrow guard.
- **Idea 33** (daemonDeps.Dir field name) — naming nit.
- **Idea 34** (unreachable-error NOTE → Skip-guarded test) — discoverability nit.
- **Idea 35** (warnOnPaneKeyDrift signature pass-through) — minor refactor.
- **Idea 36** (NoOpServer/NoOpRestoringMarker deny-list test) — speculative regression guard.
- **Idea 37** (NoOpFIFOSweeper doc-comment fallback) — doc-only.
- **Idea 38** (FIFOSweeper.Logger asymmetric type) — minor; not load-bearing.
- **Idea 41** (ReadIndex three-tuple → typed result) — readability refactor; deferrable.
- **Idea 42** (specification CreateFIFO vs SweepOrphanFIFOs wording) — spec doc edit.
- **Idea 43** (daemon-level corrupt-seed test) — broader test gap addressed by promoted integration tasks.

All 7 blocking items are promoted. 7 non-blocking items are promoted (quick-fixes 1, 5, 9, 10; ideas 19, 27, 39; bugs 44, 45, 46 — bugs 45/46 fold into existing blocking tasks; bug 44 promoted standalone; quick-fix 5 promoted standalone).
