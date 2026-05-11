---
status: complete
created: 2026-05-11
cycle: 2
phase: Traceability Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

## Notes

Cycle 1 produced three findings (FIFO sweep confirmation, `@portal-restoring` marker extension acknowledgement, `CLAUDE.md` `state` row documentation), all of which were applied and are present in the cycle 2 plan:

- Task 1.1 acceptance criteria now includes the daemon-side FIFO sweep read-only confirmation bullet.
- Task 2-2 edge cases now surface the `@portal-restoring` marker lifetime under barrier timeout (with the explicit "do not tighten / narrow / add diagnostic" note).
- Task 1.2 acceptance criteria now includes the `CLAUDE.md` `state` package row update.

Fresh-eyes Direction 1 (Spec → Plan) traced every specification element to plan coverage:

- Root Cause Defect 1 (no singleton enforcement) → Phase 1 tasks 1.1, 1.2.
- Root Cause Defect 2 (bootstrap does not synchronise with killed daemon's exit) → Phase 2 tasks 2-1, 2-2.
- Fix Part 1 Behaviour (LOCK_EX|LOCK_NB, fd retention, FD_CLOEXEC, EWOULDBLOCK → WARN+exit 0, open errors → ERROR+non-zero, mode 0600, no stateDir creation, seam pattern, placement before WritePIDFile) → tasks 1.1, 1.2.
- Fix Part 1 Loser-daemon session aftermath (empty-session, dead-pane-under-remain-on-exit) → task 1.4.
- Fix Part 2 Behaviour (steps 1–7 of barrier flow, defensive PID handling, no blocking, non-fatal timeout) → task 2-1.
- Fix Part 2 Both kill sites use the barrier → task 2-2.
- Fix Part 2 Test seams → task 2-1.
- Fix Part 2 `@portal-restoring` marker interaction → task 2-2 edge cases.
- Acceptance Criteria § Singleton invariant → task 2-3 integration test.
- Acceptance Criteria § Clean handover on the common case → task 2-1.
- Acceptance Criteria § Graceful degradation under timeout → tasks 2-1, 2-2.
- Acceptance Criteria § No regression on the steady-state critical path → task 2-2.
- Acceptance Criteria § Pidfile remains coherent → task 1.2.
- Acceptance Criteria § Lock cleanup on crash → task 1.3.
- Acceptance Criteria § Observability (exactly two new WARN lines, presence + level asserted, not literal text) → tasks 1.2 + 2-1.
- Test Strategy § Unit tests — daemon singleton lock → tasks 1.1, 1.2.
- Test Strategy § Unit tests — synchronous kill barrier → tasks 2-1, 2-2.
- Test Strategy § Integration test → task 2-3.
- Test Strategy § Regression test — flock-loser recovery → task 1.4 (recovery aftermath) + task 1.3 (kernel cleanup half).
- Risk and Rollout § cross-platform `flock` semantics (darwin + linux) → task 1.3 acceptance bullet.

Direction 2 (Plan → Spec) traced every plan-specific element back to the specification:

- `ErrDaemonLockHeld` sentinel, `DaemonLock(dir)` accessor, `lockAcquire` seam — implementation of spec's contention-distinguishability + seam-pattern requirements.
- `killBarrierReadPID` / `killBarrierIsAlive` / `killBarrierPollInterval` / `killBarrierTimeout` seams — implementation of spec's Test Seams requirement ("ReadPIDFile, polling clock, IsProcessAlive needed for injection").
- `BarrierLogger` interface + `killBarrierLogger` var + recording fake — implementation of spec's Observability requirement (presence + WARN level testable without real log emission).
- `killSaverAndWaitForDaemonFn` package-level var (task 2-2) — implementation of spec's Test Strategy "Both paths must invoke the shared helper. Asserted by triggering each path independently and recording barrier invocation."
- 50 ms polling cadence (task 2-1) — within spec's stated "50–100 ms is a reasonable starting point" envelope.
- 5 s timeout (task 2-1) — exact spec value, sized above 3.9 s cold-sweep ceiling.
- Integration test version strings `"v-test-1"` / `"v-test-0-old"` (task 2-3) — mechanics of triggering the real `portalSaverVersionMismatch` comparison, which spec explicitly requires ("directly write a different value into `<stateDir>/daemon.version` between calls").
- `exec.LookPath("portal")` precondition skip (task 2-3) — test-hygiene extension of spec's `tmuxtest.SkipIfNoTmux(t)` skip pattern; needed because the saver session launches the user-installed `portal` binary.
- Bounded 3 s settle window for pgrep convergence (task 2-3) — derived from spec's 1 s tick + 100 ms barrier-poll cadence as an asynchronous post-return settle window.
- Optional `BarrierTimeoutPath` sub-test (task 2-3) — marked explicitly optional; validates Phase 1 floor under Phase 2 timeout without adding a load-bearing assertion not in the spec.

No hallucinated content found. All plan content traces back to specific sections of the specification, and all specification elements are represented in plan tasks with sufficient depth that an implementer would not need to return to the specification.
