---
status: in-progress
created: 2026-03-28
cycle: 3
phase: Traceability Review
topic: config-dir-wrong-path-macos
---

# Review Tracking: config-dir-wrong-path-macos - Traceability

## Direction 1: Specification to Plan (Completeness)

All specification elements verified as covered:

- **Problem statement / root cause**: Covered in Phase 1 goal and Task 1 Problem field
- **Affected code** (cmd/config.go, configFilePath, three callers): Covered in Task 1 Do section; callers need no changes since fix targets configFilePath itself
- **Why it wasn't caught** (tests compare against os.UserConfigDir): Covered in Task 1 Problem
- **Env var overrides unaffected**: Covered in Task 1 AC (precedence) and Task 2 AC (migration skip when env var set)
- **Fix approach** (XDG_CONFIG_HOME check, ~/.config fallback, append portal/filename): Covered in Task 1 Do steps 1-3
- **Why not hardcode ~/.config** (would regress Linux XDG users): Covered by Task 1's XDG_CONFIG_HOME support
- **XDG_CONFIG_HOME edge cases** (trailing slashes, no relative path validation): Covered in Task 1 Edge Cases
- **Migration trigger** (inside configFilePath, per-file, before returning path): Covered in Task 2 Do
- **Files to migrate** (projects.json, aliases, hooks.json): Covered implicitly — configFilePath is called per-file by each caller
- **Migration idempotency** (old exists + new doesn't, no sentinel): Covered in Task 2 AC and Do
- **Platform detection** (no runtime.GOOS, os.Stat on old path): Covered in Task 2 Do (uses os.Stat, no runtime.GOOS)
- **Migration behavior** (os.Rename, partial state, skip existing, MkdirAll, empty dir cleanup): All covered in Task 2 Do, AC, Tests, and Edge Cases
- **Error handling** (best-effort, stderr warning, continue, silent on success): Covered in Task 2 AC and Tests
- **Directory creation** (configFilePath only returns path, migration must MkdirAll target): Covered in Task 2 Do step 3
- **All spec test scenarios**: Every test case from spec Testing section maps to Task 1 or Task 2 Tests

## Direction 2: Plan to Specification (Fidelity)

**Task 1** (tick-7bd039): All content traces to specification.
- Problem, Solution, Outcome: Trace to spec Problem Statement and Fix Approach
- Do steps 1-3: Direct translation of spec Fix Approach steps 1-3
- AC items 1-3: Trace to spec Fix Approach and Testing section
- AC item 4 (UserHomeDir failure): Implied by using os.UserHomeDir() — standard Go error handling
- AC items 5-6 (existing tests pass, go build): Standard quality gates
- All 6 test names: Trace to spec Fix Approach, Testing, and XDG edge cases
- Edge cases: All trace to spec Fix Approach section on XDG_CONFIG_HOME edge cases

**Task 2** (tick-6a3514): All content traces to specification. One item correctly tagged [needs-info].
- Problem, Solution, Outcome: Trace to spec Migration section
- Do steps: Direct translation of spec Migration behavior and trigger
- AC items 1-7: Trace to spec Migration behavior, error handling, and directory creation
- AC item 8 (no migration when per-file env var active): Traces to spec "Env var overrides bypass configFilePath's directory logic entirely"
- AC item 9 [needs-info] (no migration when XDG_CONFIG_HOME set): Correctly tagged — spec does not address this case
- AC item 10 (go test passes): Standard quality gate
- Test 9 [needs-info]: Correctly tagged, matches AC item 9
- All other tests: Trace to spec Migration and Testing sections
- All edge cases: Trace to spec Migration behavior

**Phase 1**: Goal, rationale, and all 10 AC items trace to specification.

## Cycle 2 Finding Verification

The single finding from cycle 2 (XDG_CONFIG_HOME migration AC/test not in spec) was correctly resolved by adding [needs-info] tags to both the AC item and test in Task 2. Verified both tags are present in current task content.

## Findings

No findings. The plan is a faithful and complete translation of the specification in both directions. The [needs-info] tag from cycle 2 is correctly in place.
