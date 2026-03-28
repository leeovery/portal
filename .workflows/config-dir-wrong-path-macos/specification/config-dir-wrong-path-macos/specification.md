# Specification: Config Dir Wrong Path macOS

## Specification

## Problem Statement

On macOS, Portal stores config files at `~/Library/Application Support/portal/` instead of the documented `~/.config/portal/`. The bug is silent — all functionality works at the wrong path, but users and tooling expecting `~/.config/portal/` find nothing.

**Root cause:** `cmd/config.go:configFilePath()` uses Go's `os.UserConfigDir()` as the base directory. On macOS, this returns `~/Library/Application Support` (Apple-native convention). The project's documented convention is XDG-style `~/.config/portal/` on all platforms.

**Why it wasn't caught:** Tests in `cmd/config_test.go` compare against `os.UserConfigDir()` itself — verifying consistency with the stdlib rather than against the intended path.

## Affected Code

Single entry point: `cmd/config.go` — `configFilePath(envVar, filename string)` function.

Three callers, all in `cmd` package:
- `cmd/alias.go` — `configFilePath("PORTAL_ALIASES_FILE", "aliases")`
- `cmd/clean.go` — `configFilePath("PORTAL_PROJECTS_FILE", "projects.json")`
- `cmd/hooks.go` — `configFilePath("PORTAL_HOOKS_FILE", "hooks.json")`

Env var overrides (`PORTAL_PROJECTS_FILE`, `PORTAL_ALIASES_FILE`, `PORTAL_HOOKS_FILE`) are user-facing features documented in the README. They bypass `configFilePath`'s directory logic entirely and are unaffected by this fix.

## Fix Approach

Replace `os.UserConfigDir()` in `configFilePath()` with XDG-compliant logic:

1. Check `XDG_CONFIG_HOME` environment variable first
2. If unset/empty, fall back to `~/.config` (via `os.UserHomeDir()` + `/.config`)
3. Append `portal/<filename>` as before

**Why not just hardcode `~/.config`:** On Linux, `os.UserConfigDir()` currently respects `XDG_CONFIG_HOME`. A naive fix that only uses `os.UserHomeDir() + "/.config"` would regress Linux users who have set a custom `XDG_CONFIG_HOME`.

**Env var overrides are unchanged:** The existing per-file env var check (`PORTAL_PROJECTS_FILE`, etc.) remains first in the resolution order and is unaffected.

## Migration

Existing macOS users have real data at `~/Library/Application Support/portal/`. A one-shot migration must move files from the old path to the new path.

**Files to migrate:** `projects.json`, `aliases`, `hooks.json`

**Migration behavior:**
- On first run after the fix, check if files exist at `~/Library/Application Support/portal/`
- If the target file already exists at `~/.config/portal/`, do not overwrite — skip that file
- If the target file does not exist, move the file from old path to new path
- Handle partial state: some files may be at old path, some at new
- Migration is macOS-only — Linux users are already at the correct path
- After migration, clean up the old `~/Library/Application Support/portal/` directory if empty

## Testing

### Config path tests (`cmd/config_test.go`)
- `configFilePath` returns `~/.config/portal/<file>` when no env var or `XDG_CONFIG_HOME` is set (not `os.UserConfigDir()`)
- `configFilePath` respects `XDG_CONFIG_HOME` when set
- Env var overrides (`PORTAL_PROJECTS_FILE`, etc.) still take precedence over both XDG and fallback paths

### Migration tests
- Migration moves files from `~/Library/Application Support/portal/` to `~/.config/portal/`
- Migration is a no-op when files are already at the correct path
- Migration does not overwrite existing files at the target path
- Migration handles partial state (some files at old path, some at new)
- Migration cleans up empty old directory after moving files

---

## Working Notes

[Optional - capture in-progress discussion if needed]
