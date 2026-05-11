TASK: killed-sessions-resurrect-on-restart-1-2 — Repoint cmd/state_signal_hydrate.go at the shared internal/state writer

ACCEPTANCE CRITERIA:
- Existing client-attached / client-session-changed signaling path calls into the shared internal/state writer rather than the cmd-local implementation.
- Edge cases: nil logger no-op; list-markers failure soft-warns and returns nil; per-pane write failure does not abort sibling panes.

STATUS: Complete

SPEC CONTEXT: Spec § "Fix 1 → Write Primitive" requires `writeFIFOSignal` and `signalHydrateRetryDelays` to relocate into `internal/state` with no new public API. § "Failure Posture" mandates soft warnings on per-FIFO write failures, zero-marker no-op, no fatal escalation.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/state_signal_hydrate.go:18-65 (signalHydrateConfig uses `state.FIFOSignaler` seam; runSignalHydrate iterates marker map → pane filter → SendSignal). Production wiring at lines 96-102 plants `state.DefaultFIFOSignaler{}`. Shared writer at /Users/leeovery/Code/portal/internal/state/signal_hydrate.go.
- No cmd-local FIFO writer remains: grep returns no matches.
- Marker-unset deliberately untouched at this site (helper owns it per spec § "100ms Settle Sleep").

TESTS:
- Status: Adequate
- Coverage (/Users/leeovery/Code/portal/cmd/state_signal_hydrate_test.go):
  - Multi-pane happy path, unmarked-pane skip, zero-marker no-op, per-FIFO failure soft-fails without unsetting marker, forbidden set-option -su guard, session-missing list-panes soft-fail, repeated-invocation idempotency, RunE-defers-logger-Close, cobra leading-dash argv-parse.
  - Underlying retry ladder exhaustively pinned in internal/state/signal_hydrate_test.go.
- Notes on plan edge cases:
  - nil logger no-op — enforced at internal/state/logger.go:213-218 and :246-249.
  - list-markers failure → soft-warn → nil — no dedicated test; structurally identical to list-panes-failure arm covered by TestSignalHydrate_SoftFailsWhenSessionDoesNotExist.
  - per-pane write failure does not abort siblings — single-pane case covered by TestSignalHydrate_PerFIFOFailureDoesNotUnsetMarker; loop-continuation structurally evident, no direct N≥2 assertion.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] No dedicated test for the list-markers (show-options) failure → soft-warn → return-nil arm at cmd/state_signal_hydrate.go:42-44.
- [idea] No multi-pane scenario asserting "pane A SendSignal fails, pane B still receives SendSignal".
