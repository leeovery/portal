TASK: killed-sessions-resurrect-on-restart-6-1 — Refresh stale doc-comment cross-references to renamed/relocated primitives

ACCEPTANCE CRITERIA:
- cmd/bootstrap/eager_signal_hydrate.go:28 names statetest.RecordingFIFOSignaler.
- internal/restoretest/restoretest.go:152 names state.WriteFIFOSignal / state.SendHydrateSignal in internal/state.
- internal/restoretest/restoretest.go:256 names state.WriteFIFOSignal in internal/state.
- grep -rn "recordingFIFOSignaler" --include="*.go" returns zero hits.
- grep -rn "writeFIFOSignal" --include="*.go" returns zero hits.

STATUS: Complete

SPEC CONTEXT: Phase 6 / Analysis Cycle 3 doc-hygiene paydown after T1-1 (relocated writeFIFOSignal to state.WriteFIFOSignal / state.SendHydrateSignal) and T5-1 (promoted recordingFIFOSignaler into statetest.RecordingFIFOSignaler). No behavioural change.

IMPLEMENTATION:
- Status: Implemented (all three edits exact)
- Location:
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate.go:28 — "Tests inject statetest.RecordingFIFOSignaler."
  - /Users/leeovery/Code/portal/internal/restoretest/restoretest.go:151-153 — "byte-equivalent to state.WriteFIFOSignal / state.SendHydrateSignal in internal/state"
  - /Users/leeovery/Code/portal/internal/restoretest/restoretest.go:254-256 — "Byte-equivalent to state.WriteFIFOSignal in internal/state"

TESTS:
- Status: Adequate (no tests required)
- Coverage: Both required greps return zero hits across *.go files. Analysis docs correctly excluded by --include="*.go".

CODE QUALITY:
- Project conventions: Followed.
- Readability: Improved.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
