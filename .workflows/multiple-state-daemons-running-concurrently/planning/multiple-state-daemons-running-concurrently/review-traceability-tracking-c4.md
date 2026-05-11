---
status: complete
created: 2026-05-11
cycle: 4
phase: Traceability Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification after cycle 3's consistency cleanups.

## Analysis Summary

Cycle 4 is a fresh-eyes traceability pass on the corrected plan after cycle 3's consistency review.

### Direction 1: Specification → Plan (completeness)

Re-traced every actionable specification element to plan coverage. All coverage from cycle 3 is intact; cycle 3's consistency-driven edits did not remove or weaken any spec → plan trace.

| Spec Element | Covered By |
|---|---|
| Problem Statement → Impact (severity, scope, shared-state corruption) | Phase 1+2 goals; Task 1.1 (lock as structural floor); Task 2.1 (barrier to keep recycle quiet) |
| Potentially affected → FIFO sweep paths (confirm read-only on daemon side) | Task 1.1 acceptance bullet |
| Root Cause → Defect 1 (no singleton enforcement) | Tasks 1.1, 1.2 |
| Root Cause → Defect 2 (bootstrap does not synchronise with killed daemon's exit) | Tasks 2.1, 2.2 |
| Root Cause → Why old daemon survives kill (tick loop, captureAndCommit cost) | Task 2.1 Problem (cold-sweep ceiling + tick loop structure cited verbatim) |
| Fix Part 1 → Behaviour (LOCK_EX\|LOCK_NB, success/contention paths, exit 0 on EWOULDBLOCK) | Tasks 1.1, 1.2 |
| Fix Part 1 → Fd retention is load-bearing (package-level var, no GC-closeable finalizer) | Task 1.2 |
| Fix Part 1 → FD_CLOEXEC | Task 1.1 (helper sets it), 1.2 (preserved on retention) |
| Fix Part 1 → Lock-file create/open semantics (mode 0600, no MkdirAll, open(2) errors fatal) | Task 1.1 |
| Fix Part 1 → Placement (before WritePIDFile in cmd/state_daemon.go) | Task 1.2 |
| Fix Part 1 → Seamed for testing (`var lockAcquire = unix.Flock`) | Task 1.1 |
| Fix Part 1 → Why fail-fast not blocking | Phase 1 goal narrative + Task 1.1 Context |
| Fix Part 1 → Loser-daemon session aftermath (empty-session + dead-pane shapes) | Task 1.4 (both sub-cases) |
| Fix Part 1 → Why a lock and not O_EXCL pidfile create | Task 1.1 design (flock chosen; not litigated in plan, which is correct — spec decision is settled) |
| Fix Part 1 → Compatibility with pidfile (acquire before WritePIDFile, pidfile authoritative) | Tasks 1.1, 1.2 |
| Fix Part 2 → Behaviour (7-step protocol) | Task 2.1 (steps 1-7 mapped into the Do section) |
| Fix Part 2 → 5s timeout sized above 3.9s cold-sweep ceiling | Task 2.1 |
| Fix Part 2 → Both kill sites use barrier (EnsurePortalSaverVersion + BootstrapPortalSaver) | Task 2.2 |
| Fix Part 2 → Shared helper (`killSaverAndWaitForDaemon`) | Tasks 2.1, 2.2 |
| Fix Part 2 → Test seams (ReadPIDFile + IsProcessAlive + clock) | Task 2.1 |
| Fix Part 2 → Critical-path latency budget | Task 2.2 Edge Cases (steady-state path verification) |
| Fix Part 2 → Interaction with @portal-restoring marker (extended 5s acceptable) | Task 2.2 Edge Cases |
| Fix Part 2 → Why not spin-wait inside BootstrapPortalSaver | Phase 2 goal narrative implicitly (barrier scoped to kill paths) |
| Fix Part 2 → Why not treat session presence as singleton signal | Settled spec decision; correctly not re-litigated in plan |
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

Re-traced every plan-introduced specific (sentinels, seam names, defaults, version strings, polling cadences) back to the specification. No hallucinated content found.

Sampled checks:

- Task 1.1's `ErrDaemonLockHeld` sentinel, mode 0600, no MkdirAll, FD_CLOEXEC, helper-accepts-stateDir-parameter — all from spec §"Fix Part 1 → Behaviour", §"Lock-file create / open semantics", §"Placement and structure", §"Test independence".
- Task 1.1 FIFO-sweep read-only confirmation bullet maps to spec §"Potentially affected → FIFO sweep paths".
- Task 1.2's package-level `daemonLockFile` var, WARN/exit-0 on EWOULDBLOCK, ERROR/exit-nonzero on other open errors, no `runtime.SetFinalizer` — all from spec §"Fix Part 1 → Behaviour", §"Fd retention is load-bearing".
- Task 1.2 CLAUDE.md doc note acceptance bullet — directly from spec §"Risk and Rollout → Documentation" ("to be evaluated during planning").
- Task 1.3's kernel-releases-on-fd-close test directly mirrors spec §"Acceptance Criteria → Lock cleanup on crash". Optional SIGKILL variant correctly flagged as strength bonus not correctness gate (matches spec's "abrupt exit / SIGKILL simulation" — simulation, not requirement).
- Task 1.4's two sub-cases (empty-session + dead-pane) directly mirror spec §"Loser-daemon session aftermath". Task 1.4's note that the "barrier" referenced in the spec test phrasing is not yet present (Phase 1 lands before Phase 2) is a correct, defensible planning interpretation — the convergence still happens via the existing tolerant-kill-and-recreate branch.
- Task 2.1's four seams (`killBarrierReadPID`, `killBarrierIsAlive`, `killBarrierPollInterval`, `killBarrierTimeout`) plus logger seam mirror spec §"Test seams" and §"Observability".
- Task 2.1's 50 ms polling default is within spec's stated "50–100 ms is a reasonable starting point" envelope.
- Task 2.1's 5 s timeout default is the exact spec value, sized above the 3.9 s cold-sweep ceiling.
- Task 2.1 §"`state.IsProcessAlive` returns true via `EPERM`" edge case cites the daemon-state implementation at `internal/state/daemon_state.go:70-72` — a code-trace observation about pre-existing portal semantics, not a hallucinated requirement; the spec's barrier behaviour (steps 1-7) is implemented against this existing primitive without reshaping it.
- Task 2.1 §"Negative or zero prior PID" edge case cites `internal/state/daemon_state.go:62-64` — same pattern: traceable to existing portal semantics, not a new requirement.
- Task 2.2's both-call-sites wiring traces verbatim to spec §"Both kill sites use the barrier".
- Task 2.2's `SetBarrierLogger` + bootstrapadapter wiring is a planning-level mechanism chosen to satisfy spec §"Observability" requirements — not a hallucination, since the spec mandates the WARN line is asserted in tests and the production code must emit it, but leaves the logger-injection mechanism to planning.
- Task 2.2 @portal-restoring marker edge case maps directly to spec §"Interaction with @portal-restoring marker" with the explicit "do not tighten timeout / narrow marker / add diagnostic" note ensuring future maintainers do not regress against the spec's explicitly accepted property.
- Task 2.3's `pgrep -P <server_pid> -f 'portal state daemon' | wc -l == 1` assertion, direct daemon.version write between calls, no new portalSaverVersionMismatch seam — all from spec §"Test Strategy → Integration test".
- Task 2.3's version strings `"v-test-1"` / `"v-test-0-old"` — mechanics of triggering the real `portalSaverVersionMismatch` comparison; spec requires "directly write a different value", does not specify the string values.
- Task 2.3's `exec.LookPath("portal")` precondition skip — test-hygiene extension of spec's `tmuxtest.SkipIfNoTmux(t)` skip pattern; needed because the saver session launches the user-installed `portal` binary. Not a hallucinated requirement, a logical consequence of the integration test's design.
- Task 2.3's bounded 3 s settle window — derived from spec's 1 s tick + 100 ms barrier-poll cadence. Reasonable planning-level concretisation of "after both calls return" + "after a bounded settle window".
- Task 2.3's optional `BarrierTimeoutPath` sub-test marked explicitly optional; correctly framed as strength bonus, not load-bearing.

### Out-of-Scope Discipline

All five spec §"Out of Scope" items remain absent from the plan:

- Bounding capture-pane scrollback depth — not in plan
- Tightening portalSaverVersionMismatch — not in plan
- Cheaper change-detection before capture-pane — not in plan
- Stale `; exec $SHELL` wrappers — not in plan
- Daemon tick loop restructure for prompt cancellation — not in plan

Spec §"Recycle-path silence" explicit non-action (no new diagnostic/info log at recycle decision point) is correctly honoured — the plan introduces exactly the two WARN lines mandated by the Observability AC, no additional info-level recycle logs.

Plan correctly observes the spec's scope boundaries.

### Verdict

Cycle 4 traceability review is clean. The cycle 3 cleanups did not introduce any traceability regressions, and no previously-missed spec elements surfaced under fresh-eyes inspection. The plan is a faithful, complete translation of the specification and is ready to proceed in the workflow.
