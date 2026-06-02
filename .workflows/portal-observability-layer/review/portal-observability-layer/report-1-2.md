TASK: Resolve log level from PORTAL_LOG_LEVEL with INFO default and invalid-value fallback (portal-observability-layer-1-2)

ACCEPTANCE CRITERIA:
- Unset PORTAL_LOG_LEVEL resolves to (LevelInfo, "default", "").
- debug/info/warn/error (exact) resolve to their levels with source=env.
- Mixed-case and surrounding whitespace resolve correctly with source=env, raw preserves verbatim input.
- "warning" (legacy alias) NOT accepted -> (info, fallback, "warning").
- Any other invalid value -> (info, fallback, verbatim).

STATUS: Complete

SPEC CONTEXT:
Spec § Log-level discipline (293-300) and § Log-level propagation verification (604-654): production default `info` (flipped from historical WARN); closed valid set debug/info/warn/error after lowercasing; invalid falls back to info + startup WARN; unset distinct from fallback; three source values env/default/fallback; raw verbatim.

IMPLEMENTATION:
- Status: Implemented (matches spec exactly; no drift)
- Location: internal/log/level.go — resolveLevel (34-52), levelString (57-70), source constants (11-20)
- Notes: Pure function; env read at call site (init.go:49). Normalisation `strings.ToLower(strings.TrimSpace(raw))`; closed-set switch so "warning" → default → fallback. raw returned verbatim in all arms. Unset → sourceDefault distinct from sourceFallback. Legacy state.parseLevel no longer exists (grep confirms migration landed; no dual-resolver drift).

TESTS:
- Status: Adequate
- Location: internal/log/level_test.go
- Coverage: DefaultsToInfoWhenUnset (AC1), ResolvesEachValidLevelWithSourceEnv (AC2), NormalisesMixedCaseAndWhitespace incl tab/newline (AC3), RejectsLegacyWarningAlias (AC4), FallsBackToInfoForInvalidValuePreservingRaw incl "WARNING"/"  bogus  " (AC5), LevelString_MapsLevelToLowercaseToken.
- Notes: Every AC and named test maps to a test; behaviour-focused, table-driven, not over-tested. levelString's defensive default arm unreachable from resolveLevel output and untested — acceptable, outside scope.

CODE QUALITY:
- Project conventions: Followed (unexported helpers, gofmt-clean, pure function).
- SOLID: Good — single responsibility, no env coupling.
- Complexity: Low.
- Modern idioms: Yes (slog.Level, strings helpers, named returns).
- Readability: Good — doc comments explain verbatim-raw contract, dropped alias, Phase-2 boundary.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] levelString's default arm is dead relative to resolveLevel output and untested; a future maintainer could drop it or add a tiny test. Defensive, outside this task's scope.
