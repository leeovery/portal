# Architecture Findings — killed-sessions-resurrect-on-restart (cycle 7)

```
AGENT: architecture
FINDINGS: none

SUMMARY: Cycle 6's two cleanup tasks landed cleanly and resolved every residual architectural finding from cycles 1-5:

- T9-1 — RestoreAdapter preamble + openTestLogger shim collapse across cmd/bootstrap tests:
  - All five open-coded `restoreInner := &restore.Orchestrator{...}` + `&bootstrapadapter.RestoreAdapter{Inner: restoreInner}` preambles in cmd/bootstrap/phase5_integration_test.go (lines 169, 257) and cmd/bootstrap/reboot_roundtrip_test.go (lines 328, 934, 1186) are now `bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)`.
  - The 12 in-package call sites of `openTestLogger` have all been migrated to `restoretest.OpenTestLogger`. The shim wrapper at orchestrator_builder_test.go:118-120 was deleted.
  - `restoretest.OpenTestLogger` is now the single named call site for the test-logger helper across every consumer touched by this work unit.

- T9-2 — duplicate FIFO non-blocking-flags test deletion:
  - `TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags` removed from cmd/state_signal_hydrate_test.go; canonical coverage at `TestOpenFIFOForSignal_NonBlockingFlags` (internal/state/signal_hydrate_test.go:244) preserved next to the function under test. The `runtime` and `syscall` imports were also dropped from cmd/state_signal_hydrate_test.go.

Architectural picture across the full 45-file delta:

- Step seam vocabulary is uniform: bootstrap.HookRegistrar / RestoringMarker / Restorer / EagerHydrateSignaler / MarkerCleaner / FIFOSweeper / StaleCleaner are all single-method (1-3 method) interfaces; production "Core" implementations (EagerSignalCore, MarkerCleanupCore) follow the same Markers/Panes/Unsetter dependency-shape pattern and tolerate nil Logger via a local no-op substitution.
- Inverse-pair symmetry intact: state.FIFOPath / state.PaneKeyFromFIFOPath are co-located in internal/state/paths.go, and the helper composition state.UnsetSkeletonMarkerForFIFO encodes the FIFO-basename ⇄ paneKey invariant in one place.
- Production no-seam entry point (state.SendHydrateSignal) wraps the retry-bearing primitive (state.WriteFIFOSignal) and is in turn wrapped by the orchestrator seam (state.DefaultFIFOSignaler / state.FIFOSignaler) — three layers, each justified by a distinct test-coverage need.
- Step ordering doc-comment lattice is consistent: bootstrap.go's package doc (steps 1-10), the per-step Step-N tags in the Orchestrator.Run body, the seam interface docstrings, and CLAUDE.md "Server bootstrap" all enumerate the same ten-step sequence with EagerSignalHydrate at position 6.

No new architectural concerns surface from cycle 6's deltas. The implementation's architecture is sound across all 45 files in scope — clean boundaries between cmd, cmd/bootstrap, internal/state, internal/restore, internal/bootstrapadapter, internal/restoretest, internal/statetest; appropriate adapter/core split; proper seam injection at every test boundary; and good composition opportunities exercised (NewRestoreAdapter constructor, NewWithDefaults builder helper, OpenTestLogger shared helper, RecordingFIFOSignaler shared fake).
```

STATUS: clean
