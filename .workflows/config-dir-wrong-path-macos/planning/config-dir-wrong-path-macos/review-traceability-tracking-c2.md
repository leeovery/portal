---
status: complete
created: 2026-03-28
cycle: 2
phase: Traceability Review
topic: config-dir-wrong-path-macos
---

# Review Tracking: config-dir-wrong-path-macos - Traceability

## Direction 1: Specification to Plan (Completeness)

All specification elements verified as covered:

- **Problem statement / root cause**: Covered in Phase 1 goal and Task 1 Problem field
- **Affected code** (cmd/config.go, configFilePath): Covered in Task 1 Do section
- **Why it wasn't caught** (tests compare against os.UserConfigDir): Covered in Task 1 Problem
- **Three callers** (alias.go, clean.go, hooks.go): Contextual — fix targets configFilePath itself, callers need no changes
- **Env var overrides unaffected**: Covered in Task 1 AC (precedence) and Task 2 AC (migration skip when env var set)
- **Fix approach** (XDG_CONFIG_HOME check, ~/.config fallback, append portal/filename): Covered in Task 1 Do steps 1-3
- **XDG_CONFIG_HOME edge cases** (trailing slashes, no relative path validation): Covered in Task 1 Edge Cases
- **Migration trigger** (inside configFilePath, per-file, before returning path): Covered in Task 2 Do
- **Migration idempotency** (old exists + new doesn't = implicit no-sentinel): Covered in Task 2 AC and Do
- **Platform detection** (no runtime.GOOS, os.Stat on old path): Covered in Task 2 Do (uses os.Stat, no runtime.GOOS)
- **Migration behavior** (os.Rename, partial state, skip existing, MkdirAll, empty dir cleanup): All covered in Task 2 Do, AC, Tests, and Edge Cases
- **Error handling** (best-effort, stderr warning, continue): Covered in Task 2 AC and Tests
- **Directory creation** (configFilePath only returns path, migration must MkdirAll target): Covered in Task 2 Do step 3
- **All spec test scenarios**: Every test case from spec Testing section maps to Task 1 or Task 2 Tests

## Direction 2: Plan to Specification (Fidelity)

**Task 1** (tick-7bd039): All content traces to specification.
- Problem, Solution, Outcome: Trace to spec Problem Statement and Fix Approach
- Do steps 1-3: Direct translation of spec Fix Approach steps 1-3
- AC items 1-3: Trace to spec Fix Approach + Testing section
- AC item 4 (UserHomeDir failure): Implied by using os.UserHomeDir() — standard Go error handling
- AC items 5-6 (existing tests pass, go build): Standard quality gates
- All 6 test names: Trace to spec Fix Approach, Testing, and XDG edge cases
- Edge cases: All trace to spec Fix Approach section on XDG_CONFIG_HOME edge cases

**Task 2** (tick-6a3514): Almost all content traces to specification. One item flagged below.
- Problem, Solution, Outcome: Trace to spec Migration section
- Do steps: Direct translation of spec Migration behavior and trigger
- AC items 1-7: Trace to spec Migration behavior, error handling, and directory creation
- AC item 8 (no migration when per-file env var active): Traces to spec "Env var overrides bypass configFilePath's directory logic entirely"
- AC item 9 (no migration when XDG_CONFIG_HOME set): **Flagged** — see Finding 1
- AC item 10 (go test passes): Standard quality gate
- Test "migration does not run when XDG_CONFIG_HOME is set": **Flagged** — see Finding 1
- All other tests: Trace to spec Migration and Testing sections
- All edge cases: Trace to spec Migration behavior

**Phase 1**: Goal, rationale, and all 10 AC items trace to specification.

## Findings

### 1. Migration skip when XDG_CONFIG_HOME is set — not in specification

**Type**: Hallucinated content
**Spec Reference**: Migration section — "Each configFilePath() call checks if its own file exists at ~/Library/Application Support/portal/" (implies migration runs on every call, not conditionally)
**Plan Reference**: Task 2 (tick-6a3514) — AC item 9 and test "migration does not run when XDG_CONFIG_HOME is set"
**Change Type**: update-task

**Details**:
The specification's Migration section says "Each configFilePath() call checks if its own file exists at ~/Library/Application Support/portal/" without restricting to any particular code path. The spec also says the target is "~/.config/portal/" specifically (in the Testing section: "Migration moves files from ~/Library/Application Support/portal/ to ~/.config/portal/"). These two statements create ambiguity about what should happen when XDG_CONFIG_HOME is set — the spec never addresses this case.

The task resolves this ambiguity by adding "Migration does NOT run when XDG_CONFIG_HOME is set" as an acceptance criterion and test case. This is a design decision not present in the specification. While it may be a reasonable implementation choice, it cannot be traced to any specific spec statement and should be marked `[needs-info]` for the user to confirm.

**Current**:
In AC list:
```
- [ ] Migration does NOT run when XDG_CONFIG_HOME is set
```

In Tests list:
```
- "migration does not run when XDG_CONFIG_HOME is set"
```

**Proposed**:
In AC list:
```
- [ ] [needs-info] Migration does NOT run when XDG_CONFIG_HOME is set (spec does not address this case — confirm intended behavior)
```

In Tests list:
```
- "migration does not run when XDG_CONFIG_HOME is set" [needs-info]
```

**Resolution**: Fixed
**Notes**: Applied [needs-info] tags to AC and test in both phase-1-tasks.md and tick task. User must confirm intended behavior.
