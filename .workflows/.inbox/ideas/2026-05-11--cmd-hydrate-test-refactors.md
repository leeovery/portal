# cmd-layer hydrate test refactors

Four independent micro-refactors in `cmd/state_hydrate_test.go` that share a file and disposition. Best landed together as a single "tighten cmd hydrate test scaffolding" pass.

1. **EISDIR-via-mkdir fixture helper.** The pattern `os.Mkdir(hooksPath, 0o700)` to force `os.ReadFile` to fail with EISDIR recurs at `cmd/state_hydrate_test.go:1517-1521` (`TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning`) and `cmd/state_hydrate_test.go:1576-1580` (`TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning`). Extract a `seedUnreadableHookStore(t, dir)` helper.

2. **Table-driven convergence on `OpenFIFO` seam.** `TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning` (state_hydrate_test.go:1504) and `TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning` (state_hydrate_test.go:1565) are near-identical except for the `OpenFIFO` seam (timeout vs signal-arrived). A table-driven test parameterised on `OpenFIFO` would express the "both branches share `execShellOrHookAndExit`" invariant more compactly. Convergence of recovery paths onto a single exec contract is the whole point of Fix 2 — the duplication may be deliberate for spec-traceability, weigh that before collapsing.

3. **Sleep-ownership subtest table.** The runHydrate-level lower-bound timing test (`TestHydrate_Timeout_PreservesSettleSleepBeforeExec` at line 1050) and the handler-level direct test (`TestHydrate_TimeoutHandler_OrderingAndTimingInvariants` at line 1212) could be co-located as adjacent subtests in a single `TestHydrate_Timeout_SleepOwnership` table — would make the "sleep lives in runHydrate, not the handler" contract more obvious to future readers.

4. **`makeAndSignalFIFO` companion helper.** Many call sites pair `fifo := makeFIFO(...); signalFIFOAsync(t, fifo)`. A 2→1 line collapse at 30+ sites via an optional `makeAndSignalFIFO(t, dir) string` helper would extend the task 4-5 cleanup further. Plan marked this companion as optional.

Source: review of killed-sessions-resurrect-on-restart/killed-sessions-resurrect-on-restart
