TASK: process: exit emission in Close (portal-observability-layer-2-12)

ACCEPTANCE CRITERIA:
- Close(0) emits exactly one process: exit code=0 took=<dur> INFO, returns without os.Exit.
- Close(1)/Close(2) render code accordingly.
- took from startTime non-negative for normal Init→Close.
- Close before any Init: no panic, bounded took.
- Exactly one exit line per Close call.
- Exit line visible at WARN/ERROR (level-filter bypass).

STATUS: Complete

SPEC CONTEXT:
Spec § Mechanical rule process: exit + main exit shape (518-561). Close(exitCode) is a pure marker-emitter computing took from package-private startTime; emits one INFO; logger owns no control flow. "exit" in closed lifecycle set → bypasses level filter. Baselines auto-injected.

IMPLEMENTATION:
- Status: Implemented (matches spec verbatim)
- Location: init.go:151-160 (Close + computeTook seam); startTime :26 captured/reset :52; handler.go:30,46-52 (processComponent + "exit" in bypass set); :142-143 (bypass enforcement); main.go:41-44 (Close gated behind !panicked).
- Notes: Close emits For(processComponent).Info("exit","code",exitCode,"took",computeTook()) — no os.Exit, no baselines passed. No os.Exit in internal/log outside doc comments. took factored to computeTook() (DRY, testable). Pre-Init zero startTime → large finite non-negative took, no panic (swap always holds valid handler). Bypass real ("exit" in set + component gate). main skips Close on panic path (no double-emit).

TESTS:
- Status: Adequate
- Location: internal/log/close_exit_test.go + init_test.go
- Coverage: EmitsProcessExitWithCodeAndTook (INFO, code, KindDuration non-negative); RendersPassedExitCode {0,1,2}; ReturnsWithoutTerminatingProcess; NonNegativeTookForNormalInitThenClose + CapturesStartTime; SafeBeforeInitEmitsBoundedTook + SafeBeforeAnyInit; EmitsExactlyOneExitLinePerCall; ExitLineVisibleAtConfiguredWarn (real text handler, baselines, exactly one line).
- Notes: Behaviour-asserting (level/msg/attrs/duration/count/bypass) against recording + real text handler. Mild justified overlap between close_exit and init test pairs (different assertion surfaces).

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; no internal/state import; doc comments).
- SOLID: Good — Close single responsibility; control flow externalized to main; computeTook focused extraction.
- Complexity: Low (one-liners).
- Modern idioms: Yes (slog, time.Since).
- Readability: Good — doc documents pre-Init, no-os.Exit, panic-skip, bypass.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Minor test overlap between close_exit_test.go and init_test.go on non-negative took + pre-Init cases (not redundant — different assertion surfaces); a comment noting the division-of-labour would prevent a maintainer deleting one as "duplicate."
