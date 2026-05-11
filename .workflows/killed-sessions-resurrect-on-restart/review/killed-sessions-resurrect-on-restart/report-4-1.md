TASK: killed-sessions-resurrect-on-restart-4-1 — Collapse EagerHydrateSignaler adapter via typed FIFO-signal seam and no-seam production helper

ACCEPTANCE CRITERIA:
- Replace `WriteFIFOSignal func(path) error` field on EagerSignalCore with typed FIFOSignaler interface seam.
- Add state.SendHydrateSignal(path) no-seam helper pinning OpenFIFOForSignal + time.Sleep.
- Add state.DefaultFIFOSignaler{} zero-value production implementation.
- Eliminate bootstrapadapter.EagerHydrateSignaler wrapper; construct *EagerSignalCore inline at cmd/bootstrap_production.go.
- Unify test-fake shape across cmd and cmd/bootstrap.
- Retain retry-ladder coverage in internal/state/signal_hydrate_test.go.
- Edge cases: avoid nil-receiver panic for zero-value EagerSignalCore; cmd/state_signal_hydrate.go retains its cmd-local seam.

STATUS: Complete

SPEC CONTEXT: Cycle 1 architecture analysis flagged the redundant two-layer adapter. Recommended collapse: typed FIFOSignaler seam + no-seam production helper so wiring can construct inline (mirroring MarkerCleanupCore).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate.go:78-118 — SendHydrateSignal, FIFOSignaler, DefaultFIFOSignaler.
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go:33-94 — EagerSignalCore carries `Signaler state.FIFOSignaler`.
  - /Users/leeovery/Code/portal/cmd/bootstrap_production.go:117-150 — EagerSignalCore constructed inline; Signaler: state.DefaultFIFOSignaler{}.
  - /Users/leeovery/Code/portal/internal/bootstrapadapter/adapters.go — EagerHydrateSignaler adapter type removed.
  - /Users/leeovery/Code/portal/cmd/state_signal_hydrate.go:18-23, 39-65, 67-70, 101 — Signaler typed state.FIFOSignaler; cmd-local signalHydrateRunFunc seam retained at line 70.
  - /Users/leeovery/Code/portal/CLAUDE.md:78 — step-6 doc updated.
- Notes: FIFOSignaler lives in internal/state (not cmd/bootstrap) — sensible deviation enabling cross-package sharing. Compile-time assertion at internal/statetest/fifo_signaler_recorder.go:48.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_test.go.
  - /Users/leeovery/Code/portal/internal/statetest/fifo_signaler_recorder_test.go — priority order (Err > ErrOn > nil); compile-time + runtime FIFOSignaler-satisfaction assertion.
  - /Users/leeovery/Code/portal/cmd/state_signal_hydrate_test.go.
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate_test.go — retry-ladder coverage anchored where primitive lives.
- recordingFIFOWriter removed; consumers share statetest.RecordingFIFOSignaler.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Dependency inversion via typed interface; single-method interface.
- Complexity: Low. Reduces conceptual surface.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None observed.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] FIFOSignaler interface lives in internal/state rather than cmd/bootstrap; a one-line docstring note calling out the design decision would close planning-artifact-vs-implementation drift.
- [idea] EagerSignalCore has no defensive guard against nil Markers or nil Signaler; a one-line docstring note ("Markers and Signaler are mandatory; behaviour with either nil is undefined") would harden the contract.
