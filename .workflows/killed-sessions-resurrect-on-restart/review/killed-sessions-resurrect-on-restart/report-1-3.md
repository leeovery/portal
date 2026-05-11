TASK: killed-sessions-resurrect-on-restart-1-3 — Define EagerHydrateSignaler seam and EagerSignalHydrate step in cmd/bootstrap

ACCEPTANCE CRITERIA:
- EagerHydrateSignaler seam interface defined in cmd/bootstrap.
- Production adapter wired in internal/bootstrapadapter against state.ListSkeletonMarkers and state.WriteFIFOSignal.
- Zero-marker post-Restore is a no-op (no FIFO writes attempted, step returns nil).
- Per-FIFO write failure logs soft warning `WARN | hydrate | eager-signal: write fifo <path>: <error>` and continues.
- Step never escalates to fatal.

STATUS: Complete

SPEC CONTEXT: Per spec § Fix 1 → Adapter Wiring, the orchestrator step iterates the post-Restore marker map, derives FIFO path via state.FIFOPath(stateDir, paneKey), and writes the hydrate signal byte. Per § Failure Posture: per-FIFO write failures are soft; zero markers is a no-op; never fatal. Phase 4 task 4-1 collapsed the original two-method seam into a single-method EagerHydrateSignaler whose concrete *EagerSignalCore depends on typed state.ServerOptionLister + state.FIFOSignaler seams.

IMPLEMENTATION:
- Status: Implemented (post-Phase-4-task-4-1 refactor shape)
- Location:
  - Seam interface: /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap.go:112-114; Orchestrator field at :199.
  - Concrete core + algorithm: /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go:33-94.
  - Production wiring: /Users/leeovery/Code/portal/cmd/bootstrap_production.go:131-136 (inline literal). Documented at internal/bootstrapadapter/adapters.go:12-15.
  - NoOp fallback: /Users/leeovery/Code/portal/cmd/bootstrap/noop.go:51-54.
  - Step invocation: /Users/leeovery/Code/portal/cmd/bootstrap/bootstrap.go:289-300 (step 6, between Restore step 5 and Clear step 7; failure logged via Warn and swallowed).
- Notes: Algorithm matches spec: nil-Logger substitution → enumerate markers → zero-marker short-circuit → per-paneKey FIFOPath derivation + Signaler.SendSignal → Warn-and-continue on failure → final return nil.

TESTS:
- Status: Adequate
- Coverage (all in /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_test.go):
  - TestEagerHydrateSignalerInterfaceContract (compile-time pin)
  - TestNoOpEagerHydrateSignaler_ReturnsNil
  - TestEagerSignalHydrate_WritesSignalToEveryMarkerFIFO
  - TestEagerSignalHydrate_ZeroMarkersIsNoOp
  - TestEagerSignalHydrate_PerFIFOWriteFailureLogsAndContinues
  - TestEagerSignalHydrate_ReturnsErrorWhenListSkeletonMarkersFails
  - TestEagerSignalHydrate_NilLoggerTolerated
  - TestOrchestrator_HasEagerSignalerField
  - AC1 integration test at /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go gates the 2-second marker-clearance contract.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single-method seam (ISP); typed dependencies (DIP); nil-tolerant Logger.
- Complexity: Low. ~30 LOC of straight-line control flow.
- Modern idioms: Yes.
- Readability: Good. 32-line method-level docstring spells out the algorithm.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The spec literal proposed a two-method `EagerHydrateSignaler { ListSkeletonMarkers; WriteFIFOSignal }` seam. Shipped shape is single-method (acknowledged outcome of task 4-1). No action needed.
- [idea] Production adapter wiring lives at cmd/bootstrap_production.go (inline struct literal) rather than as a named adapter type in internal/bootstrapadapter. Functionally equivalent and documented at adapters.go:12-15.
