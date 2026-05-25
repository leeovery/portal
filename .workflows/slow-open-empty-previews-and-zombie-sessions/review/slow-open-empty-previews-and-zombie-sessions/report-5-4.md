TASK: 5-4 — Unit test counter reset on transient-then-recover via stubbed probe

STATUS: Complete

SPEC CONTEXT: Component D acceptance "No false-positive exit on legitimate transient" — stub probe to return absent for k<N then present; daemon does NOT exit, counter resets. Reset-not-decrement invariant is load-bearing.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon_self_supervision_test.go`; production reset at `cmd/state_daemon.go:243` (`consecutiveAbsenceTicks = 0`)
- `defaultDaemonTickLoop` performs hard reset to 0 on probe-true (not decrement) — regression guard's premise is real

TESTS:
- Status: Adequate
- Four tests directly satisfy 5-4 enumeration:
  1. `TestSelfSupervisionCounter_ResetsFullyOnFirstProbeTrue` (505-549) — pins reset-not-decrement via `(false × N-1, true, false × N-1)`; under buggy decrement counter would reach `2N-3 ≥ N` and eject
  2. `TestSelfSupervisionCounter_BoundaryKEqualsNMinus1` (555-592) — k=N-1 exactly, 5 cycles
  3. `TestSelfSupervisionCounter_ManyAbsentPresentCycles` (597-641) — varied absent-streak lengths
  4. `TestSelfSupervisionCounter_IncrementsUniformlyOnProbeFalse` (651-701) — uniform-increment via prelude
- `scriptedProbe` helper (465-477) provides clean script-driven probe with safe post-script tail
- `TickerPeriod = 1ms`; ctx timeouts 100ms-2s; runtime within ~200ms budget per test
- Seams swapped via `withSaverMembershipProbeFake`/`withOsExitFake`/`withDaemonShutdownFuncFake` via `t.Cleanup`
- Parameterised on `selfSupervisionHysteresisTicks`; no `t.Parallel`

CODE QUALITY:
- Project conventions: Followed; `sync/atomic` for cross-goroutine counters
- SOLID: Good; helpers reusable
- Complexity: Low; linear bodies
- Modern idioms: `context.WithTimeout`, `t.Cleanup`, atomic counters
- Readability: Good; "buggy decrement impl" counterfactual spelled out

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `scriptedProbe`'s post-script tail returns `true` — deliberate, documented; future maintainer might "fix" this unnecessarily
- [idea] `const cycles = 5` satisfies task floor exactly; floor not margin
- [quickfix] Docstring on `TestSelfSupervisionCounter_IncrementsUniformlyOnProbeFalse` mentions 5-2 invariant ("seam returns single bool"); cosmetic
