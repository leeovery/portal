TASK: killed-sessions-resurrect-on-restart-5-1 — Promote duplicated state.FIFOSignaler recording fakes into internal/statetest

ACCEPTANCE CRITERIA:
- Priority order (global Err -> per-path ErrOn -> nil) preserved verbatim.
- Field-access casing flips lowercase -> Exported at both consumer sites.
- New statetest helper carries its own compile-time `var _ state.FIFOSignaler` assertion.
- Delete private recordingSignaler (cmd/) and recordingFIFOSignaler (cmd/bootstrap/).

STATUS: Complete

SPEC CONTEXT: Phase-5 (Cycle 2) duplication-cleanup task. Both consumer test files had open-coded private recording fakes for state.FIFOSignaler with identical priority semantics. Promotion mirrors statetest.RecordingSleep (sleep_recorder.go).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/statetest/fifo_signaler_recorder.go (new; compile-time `var _ state.FIFOSignaler = (*RecordingFIFOSignaler)(nil)` at line 48).
  - /Users/leeovery/Code/portal/cmd/state_signal_hydrate_test.go — eight call sites adopt statetest.RecordingFIFOSignaler{}, with `Err:` at line 174.
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_test.go — five call sites adopt statetest.RecordingFIFOSignaler{}, with `ErrOn:` at lines 113 and 189.
- Notes:
  - Priority order preserved verbatim in fifo_signaler_recorder.go:35-44 — Calls = append(...) always, then if r.Err != nil → return r.Err (global), then r.ErrOn[path] (per-path), else nil.
  - Compile-time interface assertion present at line 48.
  - Repo-wide grep confirms no remaining private recordingFIFOSignaler / recordingSignaler type declarations.

TESTS:
- Status: Adequate
- Coverage (internal/statetest/fifo_signaler_recorder_test.go):
  - TestRecordingFIFOSignaler_GlobalErrTakesPrecedence
  - TestRecordingFIFOSignaler_PerPathErrOnReturnsConfiguredError
  - TestRecordingFIFOSignaler_DefaultRecordsAndReturnsNil
  - TestRecordingFIFOSignaler_SatisfiesFIFOSignaler

CODE QUALITY:
- Project conventions: Followed — mirrors precedent of internal/statetest/sleep_recorder.go.
- SOLID: Good. Single responsibility.
- Complexity: Low.
- Modern idioms: Yes. Pointer receiver, idiomatic compile-time interface assertion.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] fifo_signaler_recorder.go and sleep_recorder.go both carry a duplicated "Concurrent invocation from multiple goroutines is NOT supported" paragraph. A short package-level doc-block stating "all recording helpers in this package are single-goroutine only" would let each helper drop the paragraph.
- [idea] TestRecordingFIFOSignaler_SatisfiesFIFOSignaler duplicates the compile-time assertion at fifo_signaler_recorder.go:48 — both fail at compile-time if interface drifts. Removable without coverage loss; retained as cheap intent-documentation.
