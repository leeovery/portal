---
status: complete
created: 2026-05-11
cycle: 3
phase: Traceability Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

## Analysis Summary

### Direction 1: Specification → Plan (completeness)

Every actionable specification element has corresponding plan coverage:

| Spec Element | Covered By |
|---|---|
| Fix Part 1 → Behaviour (LOCK_EX\|LOCK_NB, success/contention paths, exit 0 on EWOULDBLOCK) | Tasks 1.1, 1.2 |
| Fix Part 1 → Fd retention is load-bearing (package-level var, no GC-closeable finalizer) | Task 1.2 |
| Fix Part 1 → FD_CLOEXEC | Task 1.1 (helper sets it), 1.2 (preserved on retention) |
| Fix Part 1 → Lock-file create/open semantics (mode 0600, no MkdirAll, open(2) errors fatal) | Task 1.1 |
| Fix Part 1 → Placement (before WritePIDFile in cmd/state_daemon.go) | Task 1.2 |
| Fix Part 1 → Seamed for testing (`var lockAcquire = unix.Flock`) | Task 1.1 |
| Fix Part 1 → Loser-daemon session aftermath (empty-session + dead-pane shapes) | Task 1.4 (both sub-cases) |
| Fix Part 1 → Compatibility with pidfile (acquire before WritePIDFile) | Tasks 1.1, 1.2 |
| Fix Part 2 → Behaviour (7-step protocol) | Task 2.1 |
| Fix Part 2 → 5s timeout sized above 3.9s cold-sweep ceiling | Task 2.1 |
| Fix Part 2 → Both kill sites use barrier (EnsurePortalSaverVersion + BootstrapPortalSaver) | Task 2.2 |
| Fix Part 2 → Shared helper (`killSaverAndWaitForDaemon`) | Tasks 2.1, 2.2 |
| Fix Part 2 → Test seams (ReadPIDFile + IsProcessAlive + clock) | Task 2.1 |
| Fix Part 2 → Interaction with @portal-restoring marker | Task 2.2 Edge Cases |
| AC → Singleton invariant | Task 2.3 (integration test) |
| AC → Clean handover on common case (no WARN) | Task 2.1 |
| AC → Graceful degradation under timeout | Tasks 2.1 + 1.1/1.2 (lock as safety net) + 1.4 (recovery) |
| AC → No regression on steady-state critical path | Task 2.2 |
| AC → Pidfile remains coherent | Task 1.2 |
| AC → Lock cleanup on crash | Task 1.3 |
| AC → Observability (two WARN lines, no logs on common case) | Tasks 1.2 (lock contention WARN), 2.1 (barrier timeout WARN), 2.2 (production logger wiring) |
| Test Strategy → Unit tests daemon singleton lock | Tasks 1.1, 1.2 |
| Test Strategy → Unit tests synchronous kill barrier | Tasks 2.1, 2.2 |
| Test Strategy → Integration test singleton invariant under real tmux | Task 2.3 |
| Test Strategy → Regression test flock-loser recovery | Tasks 1.3 (kernel cleanup half), 1.4 (tolerant-kill-and-recreate half) |
| Test Strategy → Test independence (per-test t.TempDir(), no t.Parallel()) | All tasks |
| Risk and Rollout → Documentation note re: CLAUDE.md state row | Task 1.2 acceptance criterion |

### Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every task's content traces to a specific specification section. Sampled checks:

- Task 1.1's `ErrDaemonLockHeld` sentinel, mode 0600, no MkdirAll, FD_CLOEXEC, helper-accepts-stateDir-parameter — all from spec §"Fix Part 1 → Behaviour", §"Lock-file create / open semantics", §"Placement and structure", §"Test independence".
- Task 1.1 AC about FIFO sweep paths being read-only is from spec §"Potentially affected".
- Task 1.2's package-level `daemonLockFile` var, WARN/exit-0 on EWOULDBLOCK, ERROR/exit-nonzero on other open errors, no `runtime.SetFinalizer` — all from spec §"Fix Part 1 → Behaviour", §"Fd retention is load-bearing".
- Task 1.2 AC about CLAUDE.md doc note is from spec §"Risk and Rollout → Documentation".
- Task 1.3's kernel-releases-on-fd-close test directly mirrors spec §"Acceptance Criteria → Lock cleanup on crash".
- Task 1.4's two sub-cases (empty-session + dead-pane) directly mirror spec §"Loser-daemon session aftermath".
- Task 2.1's four seams (`killBarrierReadPID`, `killBarrierIsAlive`, `killBarrierPollInterval`, `killBarrierTimeout`) plus logger seam mirror spec §"Test seams" and §"Observability".
- Task 2.1's 50 ms / 5 s defaults trace to spec §"Behaviour" (50–100 ms cadence) and §"Timeout rationale".
- Task 2.2's both-call-sites wiring traces verbatim to spec §"Both kill sites use the barrier".
- Task 2.2's `SetBarrierLogger` + bootstrapadapter wiring is the planning-level mechanism chosen to satisfy spec §"Observability" — a planning decision within spec-granted freedom, not a hallucination.
- Task 2.3's `pgrep -P <server_pid> -f 'portal state daemon' | wc -l == 1` assertion, direct daemon.version write between calls, no new portalSaverVersionMismatch seam — all from spec §"Test Strategy → Integration test".

### Out-of-Scope Discipline

All five spec §"Out of Scope" items remain absent from the plan:

- Bounding capture-pane scrollback depth — not in plan
- Tightening portalSaverVersionMismatch — not in plan
- Cheaper change-detection before capture-pane — not in plan
- Stale `; exec $SHELL` wrappers — not in plan
- Daemon tick loop restructure — not in plan

Plan correctly observes the spec's scope boundaries.

### Verdict

Cycle 3 traceability review is clean. The plan is ready to proceed in the workflow.
