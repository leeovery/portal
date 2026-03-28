---
status: in-progress
created: 2026-03-28
cycle: 1
phase: Gap Analysis
topic: config-dir-wrong-path-macos
---

# Review Tracking: config-dir-wrong-path-macos - Gap Analysis

## Findings

### 1. Migration trigger point and mechanism unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Migration

**Details**:
The spec says "On first run after the fix, check if files exist at the old path" but does not specify:
- Where in the code the migration runs (during `configFilePath`? In `PersistentPreRunE`? A separate init function?)
- Whether migration runs on every invocation or only once
- Whether there is a sentinel/marker to record that migration has already been attempted, or whether it just relies on "old files no longer exist" as the signal

An implementer would need to make a design decision about where to place this logic and whether to guard against repeated migration attempts.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. Migration error handling unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Migration

**Details**:
The spec does not address what happens when migration fails partway through (e.g., permission denied on one file, disk full during copy). Specifically:
- Should migration errors be fatal (exit with error) or best-effort (log warning, continue)?
- If a file move fails, should remaining files still be attempted?
- Should the user see any output on successful migration (e.g., "Migrated config from X to Y")?

Since migration involves file moves that could fail, an implementer would need to decide on error strategy.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. Migration: "move" semantics unclear (rename vs copy+delete)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Migration

**Details**:
The spec says "move the file from old path to new path." On macOS, `~/Library/Application Support` and `~/.config` could be on different volumes (if home directory is unusual), making `os.Rename` fail. The spec doesn't clarify whether "move" means:
- `os.Rename` (atomic but fails cross-device)
- Copy + delete (works cross-device but not atomic)

For the overwhelmingly common case this is the same volume, but the spec should state the expected approach so the implementer doesn't over-engineer or under-engineer.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. Target directory creation not mentioned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Migration, Fix Approach

**Details**:
The fix changes the config directory from one that `os.UserConfigDir()` returns (which exists by default on macOS) to `~/.config/portal/`. The spec doesn't mention:
- Whether `~/.config/portal/` needs to be created if it doesn't exist (it likely does)
- Whether this directory creation should happen during migration, during `configFilePath`, or elsewhere
- What permissions the directory should have (0755 is standard but unstated)

The current `configFilePath` function only returns a path -- it doesn't create directories. If the callers already handle directory creation, this may be fine, but the spec should note whether the fix needs to account for this or if existing code already handles it.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 5. XDG_CONFIG_HOME with trailing slash or non-absolute path

**Source**: Specification analysis
**Category**: Minor - Gap/Ambiguity
**Affects**: Fix Approach

**Details**:
The spec says to "check `XDG_CONFIG_HOME` environment variable first" but doesn't specify handling of edge cases for this value:
- Should a trailing slash be stripped?
- Should a relative path be rejected or resolved?
- The XDG Base Directory Specification says the value must be an absolute path; should non-absolute values be ignored?

This is minor since Go's `filepath.Join` handles trailing slashes, and matching `os.UserConfigDir()`'s existing behavior (which doesn't validate either) is reasonable. But worth noting.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 6. Migration is macOS-only but detection mechanism unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Migration

**Details**:
The spec states "Migration is macOS-only" but doesn't specify how the code determines it's running on macOS. Options include:
- `runtime.GOOS == "darwin"` check
- Build tags (`_darwin.go` file)
- Simply checking if the old path exists (implicitly macOS-only since `~/Library/Application Support/portal/` wouldn't exist on Linux)

The third approach would make migration platform-agnostic (just check old path existence), which may be simpler. The spec should clarify the intended approach.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:
