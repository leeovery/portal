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

`os.UserConfigDir()` follows platform conventions — on macOS that's `~/Library/Application Support`, not `~/.config`. The project intends XDG-style paths on all platforms.

### Code Trace

**Entry point:** `cmd/config.go:12` — `configFilePath(envVar, filename string)`

**Execution path:**
1. `cmd/config.go:13` — checks env var override first (e.g. `PORTAL_PROJECTS_FILE`)
2. `cmd/config.go:17` — falls back to `os.UserConfigDir()` which returns `~/Library/Application Support` on macOS
3. `cmd/config.go:22` — joins with `portal/<filename>`

**Three callers, all in `cmd` package:**
- `cmd/alias.go:102` — `configFilePath("PORTAL_ALIASES_FILE", "aliases")`
- `cmd/clean.go:113` — `configFilePath("PORTAL_PROJECTS_FILE", "projects.json")`
- `cmd/hooks.go:130` — `configFilePath("PORTAL_HOOKS_FILE", "hooks.json")`

**Key files involved:**
- `cmd/config.go` — the single function that determines the base config directory
- `cmd/config_test.go` — tests use `os.UserConfigDir()` itself as the expected value, so they pass on macOS (wrong path but consistent)

**Env var overrides** (`PORTAL_PROJECTS_FILE`, `PORTAL_ALIASES_FILE`, `PORTAL_HOOKS_FILE`) are used exclusively in tests to redirect config to temp dirs. They take precedence over `configFilePath`'s directory logic, so they are unaffected by this bug.

### Root Cause

`configFilePath` uses `os.UserConfigDir()` as the base directory. On macOS, Go's `os.UserConfigDir()` returns `~/Library/Application Support` (the Apple-native path), not `~/.config` (the XDG path). The project's documented convention is `~/.config/portal/`.

**Why this happens:** Go's stdlib follows platform conventions. The project chose XDG conventions cross-platform but used a platform-aware stdlib function.

### Contributing Factors

- Go's `os.UserConfigDir()` is the "correct" stdlib call for platform-native config, but the project wants XDG-style paths
- No `XDG_CONFIG_HOME` handling exists anywhere in the codebase
- The function works correctly — it just resolves to the wrong directory for this project's conventions

### Why It Wasn't Caught

- `cmd/config_test.go` tests compare against `os.UserConfigDir()` — the same function that's wrong. Tests pass because they verify consistency with the stdlib, not against the documented path.
- The bug is silent — all config operations succeed at the wrong path
- Development and testing happen on macOS where the bug is always present, so the "wrong" path became normal

### Blast Radius

**Directly affected:**
- `cmd/config.go` — needs to hardcode `~/.config` instead of using `os.UserConfigDir()`
- `cmd/config_test.go` — tests need to verify against `~/.config`, not `os.UserConfigDir()`
- All three config files: `projects.json`, `aliases`, `hooks.json`

**Documentation references (say `~/.config/portal/` correctly, but code disagrees):**
- `CLAUDE.md` — three references
- `README.md` — config section
- Specification at `.workflows/v1/specification/portal/specification.md`

**Migration concern:**
- Existing macOS users have real data at `~/Library/Application Support/portal/`
- Fix must migrate files from old path to new path on first run

---

## Fix Direction

### Chosen Approach

### Options Explored

### Discussion

### Testing Recommendations

### Risk Assessment

---

## Notes
