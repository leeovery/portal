TASK: killed-sessions-resurrect-on-restart-1-1 — Relocate writeFIFOSignal and signalHydrateRetryDelays into internal/state

ACCEPTANCE CRITERIA:
- Relocate `writeFIFOSignal` and `signalHydrateRetryDelays` from `cmd` into `internal/state`.
- No public API surface added beyond what is necessary for sharing.
- `cmd/state_signal_hydrate.go` and the new bootstrap step both call into the shared package.
- Edge cases: ENXIO/EAGAIN retry ladder preserved verbatim; ENOENT surfaces immediately; retries-exhausted wrapping unchanged.

STATUS: Complete

SPEC CONTEXT: Spec §"Write Primitive" (Fix 1) mandates moving `writeFIFOSignal` and `signalHydrateRetryDelays` from `cmd` into `internal/state` so both `cmd/state_signal_hydrate.go` and the new `cmd/bootstrap` EagerSignalHydrate step share the same retry/open semantics. Failure semantics (ENOENT immediate, ENXIO/EAGAIN retry per the ladder, retries-exhausted wrap) must remain identical.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate.go:18 `SignalHydrateRetryDelays` exported, ladder 10/20/40/80/160/190 ms (500 ms total) — verbatim.
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate.go:45 `WriteFIFOSignal(path, openFIFO, sleep)` exported, seam-bearing.
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate.go:74 `isRetryableFIFOError` unexported (only ENXIO/EAGAIN retryable; ENOENT explicitly surfaces immediately).
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate.go:88 `SendHydrateSignal` no-seam production entry pinning `OpenFIFOForSignal` + `time.Sleep`.
  - /Users/leeovery/Code/portal/internal/state/signal_hydrate.go:104,114 `FIFOSignaler` interface and `DefaultFIFOSignaler` zero-value adapter.
  - /Users/leeovery/Code/portal/cmd/state_signal_hydrate.go:23 `Signaler state.FIFOSignaler` and (line 101) production wires `state.DefaultFIFOSignaler{}`. No remnant of the cmd-private `writeFIFOSignal` / `signalHydrateRetryDelays` exists.
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go:36 `EagerSignalCore` embeds `state.FIFOSignaler`; production wiring at `cmd/bootstrap_production.go:134` injects `state.DefaultFIFOSignaler{}`.
- Notes:
  - Error wrapping unchanged: `open fifo %s: %w` on first-iteration non-retryable (signal_hydrate.go:59), `retries exhausted opening fifo %s: %w` on ladder exhaustion (signal_hydrate.go:67).
  - The relocation necessarily exports `WriteFIFOSignal` and `SignalHydrateRetryDelays`. Additional new symbols (`SendHydrateSignal`, `OpenFIFOForSignal`, `FIFOSignaler`, `DefaultFIFOSignaler`) are sibling exports justified by Phase 1 tasks 1-3 / 1-5 and cycle-1 4-1.

TESTS:
- Status: Adequate
- Coverage (/Users/leeovery/Code/portal/internal/state/signal_hydrate_test.go):
  - TestSignalHydrateRetryDelays_MatchesSpecLadder (ladder values pinned).
  - TestSignalHydrateRetryDelays_CumulativeBudget500ms.
  - TestWriteFIFOSignal_WritesOneByteOnFirstTrySuccess.
  - TestWriteFIFOSignal_RetriesOnENXIOPerLadder.
  - TestWriteFIFOSignal_RetriesOnEAGAINPerLadder.
  - TestWriteFIFOSignal_ENOENTReturnsImmediatelyWithOpenFifoWrap.
  - TestWriteFIFOSignal_NonRetryableErrorReturnsImmediately.
  - TestWriteFIFOSignal_RetryExhaustionWrapsLastErrWithRetriesExhausted.
  - TestOpenFIFOForSignal_NonBlockingFlags.
  - TestSendHydrateSignal_* / TestDefaultFIFOSignaler_* pin production no-seam entry contracts.
- Notes: All three plan edge cases have dedicated tests. Not over-tested. Recording fakes live in `internal/statetest`.

CODE QUALITY:
- Project conventions: Followed.
- SOLID principles: Good.
- Complexity: Low.
- Modern idioms: Yes (`errors.Is`, `%w`).
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Sibling symbols added alongside the strict relocation (`SendHydrateSignal`, `FIFOSignaler`, `DefaultFIFOSignaler`, `OpenFIFOForSignal`) widen the package's public surface beyond the literal "no public API surface added" wording. All are justified by downstream tasks; a future plan-traceability reviewer may flag the wording mismatch.
