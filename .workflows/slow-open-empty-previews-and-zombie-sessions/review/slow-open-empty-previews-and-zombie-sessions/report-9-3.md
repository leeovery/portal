TASK: 9-3 — Delete waitForAnyDaemonPID (functionally identical to waitForDaemonPID)

STATUS: Complete

SPEC CONTEXT: c3 duplication finding #2. Both helpers polled `state.ReadPIDFile` via `tmuxtest.PollUntil` for expected PID. `waitForAnyDaemonPID`'s docstring falsely claimed distinction.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/upgrade_path_integration_test.go:380-404` — sole surviving `waitForDaemonPID(t, stateDir, expectedPID)`
  - `cmd/bootstrap/composition_e2e_harness_integration_test.go:188` — call site for `legitimateDaemonPID`
  - `cmd/bootstrap/composition_e2e_harness_integration_test.go:204` — call site for `orphan1.Process.Pid` (migrated to expected-PID form)
- Grep confirms zero remaining references to `waitForAnyDaemonPID` or `compositeOrphanPID{Timeout,Tick}` in production or test code
- Constants reconciled to `upgradePathPIDFileTimeout` (3s) / `upgradePathPIDFilePollTick` (50ms)

TESTS:
- Status: Adequate (refactor — coverage unchanged)
- Existing orphan-sweep / composite-harness integration tests exercise both call sites; both assert convergence on specific expected PID

CODE QUALITY:
- All good — single helper, single responsibility

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
