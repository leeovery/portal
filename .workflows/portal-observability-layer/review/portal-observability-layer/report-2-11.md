TASK: process: start and log-level resolved emission in Init (with invalid-level WARN) (portal-observability-layer-2-11)

ACCEPTANCE CRITERIA:
- Init emits exactly one process: start INFO (cmd=Base(Args[0]), args=Join(Args[1:])) as final pre-return action.
- Init emits exactly one process: log-level resolved INFO immediately after start (resolved/source/raw).
- source=env (valid), default (unset, raw=), fallback (invalid).
- source=fallback → also one bootstrap: invalid PORTAL_LOG_LEVEL raw=<v> resolved=info WARN.
- Both process lines visible even at WARN/ERROR (level-filter bypass).
- Baseline attrs auto-injected (call sites don't pass them).

STATUS: Complete

SPEC CONTEXT:
Spec § Defensive invariants (process: start, 500) + § Log-level propagation verification (604) + § Log-level discipline default/invalid handling (293). process lifecycle set bypasses level filter; process: start is first record (drives first-of-day open + gated sweep). Baselines auto-injected.

IMPLEMENTATION:
- Status: Implemented
- Location: init.go:48-60 (Init resolves level, swaps handler, calls emitLifecycleMarkers as last statement before return); :86-103 (emitLifecycleMarkers: start first, then log-level resolved, then bootstrap WARN only when fallback; no baselines passed); level.go:34-52/57; handler.go:46-52,142-145 (bypass for process + msg; bootstrap NOT bypassed); main.go:33 (Init once before any portal code).
- Notes: Order correct (start first record → first-of-day open). bootstrap WARN under bootstrap component (not process), visible at always-info fallback. Idempotent re-emit documented.

TESTS:
- Status: Adequate
- Location: internal/log/init_test.go
- Coverage: EmitsProcessStartThenLogLevelResolvedInOrder (exactly-one each, order, cmd/args/level/source/raw env); SourceDefaultWhenUnset; SourceFallbackEmitsBootstrapWarn (verbatim raw=trace, exactly-one WARN); NoBootstrapWarnWhenSourceNotFallback; BothProcessLinesVisibleAtWarnLevel; ProcessLinesCarryAutoInjectedBaselinesNotDoubleEmitted; SecondInitReEmitsBothProcessLines.
- Notes: 1:1 with micro-acceptance + edge cases. Behaviour (rendered lines). Double-emit guard precise. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (no internal/state import, guarded; emission via For(processComponent); no t.Parallel).
- SOLID: Good — emitLifecycleMarkers single-responsibility seam; resolveLevel/levelString pure.
- Complexity: Low.
- Modern idioms: Yes (filepath.Base, strings.Join per spec).
- Readability: Good — doc explains order, bypass, bootstrap-vs-process distinction.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Spec render examples show quoted raw="<v>" / raw="" but the text handler renders single-token values unquoted (raw=trace, raw=) per the Phase-1 quoteIfMultiWord convention; spec examples are illustrative. A one-line spec clarification would remove the apparent mismatch; no code change needed. (Recurring across 2-11/2-12.)
