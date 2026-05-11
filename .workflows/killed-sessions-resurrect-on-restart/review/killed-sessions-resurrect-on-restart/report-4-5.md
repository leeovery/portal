TASK: killed-sessions-resurrect-on-restart-4-5 — Extract signalFIFOAsync goroutine helper in cmd/state_hydrate_test.go

ACCEPTANCE CRITERIA:
- signalFIFOAsync(t *testing.T, fifo string) declared once near makeFIFO.
- Zero remaining inline goroutine blocks except divergent sites.
- All hydrate tests pass.
- Preserve t.Helper() / t.Cleanup() semantics.

STATUS: Complete

SPEC CONTEXT: Phase 4 cycle 1 duplication finding — 5-line goroutine pattern duplicated ~32 times across cmd/state_hydrate_test.go. Approved task explicitly carved out divergent sites (multi-byte writes, delays).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Helper at /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:33-47, after makeFIFO at lines 23-31.
  - Signature signalFIFOAsync(t *testing.T, fifo string) matches plan.
  - t.Helper() at line 38.
  - 33 call sites use the helper.
  - Two deliberate inline goroutines retained with explanatory comments:
    - cmd/state_hydrate_test.go:85-100 — TestHydrate_BlocksOnFIFOUntilSignalArrives with 50ms delay + signalSent channel.
    - cmd/state_hydrate_test.go:136-145 — TestHydrate_ReadsSingleByteFromFIFOOnSignal with multi-byte payload.

TESTS:
- Status: Adequate
- Coverage: Behaviour-preserving refactor; no new tests required. Transitive coverage strong via 30+ consumer sites.

CODE QUALITY:
- Project conventions: Followed. File header preserves no t.Parallel caveat.
- SOLID: Good. Single responsibility.
- Complexity: Low.
- Modern idioms: Idiomatic t.Helper(), _ = f.Close().
- Readability: Good. Reverse-pointer "Inline (not signalFIFOAsync) — ..." comments at retained inline sites.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Optional `makeAndSignalFIFO(t, dir) string` companion not added; would extend the cleanup further but plan marked it optional.
- [idea] Helper's goroutine has no test-side synchronisation back; preserves original behaviour but worth noting for future t.Context()-aware shift.
