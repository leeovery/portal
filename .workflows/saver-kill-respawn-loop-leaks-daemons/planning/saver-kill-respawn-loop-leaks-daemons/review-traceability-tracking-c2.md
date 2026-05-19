---
status: complete
created: 2026-05-19
cycle: 2
phase: Traceability Review
topic: saver-kill-respawn-loop-leaks-daemons
---

# Review Tracking: Saver Kill-Respawn Loop Leaks Daemons - Traceability (Cycle 2)

## Findings

None. The corrected plan remains a faithful, complete translation of the specification.

## Cycle 2 scope

Cycle 1 was clean. Between cycles a single Minor integrity fix landed: a `Tests` field was added to task 1-1 (`Reframe portalSaverVersionMismatch table tests`). This cycle re-checks traceability of that addition and re-verifies the unchanged remainder of the plan.

## Cycle 2 delta verification — task 1-1 `Tests` field

The newly-added `Tests` block in task 1-1 (phase-1-tasks.md, lines 38-44) lists six entries, each mapping 1:1 to a row of the predicate matrix in spec §Testing Requirements > Unit tests:

| Task 1-1 test name | Spec matrix row |
|---|---|
| `match — stored and current equal, neither dev, returns false` | `0.5.0` / `0.5.0` / nil → `false` |
| `real mismatch — stored and current differ, neither dev, returns true` | `0.5.0` / `0.5.1` / nil → `true` |
| `absent neither dev — read returns ErrVersionFileAbsent, predicate returns true (alive-check is caller's responsibility)` | `""` / `0.5.0` / `ErrVersionFileAbsent` → `true` |
| `non-absent I/O read error — read returns a generic I/O error, predicate returns true` | `""` / `0.5.0` / other I/O error → `true` |
| `stored dev short-circuit — stored=dev forces true regardless of current` | `dev` / `0.5.0` / nil → `true` |
| `current dev short-circuit — current=dev forces true regardless of stored` | `0.5.0` / `dev` / nil → `true` |

The reframing rider attached to test 3 (`alive-check is caller's responsibility`) traces to spec §Testing Requirements > Unit tests prose: *"the predicate still returns `true` for absent ... `EnsurePortalSaverVersion` no longer drives the kill decision from the predicate alone — the alive-check ordering ... is now the authoritative gate."*

No fabricated tests. No missing rows. No content asserting behaviour the spec does not require.

## Direction 1 — Specification → Plan (completeness)

All specification elements remain covered (no regression from cycle 1):

- **Change 1 (alive-check first)** — Tasks 1-3 (gate kill on `BootstrapAliveCheck`), 1-4 (defensive `WriteVersionFile` on alive+absent), 1-5 (revise function comment at lines 232-241), 1-1 (reframed predicate table tests).
- **Change 2 (ctx-aware captureAndCommit)** — Tasks 2-1 (thread ctx + happy-path regression), 2-2 (observation point 1 / entry), 2-3 (observation point 2 / post-enumeration), 2-4 (observation point 3 / between per-pane iterations).
- **Change 3 (WriteVersionFile DEBUG breadcrumb)** — Task 1-2 (under `ComponentDaemon`, prefix `daemon.version write:`, includes version + pid + path).
- **Acceptance Criteria #1-#10** — All ten criteria mapped: Phase 1 acceptance covers steady-state (#1-#5), version-upgrade (#6), and diagnostic (#9); Phase 2 acceptance covers daemon responsiveness (#7, #8) and regression (#10).
- **Testing Requirements** — Predicate table tests (Task 1-1, now with Tests field populated), `EnsurePortalSaverVersion` ordering tests (Task 1-3), `captureAndCommit` cancellation tests (Tasks 2-2/2-3/2-4 with happy-path guard in 2-1), Integration test #1 alive+absent (Task 1-6), Integration test #2 mid-tick SIGHUP (Task 2-5), Integration test #3 lock-loser cascade (Task 2-6).
- **Regression preservation** — `multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart` all listed in Phase 2 acceptance and Task 2-6.
- **What stays unchanged** — `daemon.lock` primitive, `killSaverAndWaitForDaemon`, `BootstrapPortalSaver`, `killBarrierTimeout=5s`, no-daemon path, dev-build handling, `internal/state/capture.go` signatures, per-tick capture algorithm — all reflected in task constraints.
- **Rejected alternatives** (raise timeout, bound capture-pane lines, goroutine restructure, distinguish `ErrVersionFileAbsent` in predicate only) — none re-introduced.
- **Out of Scope** items (hook-registration redundancy, capture-pane line bounding, raising timeout, identifying Defect 3 root cause, goroutine restructure, TUI loading floor, version storage migration) — none addressed by tasks.
- **AC #5 "~520ms reclaimed"** — spec marks this informational, not a hard regression test; plan correctly omits a guarded test for it.

## Direction 2 — Plan → Specification (fidelity)

Every task element continues to trace to specification content. The cycle 2 addition (task 1-1 `Tests` field) introduces no plan-only content — each test name reproduces a row from the spec's predicate matrix verbatim with a reframing note that is directly quoted from the spec.

All prior-cycle traceability findings remain valid:

- Task 1-1 framing change (predicate-as-one-input) traces to spec §Testing Requirements > Unit tests.
- Task 1-2 prefix contract `daemon.version write:` traces verbatim to spec §Change 3.
- Task 1-3 six-row decision matrix traces verbatim to spec §Change 1 decision matrix.
- Task 1-4 defensive write semantics trace verbatim to spec §Change 1 "Defensive complement" paragraph.
- Task 1-5 comment-revision target (`internal/tmux/portal_saver.go:232-241`) is named verbatim in spec §Change 1.
- Task 1-6 integration test maps to spec §Testing Requirements > Integration tests #1 and §Acceptance Criteria steady-state #1-#4.
- Task 2-1 plumbing scope traces to spec §Change 2 first paragraph.
- Task 2-1 shutdown-flush rationale (`context.Background()`) traces to spec §Change 2 cancellation semantics and §Risk & Rollout.
- Tasks 2-2 / 2-3 / 2-4 three observation points map verbatim to spec §Change 2 cancellation semantics points 1, 2, 3.
- Task 2-4 non-rollback of completed per-pane writes traces to spec §Change 2 "no half-applied scrollback writes" interpretation.
- Task 2-5 threshold-from-measurement requirement traces verbatim to spec §Testing Requirements > Integration tests #2.
- Task 2-5 optional sweep-pressure variant traces to spec §Defect 2 "self-amplifying property".
- Task 2-6 fault-injection harness traces verbatim to spec §Testing Requirements > Integration tests #3.
- Task 2-6 regression-watch suite list traces to spec §Coordination with prior bugfix.

No hallucinated requirements, behaviours, edge cases, or acceptance criteria.

## Notes

The integrity fix (adding `Tests` to task 1-1) brought the task into compliance with `task-design.md` field requirements without expanding scope or introducing non-spec content. Traceability remains clean across both directions.
