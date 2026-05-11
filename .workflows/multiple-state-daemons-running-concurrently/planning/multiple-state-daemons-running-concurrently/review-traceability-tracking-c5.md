---
status: complete
created: 2026-05-11
cycle: 5
phase: Traceability Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification.

## Analysis Summary

Cycle 5 is a final fresh-eyes traceability pass on the corrected plan. Cycles 3 and 4 were clean; cycle 5 confirms convergence with no new findings.

### Direction 1: Specification → Plan (completeness)

Independent re-trace of every actionable specification element to plan coverage. All coverage remains intact.

| Spec Element | Covered By |
|---|---|
| Problem Statement → Impact (severity, scope, shared-state corruption) | Phase 1+2 goals; Task 2.1 Problem cites cold-sweep + tick loop structure |
| Potentially affected → FIFO sweep paths (confirm read-only on daemon side) | Task 1.1 acceptance bullet |
| Root Cause → Defect 1 (no singleton enforcement) | Tasks 1.1, 1.2 |
| Root Cause → Defect 2 (bootstrap does not synchronise with killed daemon's exit) | Tasks 2.1, 2.2 |
| Root Cause → Why old daemon survives kill (tick loop, captureAndCommit cost) | Task 2.1 Problem |
| Fix Part 1 → Behaviour (LOCK_EX\|LOCK_NB, success/contention paths, exit 0 on EWOULDBLOCK) | Tasks 1.1, 1.2 |
| Fix Part 1 → Fd retention is load-bearing (package-level var, no GC-closeable finalizer) | Task 1.2 |
| Fix Part 1 → FD_CLOEXEC | Task 1.1 (helper sets it), 1.2 (preserved on retention) |
| Fix Part 1 → Lock-file create/open semantics (mode 0600, no MkdirAll, open(2) errors fatal) | Task 1.1 |
| Fix Part 1 → Placement (before WritePIDFile in cmd/state_daemon.go) | Task 1.2 |
| Fix Part 1 → Seamed for testing (`var lockAcquire = unix.Flock`) | Task 1.1 |
| Fix Part 1 → Why fail-fast not blocking | Task 1.1 Context |
| Fix Part 1 → Loser-daemon session aftermath (empty-session + dead-pane shapes) | Task 1.4 (both sub-cases) |
| Fix Part 1 → Why a lock and not O_EXCL pidfile create | Settled spec decision; correctly not re-litigated |
| Fix Part 1 → Compatibility with pidfile (acquire before WritePIDFile, pidfile authoritative) | Tasks 1.1, 1.2 |
| Fix Part 2 → Behaviour (7-step protocol) | Task 2.1 Do section |
| Fix Part 2 → Timeout rationale (5s above 3.9s cold-sweep ceiling) | Task 2.1 |
| Fix Part 2 → Both kill sites use barrier (EnsurePortalSaverVersion + BootstrapPortalSaver) | Task 2.2 |
| Fix Part 2 → Shared helper (`killSaverAndWaitForDaemon`) | Tasks 2.1, 2.2 |
| Fix Part 2 → Test seams (ReadPIDFile + IsProcessAlive + clock) | Task 2.1 |
| Fix Part 2 → Critical-path latency budget | Task 2.2 Edge Cases (steady-state path verification) |
| Fix Part 2 → Interaction with @portal-restoring marker (extended 5s acceptable) | Task 2.2 Edge Cases (with "do not tighten" guidance) |
| Fix Part 2 → Why not spin-wait inside BootstrapPortalSaver | Settled spec decision; barrier scoped to kill paths |
| Fix Part 2 → Why not treat session presence as singleton signal | Settled spec decision; correctly not re-litigated |
| AC → Singleton invariant | Task 2.3 (integration test) |
| AC → Clean handover on common case (no WARN) | Task 2.1 |
| AC → Graceful degradation under timeout | Tasks 2.1 + 1.1/1.2 (lock safety net) + 1.4 (recovery) |
| AC → No regression on steady-state critical path | Task 2.2 |
| AC → Pidfile remains coherent | Task 1.2 |
| AC → Lock cleanup on crash | Task 1.3 |
| AC → Observability (two WARN lines, no logs on common case) | Tasks 1.2 (lock contention WARN), 2.1 (barrier timeout WARN), 2.2 (production logger wiring) |
| Test Strategy → Unit tests daemon singleton lock | Tasks 1.1, 1.2 |
| Test Strategy → Unit tests synchronous kill barrier | Tasks 2.1, 2.2 |
| Test Strategy → Integration test singleton invariant under real tmux | Task 2.3 |
| Test Strategy → Regression test flock-loser recovery | Tasks 1.3 (kernel cleanup half), 1.4 (tolerant-kill-and-recreate half) |
| Test Strategy → Test independence (per-test t.TempDir(), no t.Parallel()) | All tasks |
| Risk and Rollout → Cross-platform flock semantics (darwin + linux) | Task 1.3 acceptance bullet |
| Risk and Rollout → Documentation note re: CLAUDE.md state row | Task 1.2 acceptance criterion |
| Risk and Rollout → Upgrade behaviour (orphan drain across bootstraps) | Operational consequence of Task 1.1/1.4; no separate implementable item required |

### Direction 2: Plan → Specification (fidelity / anti-hallucination)

Re-traced every plan-introduced specific (sentinels, seam names, default values, version strings, polling cadences, file locations) back to the specification. No hallucinated content found.

Independent spot-checks:

- Task 1.1's `ErrDaemonLockHeld` sentinel, mode 0600, no MkdirAll, FD_CLOEXEC, `lockAcquire` seam, helper-accepts-stateDir-parameter — all from spec §"Fix Part 1 → Behaviour", §"Lock-file create / open semantics", §"Placement and structure", §"Test independence".
- Task 1.1's file location `internal/state/daemon_lock.go` traces to spec §"Placement and structure" ("may live in `internal/state` (alongside the pidfile helpers) for symmetry").
- Task 1.1 FIFO-sweep read-only confirmation bullet maps to spec §"Potentially affected → FIFO sweep paths".
- Task 1.2's package-level `daemonLockFile` var, WARN/exit-0 on EWOULDBLOCK, ERROR/exit-nonzero on other open errors, no `runtime.SetFinalizer` introduction — all from spec §"Fix Part 1 → Behaviour", §"Fd retention is load-bearing".
- Task 1.2's CLAUDE.md doc note acceptance bullet directly maps to spec §"Risk and Rollout → Documentation" ("to be evaluated during planning").
- Task 1.3's kernel-releases-on-fd-close test mirrors spec §"Acceptance Criteria → Lock cleanup on crash". Optional SIGKILL variant correctly flagged as strength bonus, not correctness gate.
- Task 1.4's two sub-cases (empty-session + dead-pane) mirror spec §"Loser-daemon session aftermath". The Phase-1-without-barrier convergence note is a correct planning interpretation — the existing tolerant-kill-and-recreate branch is the convergence mechanism prior to Phase 2 landing.
- Task 2.1's four seams (`killBarrierReadPID`, `killBarrierIsAlive`, `killBarrierPollInterval`, `killBarrierTimeout`) plus logger seam mirror spec §"Test seams" and §"Observability".
- Task 2.1's 50 ms polling default sits within spec's "50–100 ms is a reasonable starting point" envelope.
- Task 2.1's 5 s timeout default is the exact spec value, sized above the 3.9 s cold-sweep ceiling.
- Task 2.1's `EPERM` edge case and "negative or zero prior PID" edge case cite pre-existing portal semantics (`internal/state/daemon_state.go:70-72`, `:62-64`) — code-trace observations, not invented requirements.
- Task 2.2's both-call-sites wiring traces verbatim to spec §"Both kill sites use the barrier" ("these are not alternatives").
- Task 2.2's `SetBarrierLogger` + bootstrapadapter wiring is the planning-chosen mechanism for satisfying spec §"Observability" production WARN emission. Spec mandates the WARN exists and is tested; the injection mechanism is a planning concern within spec-granted freedom.
- Task 2.2 @portal-restoring marker edge case maps to spec §"Interaction with @portal-restoring marker" with explicit "do not tighten timeout / narrow marker / add diagnostic" guidance preventing future regression against the spec's explicitly accepted property.
- Task 2.3's `pgrep -P <server_pid> -f 'portal state daemon' | wc -l == 1` assertion, direct `daemon.version` write between calls, no new `portalSaverVersionMismatch` seam — all from spec §"Test Strategy → Integration test".
- Task 2.3's version strings `"v-test-1"` / `"v-test-0-old"` — concretisation of spec's "directly write a different value"; spec leaves the literal values to planning.
- Task 2.3's `exec.LookPath("portal")` precondition skip — logical extension of spec's `tmuxtest.SkipIfNoTmux(t)` pattern; the saver session launches `portalSaverCommand = "portal state daemon"` so the user-installed binary must be on PATH.
- Task 2.3's 3 s settle window — derived from spec's 1 s tick interval + barrier-poll cadence. Reasonable planning-level concretisation.
- Task 2.3's optional `BarrierTimeoutPath` sub-test correctly framed as optional strength bonus.

### Out-of-Scope Discipline

All five spec §"Out of Scope" items remain absent from the plan:

- Bounding `capture-pane -S -<N>` scrollback depth — not in plan
- Tightening `portalSaverVersionMismatch` — not in plan
- Cheaper change-detection before `capture-pane` — not in plan
- Stale `; exec $SHELL` wrappers from hydrate helper — not in plan
- Daemon tick loop restructure for prompt cancellation — not in plan

Spec §"Recycle-path silence is preserved" explicit non-action (no new diagnostic/info log at recycle decision point) remains correctly honoured — the plan introduces exactly the two WARN lines mandated by the Observability AC, no additional info-level recycle logs.

### Convergence Assessment

Cycles 1–2 produced 8 findings (all fixed). Cycles 3 and 4 traceability passes were clean. Cycle 5 fresh-eyes pass is also clean — no new findings, no traceability regressions, no missed spec elements. The plan has converged.

### Verdict

Cycle 5 traceability review is clean. The plan is a faithful, complete translation of the specification and is ready to proceed in the workflow.
