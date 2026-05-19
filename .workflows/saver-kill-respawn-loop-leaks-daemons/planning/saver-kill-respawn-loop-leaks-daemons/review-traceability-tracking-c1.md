---
status: complete
created: 2026-05-19
cycle: 1
phase: Traceability Review
topic: saver-kill-respawn-loop-leaks-daemons
---

# Review Tracking: Saver Kill-Respawn Loop Leaks Daemons - Traceability

## Findings

None. The plan is a faithful, complete translation of the specification.

## Direction 1 — Specification → Plan (completeness)

All specification elements have plan coverage:

- **Change 1 (alive-check first)** — Tasks 1-3 (gate kill on `BootstrapAliveCheck`), 1-4 (defensive `WriteVersionFile` on alive+absent), 1-5 (revise function comment at lines 232-241), 1-1 (reframed predicate table tests).
- **Change 2 (ctx-aware captureAndCommit)** — Tasks 2-1 (thread ctx + happy-path regression), 2-2 (observation point 1 / entry), 2-3 (observation point 2 / post-enumeration), 2-4 (observation point 3 / between per-pane iterations).
- **Change 3 (WriteVersionFile DEBUG breadcrumb)** — Task 1-2 (under `ComponentDaemon`, prefix `daemon.version write:`, includes version + pid + path).
- **Acceptance Criteria #1-#10** — All ten criteria mapped: Phase 1 acceptance covers steady-state (#1-#5), version-upgrade (#6), and diagnostic (#9); Phase 2 acceptance covers daemon responsiveness (#7, #8) and regression (#10).
- **Testing Requirements** — Predicate table tests (Task 1-1), `EnsurePortalSaverVersion` ordering tests (Task 1-3), `captureAndCommit` cancellation tests (Tasks 2-2/2-3/2-4 with happy-path guard in 2-1), Integration test #1 alive+absent (Task 1-6), Integration test #2 mid-tick SIGHUP (Task 2-5), Integration test #3 lock-loser cascade (Task 2-6).
- **Regression preservation** — `multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart` all listed in Phase 2 acceptance and Task 2-6.
- **What stays unchanged** (daemon.lock primitive, `killSaverAndWaitForDaemon`, `BootstrapPortalSaver`, `killBarrierTimeout=5s`, no-daemon path, dev-build handling, `internal/state/capture.go` signatures, per-tick capture algorithm) — all reflected in task constraints.
- **Rejected alternatives** (raise timeout, bound capture-pane lines, goroutine restructure, distinguish `ErrVersionFileAbsent` in predicate only) — none re-introduced in the plan.
- **Out of Scope** items (hook-registration redundancy, capture-pane line bounding, raising timeout, identifying Defect 3 root cause, goroutine restructure, TUI loading floor, version storage migration) — none addressed by tasks.
- **AC #5 "~520ms reclaimed"** — spec marks this informational, not a hard regression test; plan correctly omits a guarded test for it (Phase 1 acceptance criteria capture the user-visible WARN-absence and daemon-survival assertions that are the load-bearing observables).

## Direction 2 — Plan → Specification (fidelity)

Every task element traces to specification content:

- Task 1-1 framing change (predicate-as-one-input) traces to spec §Testing Requirements > Unit tests ("its framing must be reworked, not deleted").
- Task 1-2 prefix contract `daemon.version write:` traces verbatim to spec §Change 3 ("Format anchor: the log line MUST begin with `daemon.version write:`").
- Task 1-3 six-row decision matrix traces verbatim to spec §Change 1 decision matrix.
- Task 1-4 defensive write semantics (`currentVersion`, no comparison to running daemon, "going-forward" version assertion) traces verbatim to spec §Change 1 "Defensive complement" paragraph; the new `portalSaverWriteVersionFile` package-level seam is implementation scaffolding consistent with the existing `BootstrapAliveCheck` injection pattern documented in spec.
- Task 1-5 comment-revision target (`internal/tmux/portal_saver.go:232-241`) is named verbatim in spec §Change 1 "Update the existing function comment".
- Task 1-6 integration test maps to spec §Testing Requirements > Integration tests #1 and §Acceptance Criteria steady-state #1-#4.
- Task 2-1 plumbing scope ("Signature changes are local to this file", `internal/state/capture.go` untouched) traces to spec §Change 2 first paragraph.
- Task 2-1 shutdown-flush rationale (pass `context.Background()` to preserve non-cancellable on-exit save) traces to spec §Change 2 cancellation semantics ("Shutdown flush behaviour … is unchanged") and §Risk & Rollout ("daemonShutdownFunc does not depend on the cancelled tick's output").
- Tasks 2-2/2-3/2-4 three observation points map verbatim to spec §Change 2 cancellation semantics points 1, 2, 3.
- Task 2-4 non-rollback of completed per-pane writes traces to spec §Change 2 "no half-applied scrollback writes" interpretation (atomic per-pane writes; spec requires no partial commit of `sessions.json`, not per-pane rollback).
- Task 2-5 threshold-from-measurement requirement traces verbatim to spec §Testing Requirements > Integration tests #2 ("Implementation should take that measurement and either confirm 2s as appropriate or adjust the threshold").
- Task 2-5 optional sweep-pressure variant traces to spec §Defect 2 "self-amplifying property".
- Task 2-6 fault-injection harness (sentinel goroutine holding `daemon.lock` via `state.AcquireDaemonLock`, three cascade assertions with exact poll ratios) traces verbatim to spec §Testing Requirements > Integration tests #3.
- Task 2-6 regression-watch suite list traces to spec §Coordination with prior bugfix.

No plan content was found that cannot be traced to a specific specification section. No hallucinated requirements, behaviours, edge cases, or acceptance criteria.

## Notes

Phase ordering (Change 1 + Change 3 in Phase 1, Change 2 in Phase 2) is justified in the plan's "Why this order" prose and aligns with spec §Defect 3 framing ("Fixing Defect 1 makes the disappearance non-load-bearing for the user-visible symptom") and §Risk & Rollout "Low" rating.
