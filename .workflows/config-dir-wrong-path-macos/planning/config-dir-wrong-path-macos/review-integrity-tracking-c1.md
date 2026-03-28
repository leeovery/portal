---
status: in-progress
created: 2026-03-28
cycle: 1
phase: Plan Integrity Review
topic: Config Dir Wrong Path Macos
---

# Review Tracking: Config Dir Wrong Path Macos - Integrity

## Findings

### 1. Task 2 missing test for XDG_CONFIG_HOME suppressing migration

**Severity**: Important
**Plan Reference**: Phase 1 / Task config-dir-wrong-path-macos-1-2 (tick-6a3514)
**Category**: Acceptance Criteria Quality
**Change Type**: add-to-task

**Details**:
Task 2's Do section specifies that migration runs "on fallback path only" -- meaning migration is skipped when `XDG_CONFIG_HOME` is set. This is an important behavioral boundary, but there is no acceptance criterion or test verifying it. Without this, an implementer could place the migration call outside the fallback branch, causing migration to run when `XDG_CONFIG_HOME` is set (moving files to an unexpected custom location). The existing test "migration does not run when per-file env var is set" covers the env var override case but not the XDG case.

**Current**:
```
**Acceptance Criteria**:
- [ ] configFilePath moves file from old to new path when old exists and new does not
- [ ] Does NOT overwrite existing file at new path
- [ ] No-op when old directory does not exist
- [ ] Removes old directory if empty after migration
- [ ] Preserves old directory if non-empty after migration
- [ ] Logs warning to stderr on rename failure, still returns correct path
- [ ] Creates target directory via MkdirAll if missing
- [ ] Migration does NOT run when per-file env var override is active
- [ ] go test ./cmd/... passes

**Tests**:
- "migrates file from old macOS path to new path"
- "migration is no-op when old directory does not exist"
- "migration does not overwrite existing file at new path"
- "migration handles partial state"
- "migration cleans up empty old directory"
- "migration preserves non-empty old directory"
- "migration creates target directory if missing"
- "migration does not run when per-file env var is set"
- "migration logs warning on rename failure"
```

**Proposed**:
```
**Acceptance Criteria**:
- [ ] configFilePath moves file from old to new path when old exists and new does not
- [ ] Does NOT overwrite existing file at new path
- [ ] No-op when old directory does not exist
- [ ] Removes old directory if empty after migration
- [ ] Preserves old directory if non-empty after migration
- [ ] Logs warning to stderr on rename failure, still returns correct path
- [ ] Creates target directory via MkdirAll if missing
- [ ] Migration does NOT run when per-file env var override is active
- [ ] Migration does NOT run when XDG_CONFIG_HOME is set
- [ ] go test ./cmd/... passes

**Tests**:
- "migrates file from old macOS path to new path"
- "migration is no-op when old directory does not exist"
- "migration does not overwrite existing file at new path"
- "migration handles partial state"
- "migration cleans up empty old directory"
- "migration preserves non-empty old directory"
- "migration creates target directory if missing"
- "migration does not run when per-file env var is set"
- "migration does not run when XDG_CONFIG_HOME is set"
- "migration logs warning on rename failure"
```

**Resolution**: Pending
**Notes**:
