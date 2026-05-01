AGENT: standards
STATUS: clean
FINDINGS_COUNT: 0

SUMMARY: Implementation conforms to specification and project conventions. Both Part 1 (`--` separator + migration) and Part 2 (deletion of `PredictLiveIndices` and consumers) are implemented per spec, including the precise eviction predicate (requires `command -v portal` prefix, requires `portal state signal-hydrate`, forbids `--`), descending-index UnsetGlobalHookAt iteration, INFO-on-eviction / silent-on-zero log shape, and tightened `signalHydrateSubstring` for dedupe. All five Acceptance Criteria and all four Testing Requirements are covered: cobra-level argv-parse test for leading-dash sessions (now also asserts FIFO byte written, satisfying AC #2 verbatim per cycle 1 task T3-4), real-tmux migration tests, integration reboot round-trip with leading-dash session driving the production binary, and shared `predictedVsLiveWarnRegex` proven false-positive-safe against the preserved armPanes:202 mismatch warning. No `t.Parallel()` violations; integration files correctly tagged `//go:build integration`. The mock-based partial-failure migration test is a documented, principled deviation (real-tmux harness cannot inject per-index UnsetGlobalHookAt failures) and goes through the canonical RegisterPortalHooks entry point per cycle 1 task T3-6.

Convergence achieved.
