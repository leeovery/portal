# Investigation: Config Dir Wrong Path Macos

## Symptoms

### Problem Description

**Expected behavior:**
Config files (projects.json, aliases, hooks.json) should be stored at `~/.config/portal/` following XDG conventions, as documented in the specification, README, and CLAUDE.md.

**Actual behavior:**
On macOS, `os.UserConfigDir()` returns `~/Library/Application Support` instead of `~/.config`, so config files are silently written to `~/Library/Application Support/portal/`.

### Manifestation

- Config files appear at `~/Library/Application Support/portal/` instead of `~/.config/portal/`
- Users/tooling checking `~/.config/portal/` find nothing
- No error messages — the bug is silent
- All functionality works correctly at the wrong path

### Reproduction Steps

1. Run Portal on macOS (any command that reads/writes config)
2. Check `~/.config/portal/` — directory does not exist
3. Check `~/Library/Application Support/portal/` — config files are there

**Reproducibility:** Always (on macOS)

### Environment

- **Affected environments:** macOS only (Linux works correctly)
- **Platform:** macOS — `os.UserConfigDir()` returns `~/Library/Application Support`
- **User conditions:** All macOS users

### Impact

- **Severity:** Medium
- **Scope:** All macOS users
- **Business impact:** Documentation/tooling mismatch; real user data exists at the wrong path requiring migration

### References

- `cmd/config.go` — `configFilePath` function using `os.UserConfigDir()`
- `cmd/config_test.go`
- Specification at `.workflows/v1/specification/portal/specification.md`
- `README.md`

---

## Analysis

### Initial Hypotheses

### Code Trace

### Root Cause

### Contributing Factors

### Why It Wasn't Caught

### Blast Radius

---

## Fix Direction

### Chosen Approach

### Options Explored

### Discussion

### Testing Recommendations

### Risk Assessment

---

## Notes
