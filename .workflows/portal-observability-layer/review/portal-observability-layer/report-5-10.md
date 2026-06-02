TASK: Emit daemon self-eject then log.Close(0) then os.Exit(0) at the hysteresis trip with load-bearing ordering (portal-observability-layer-5-10)

ACCEPTANCE CRITERIA:
- At trip, daemon emits daemon: self-eject ticks=N threshold=3 (ticks == consecutiveAbsenceTicks at trip, threshold == selfSupervisionHysteresisTicks).
- Ordering exactly self-eject INFO → log.Close(0) → osExit(0); log.Close(0) emits process: exit code=0 and does not itself exit.
- No daemon: shutdown line on self-eject path (daemonShutdownFunc not called).
- Per-tick probe failures below threshold emit DEBUG only (no INFO until trip); passing probe resets counter to 0.
- Eject uses osExit seam, not bare os.Exit.
- Legacy self-supervision: saver-membership lost INFO removed (replaced by self-eject).

STATUS: Complete

SPEC CONTEXT:
Spec § Defensive invariants (558) sanctioned exception: self-eject emits daemon: self-eject FIRST, THEN log.Close(0) (process: exit code=0, no exit), THEN os.Exit(0); does NOT run daemonShutdownFunc → no daemon: shutdown. § Lifecycle taxonomy (895-906): trip → self-eject with ticks+threshold; shutdown not on self-eject path. § Log-level discipline: hysteresis-internal probe failures DEBUG; one INFO on trip.

IMPLEMENTATION:
- Status: Implemented
- Location: state_daemon.go:293-312 (eject branch in defaultDaemonTickLoop); osExit seam :112; log.Close init.go:151-153.
- Notes: :298 Info("self-eject","ticks",consecutiveAbsenceTicks,"threshold",selfSupervisionHysteresisTicks) on deps.Logger (component=daemon). :306-307 log.Close(0) then osExit(0) in mandated order; log.Close emits process: exit code=0, no control flow. daemonShutdownFunc NOT invoked (only on ctx.Done() arm :319-320). Below-threshold DEBUG :316; passing probe resets to 0 :290. Unreachable return nil :311 for test-seam unwind. Legacy INFO removed (grep zero in source; remaining hits are test absence-guards + stale CLAUDE.md).

TESTS:
- Status: Adequate
- Location: cmd/state_daemon_self_eject_log_test.go + self_supervision_test.go + integration (self_supervision_integration_test.go, bootstrap/composition_e2e_self_eject_integration_test.go)
- Coverage: EmitsCatalogedEventAtTrip (ticks=3 threshold=3 component=daemon, one); RemovesLegacyInfoLine; OrderSelfEjectThenProcessExitThenOsExit (shared sink proves self-eject idx < process:exit idx, both before osExit); ProcessExitCarriesCodeZero; DoesNotEmitShutdownLine (daemonShutdownFunc calls==0, no shutdown line); BelowThresholdEmitsDebugOnly; PassingProbeResetsCounter; UsesOsExitSeam. Supervision: SelfCheckLogsInfoOnEject, SelfCheckBypassesShutdownOnEject. Live e2e asserts marker reaches real portal.log, no scrollback delta, stale daemon.pid retained.
- Notes: Ordering test snapshots sink at osExit instant. Every AC mapped. Would fail if broken.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; seams t.Cleanup; osExit seam exclusively — the single sanctioned exception; logtest.Sink + SetTestHandler).
- SOLID: Good — probe (pure observation) vs eject decision separated; seams give DI.
- Complexity: Low.
- Modern idioms: Yes (slog attrs, closure counter, func seams).
- Readability: Good — load-bearing ordering documented inline + osExit seam doc.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Mild test overlap: SelfCheckLogsInfoOnEject (5-3-era, updated in place) and EmitsCatalogedEventAtTrip + RemovesLegacyInfoLine (5-10 suite) assert substantially the same cataloged INFO + legacy-absence; could consolidate.
- [quickfix] Stale docs: CLAUDE.md:103 still describes old behavior ("self-supervision: saver-membership lost for N consecutive ticks, exiting"); shipped event is daemon: self-eject ticks=N threshold=3 + process: exit code=0 via log.Close(0). Out of scope for this task but worth a doc refresh.
