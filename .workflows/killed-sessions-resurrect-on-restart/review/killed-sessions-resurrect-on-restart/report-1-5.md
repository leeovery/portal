TASK: killed-sessions-resurrect-on-restart-1-5 — Wire production EagerHydrateSignaler adapter in internal/bootstrapadapter

ACCEPTANCE CRITERIA:
- Production adapter wired against `state.ListSkeletonMarkers` and `state.WriteFIFOSignal`
- Orchestrator owns `stateDir`, resolved once at construction
- `FIFOPath(stateDir, paneKey)` derivation per marker before SendSignal / WriteFIFOSignal
- No new public API surface

STATUS: Complete

SPEC CONTEXT: Fix 1 (spec §"Write Primitive" + §"Pane Enumeration and FIFO Resolution") requires a production EagerHydrateSignaler that iterates `state.ListSkeletonMarkers` and writes the hydrate byte to `state.FIFOPath(stateDir, paneKey)` via the shared state-package primitive. Phase 4 task 4-1 collapsed the originally-planned standalone adapter via typed FIFO-signal seam plus no-seam helper, so production wiring is now an inline `EagerSignalCore` literal at `cmd/bootstrap_production.go`.

IMPLEMENTATION:
- Status: Implemented (post-Phase-4 collapsed shape)
- Location:
  - /Users/leeovery/Code/portal/cmd/bootstrap_production.go:89-152 (buildProductionOrchestrator)
  - /Users/leeovery/Code/portal/cmd/bootstrap_production.go:96 (`stateDir, _ := state.Dir()` — resolved once)
  - /Users/leeovery/Code/portal/cmd/bootstrap_production.go:122-136 (EagerSignaler inline `EagerSignalCore` literal)
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go:33-94 (EagerSignalCore + EagerSignalHydrate body, FIFOPath derivation at line 84)
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate.go:88-118 (SendHydrateSignal + DefaultFIFOSignaler + FIFOSignaler interface)
- Notes: `stateDir` resolved exactly once at bootstrap_production.go:96 and reused by RestoreAdapter, EagerSignalCore, FIFOSweeper. Shared-stateDir invariant documented in comments at lines 127-130. `Markers` field satisfied by `*tmux.Client` directly (state.ServerOptionLister via ShowAllServerOptions). `Signaler` is state.DefaultFIFOSignaler{} — delegates SendSignal → SendHydrateSignal → WriteFIFOSignal. No new exported surface in internal/bootstrapadapter (the adapter package was bypassed for this seam per task 4-1).

TESTS:
- Status: Adequate
- Coverage:
  - Unit: cmd/bootstrap/eager_signal_hydrate_test.go.
  - Default-resolution: orchestrator_builder_eager_default_test.go.
  - Integration: eager_signal_hydrate_integration_test.go drives production-shape wiring against real tmux.
- Notes: buildProductionOrchestrator itself is not unit-tested directly — acceptable as logic-free struct-literal composition.

CODE QUALITY:
- Project conventions: Followed. Inline struct-literal adapter mirrors MarkerCleanupCore and FIFOSweeper at the same wiring site.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes. Zero-value DefaultFIFOSignaler{} avoids closure adapters.
- Readability: Good. 5-line preceding comment explaining the inline-construction rationale and shared-stateDir invariant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] File-level comment in cmd/bootstrap_production.go:17 enumerates the `internal/bootstrapadapter`-resident adapters without noting that EagerSignalCore is deliberately *not* there. A one-line addition would close a small documentation gap.
