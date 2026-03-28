---
status: in-progress
created: 2026-03-28
cycle: 1
phase: Traceability Review
topic: config-dir-wrong-path-macos
---

# Review Tracking: config-dir-wrong-path-macos - Traceability

## Direction 1: Specification to Plan (Completeness)

All specification elements verified as covered:

- **Problem statement / root cause**: Covered in Phase 1 goal and Task 1 Problem field
- **Affected code** (`cmd/config.go`, `configFilePath`): Covered in Task 1 Do section
- **Env var overrides unaffected**: Covered in Task 1 AC (precedence) and Task 2 AC (migration skip)
- **Fix approach** (XDG_CONFIG_HOME check, ~/.config fallback): Covered in Task 1 Do section steps 1-3
- **XDG_CONFIG_HOME edge cases** (trailing slashes, no special relative path handling): Covered in Task 1 Edge Cases
- **Migration trigger** (inside configFilePath, per-file): Covered in Task 2 Do section
- **Migration idempotency** (old exists + new doesn't): Covered in Task 2 AC and Do section
- **Platform detection** (no runtime.GOOS, check old path existence): Covered implicitly in Task 2 approach (os.Stat on old path)
- **Migration behavior** (os.Rename, partial state, skip existing, MkdirAll, empty dir cleanup): All covered in Task 2 Do, AC, and Edge Cases
- **Error handling** (best-effort, stderr warning, continue): Covered in Task 2 AC and tests
- **All spec test scenarios**: Every test case from the Testing section maps to Task 1 or Task 2 tests

## Direction 2: Plan to Specification (Fidelity)

All plan content verified as traceable:

**Task 1** (tick-7bd039):
- Problem, Solution, Outcome: All trace to spec Problem Statement and Fix Approach sections
- Do steps: Direct translation of spec Fix Approach steps 1-3
- All 6 AC items trace to spec Fix Approach and Testing sections
- All 6 test names trace to spec requirements
- Edge cases (empty XDG, trailing slash, UserHomeDir failure): First two are in spec; UserHomeDir failure is a natural Go error-handling requirement implied by using os.UserHomeDir()

**Task 2** (tick-6a3514):
- Problem, Solution, Outcome: All trace to spec Migration section
- Do steps: Direct translation of spec Migration behavior
- All 9 AC items trace to spec Migration section
- All 9 test names trace to spec Migration testing scenarios
- All 7 edge cases trace to spec Migration behavior and edge case descriptions
- "Migration does NOT run when per-file env var override is active": Traces to spec statement that env var overrides "bypass configFilePath's directory logic entirely"

**Phase 1**:
- Goal and rationale trace to spec Problem Statement and Fix Approach
- All 10 phase-level AC items trace to spec requirements

## Findings

No findings. The plan is a faithful and complete translation of the specification in both directions.
