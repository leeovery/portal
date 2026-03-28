---
status: in-progress
created: 2026-03-28
cycle: 2
phase: Gap Analysis
topic: config-dir-wrong-path-macos
---

# Review Tracking: config-dir-wrong-path-macos - Gap Analysis

## Findings

### 1. Migration scope per `configFilePath()` call is ambiguous

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Migration

**Details**:
The spec says migration runs inside `configFilePath()` and describes the behavior as "For each config file, check if it exists at the old path." However, `configFilePath(envVar, filename string)` receives a single filename. This creates ambiguity about whether each call:

(a) Migrates only the single file it was called with (e.g., `configFilePath("PORTAL_ALIASES_FILE", "aliases")` only migrates `aliases`), or
(b) Migrates all three config files (`projects.json`, `aliases`, `hooks.json`) on every call, requiring a hardcoded list of filenames inside `configFilePath`.

Option (a) is simpler and avoids maintaining a duplicate file list, but the "For each config file" language reads like option (b). Both work correctly with the idempotency and cleanup behavior described, but an implementer would need to choose.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:
