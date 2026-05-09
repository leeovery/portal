TASK: Insert cleanup step into orchestrator sequence between step 6 and step 7 (2-5)

ACCEPTANCE CRITERIA:
- New cleanup step inserted in `cmd/bootstrap/bootstrap.go` between Clear `@portal-restoring` and SweepOrphanFIFOs.
- Subsequent steps renumber accordingly (post phase 3-4: nine-step framing with CleanStaleMarkers as step 7).
- Package doc comment and step-entry Debug labels updated.
- Bootstrap integration test asserts execution position and soft-warning degradation matching `CleanStale`'s posture.
- No locks/sequencing constraints vs daemon.

STATUS: Complete

SPEC CONTEXT: Phase 2 introduces stale-marker cleanup step between Clear `@portal-restoring` (step 6) and SweepOrphanFIFOs. CLAUDE.md "Server bootstrap" codifies nine-step framing post phase 3-4.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/bootstrap.go:1-23` package doc lists nine steps with step 7 = CleanStaleMarkers.
  - `cmd/bootstrap/bootstrap.go:88-104` `MarkerCleaner` seam interface.
  - `cmd/bootstrap/bootstrap.go:165-175` `Orchestrator` struct gains `StaleMarkers MarkerCleaner` field.
  - `cmd/bootstrap/bootstrap.go:262-272` step-7 invocation site (Debug entry, call, Warn-on-error, continue).
  - `cmd/bootstrap/bootstrap.go:274-285` step 8 (Sweep) renumbered.
  - `cmd/bootstrap/bootstrap.go:287-292` step 9 (CleanStale) renumbered.
  - `cmd/bootstrap/bootstrap.go:177-195` `Run` doc enumerates soft-warning paths including step 7.
- Notes: Clean insertion at correct position. Debug labels and package doc consistent. Soft-warning posture mirrors `CleanStale`. CLAUDE.md "Server bootstrap" in sync.

TESTS:
- Status: Adequate
- Coverage in `cmd/bootstrap/bootstrap_test.go`:
  - `TestOrchestratorRun_executesStepsInSpecOrder` (138-161) — canonical nine-step order.
  - `TestOrchestratorRun_runsCleanStaleMarkersBetweenClearAndSweep` (739-767) — pins Clear < CleanStaleMarkers < Sweep < CleanStale.
  - `TestOrchestratorRun_runsSweepBetweenClearAndCleanStale` (651-679).
  - `TestOrchestratorRun_continuesPastCleanStaleMarkersFailure` (778-827) — soft-warning posture; Warn embeds "step 7" + "CleanStaleMarkers" + cause; Sweep/CleanStale still run.
  - `TestOrchestratorRun_emitsDebugLinePerExecutedStep` (841-874) — DEBUG line for CleanStaleMarkers.
  - Other tests include `CleanStaleMarkers` in want ordering.
  - `stepRecorder` / `newOrchestrator` updated with `CleanStaleMarkersErr`, `CleanStaleMarkers()`, `StaleMarkers: r` wiring.
- Notes: Focused; no over-testing. Each test pins distinct invariant.

CODE QUALITY:
- Project conventions: Followed. Small-interface DI; orchestrator field of interface type, production wiring in `internal/bootstrapadapter/`. Debug label format consistent.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Step-7 block comment explains both ordering rationales.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
