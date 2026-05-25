TASK: 6-3 — Invoke bootstrap and assert pgrep convergence to 1 within 6 s

STATUS: Complete

SPEC CONTEXT: Spec § Composite step 5 — pgrep returns 1 within 6s of EnsureSaver entry. Spec § End-State forbids "no such session: _portal-saver" and "prior daemon did not exit within 5s" under steady state.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/composition_e2e_convergence_integration_test.go` (192 lines, single test `TestCompositeBootstrap_ConvergesPgrepToOneWithin6s`)
- Harness consumes `setupCompositeHarness` (3-daemon pre-state)
- Bootstrap: directly calls production primitives `bootstrapadapter.NewOrphanSweeper(...).SweepOrphanDaemons()` + `tmux.BootstrapPortalSaver(client, stateDir)` — functionally equivalent to Orchestrator.Run steps 4-5
- Timing anchor: `start := time.Now()` immediately before SweepOrphanDaemons (line 93). File-header documents conservative vs spec's "entering EnsureSaver" anchor — strictly tightens assertion
- Convergence: `convergencePGrepTimeout = 6 * time.Second`; remaining-budget arithmetic preserves "within 6s of bootstrap entry"
- Survivor identity: `waitForSaverPanePID` (3s) vs `readSaverPanePID` (100ms) — bootstrap may have respawned saver pane; version-guard scenario triggered by harness setup writing orphan1 PID into daemon.pid
- Forbidden-strings: type-asserts OrphanSweeper to `*bootstrap.OrphanSweepCore` and overwrites `Logger` with `RecordingLogger`; iterates entries asserting absence

TESTS:
- Status: Adequate
- Four enumerated test names map to assertions in single test (monolithic scenario; splitting would 4× harness cost)
- Rich diagnostics on failure
- No over/under-testing

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; seams via type-assertion as precedent
- Complexity: Low; setup → invoke → assert with one guard branch
- Modern idioms: `time.Since`/`time.Now`
- Readability: Good; 47-line file-header explains timing anchor, logger-capture, direct-primitive rationale

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Forbidden substring `"prior daemon did not exit"` is shorter than spec's full `"prior daemon did not exit within 5s"`; one-line inline comment making looseness explicit
- [idea] Direct production primitives vs `Orchestrator.Run` — if future Orchestrator accepted RecordingLogger via constructor injection, could exercise full step-4-then-5 ordering
- [quickfix] File-header explanation of logger-capture is repeated at call site; trim header without losing signal
