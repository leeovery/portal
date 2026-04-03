# Implementation Review: Resume Hooks Lost on Server Restart

**Plan**: resume-hooks-lost-on-server-restart
**QA Verdict**: Approve

## Summary

All 13 tasks across 4 phases are fully implemented with zero blocking issues. The two-part fix (empty-pane guard + structural key migration) correctly addresses both root causes identified in the specification. Code quality is consistently high — idiomatic Go, clean interface-based DI, well-balanced test coverage, and consistent structural key terminology throughout. The implementation faithfully follows the specification and plan with only minor, architecturally sound deviations in test placement and helper extraction timing.

## QA Verification

### Specification Compliance

Implementation aligns with the specification:
- **Problem 1 (Hook deletion on restart)**: Empty-pane guard added at `executor.go:72`, matching `cmd/clean.go:77-80` pattern. Both nil and empty slice handled via `len(livePanes) > 0`.
- **Problem 2 (Pane ID instability)**: Structural keys (`session_name:window_index.pane_index`) replace pane IDs throughout: storage model, pane querying, hook registration/removal, volatile markers, and cleanup.
- **Storage model**: `hooks.json` now maps `structural_key -> map[event]command`.
- **Volatile markers**: Format changed to `@portal-active-{structural_key}`.
- **Behavioral requirements**: Graceful no-op without tmux-resurrect, no resurrect dependency, multi-pane support, silent operation — all tested.
- **Breaking change**: Old pane-ID entries cleaned by CleanStale on first run — tested in upgrade path test.

### Plan Completion

- [x] Phase 1 (Empty-Pane Guard) — 1/1 tasks complete
- [x] Phase 2 (Structural Key Infrastructure) — 4/4 tasks complete
- [x] Phase 3 (Consumer Migration) — 5/5 tasks complete
- [x] Phase 4 (Analysis Cycle 1 — Chores) — 3/3 tasks complete
- [x] All 13 tasks completed
- [x] No scope creep — Phase 4 tasks were analysis-driven improvements, properly planned

### Code Quality

No issues found. Highlights:
- Single-method `StructuralKeyResolver` interface follows interface segregation
- `resolveCurrentPaneKey()` helper eliminates duplication between hooks set/rm
- `writeHooksJSON`/`readHooksJSON` consolidated into single `testhelpers_test.go`
- Parameter names consistently reflect structural key semantics (`target`, `liveKeys`, `key`)
- Error wrapping uses `fmt.Errorf` with `%w` throughout

### Test Quality

Tests adequately verify requirements. Coverage includes:
- Empty-pane guard: empty slice, nil, non-empty, error path, post-restart survival
- Structural keys: construction, registration, lookup, multi-window, edge cases (colons, dots)
- Multi-pane: 3 independent panes with separate hooks firing correctly
- Graceful no-op: orphaned keys produce no errors
- Upgrade path: old pane-ID entries cleaned by CleanStale
- Resolution failure: user-facing errors for both set and rm
- No over-testing detected

### Required Changes

None.

## Recommendations

Non-blocking cosmetic observations (future cleanup candidates, not required for this review):

1. **Residual `paneID` loop variable** — `executor.go:97` loop variable `paneID` iterates structural keys. Could rename to `key` for consistency. Task 4-3 intentionally scoped to parameters only.

2. **Test-internal naming** — `keySend.paneID` field (`executor_test.go:31`) and `mockHookCleaner.livePanesReceived` (`executor_test.go:100`) retain old terminology while holding structural key values. Cosmetic only.

3. **Format string duplication** — `#{session_name}:#{window_index}.#{pane_index}` appears in `ResolveStructuralKey`, `ListPanes`, and `ListAllPanes`. A constant could reduce duplication but is not necessary for correctness.
