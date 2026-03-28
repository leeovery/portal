# Plan: Config Dir Wrong Path Macos

## Phases

### Phase 1: Fix config path resolution and migrate existing files
<!-- status: approved | approved_at: 2026-03-28 -->
**Goal**: Replace `os.UserConfigDir()` with XDG-compliant logic in `configFilePath()` and add one-shot migration of files from the old macOS path to the new path.

**Rationale**: This is a single root cause (wrong base directory function) with a necessary migration consequence. The fix and migration are coupled inside the same function and neither is useful without the other. Splitting them would leave either broken paths or orphaned data between phases.

**Acceptance Criteria**:
- [ ] `configFilePath` returns `~/.config/portal/<file>` when no env var or `XDG_CONFIG_HOME` is set (verified by test asserting against `os.UserHomeDir()` + `/.config/portal/<file>`, not `os.UserConfigDir()`)
- [ ] `configFilePath` respects `XDG_CONFIG_HOME` when set (e.g., `XDG_CONFIG_HOME=/tmp/xdg` yields `/tmp/xdg/portal/<file>`)
- [ ] Per-file env var overrides (`PORTAL_PROJECTS_FILE`, `PORTAL_ALIASES_FILE`, `PORTAL_HOOKS_FILE`) still take precedence over both XDG and fallback paths
- [ ] Migration moves files from `~/Library/Application Support/portal/` to `~/.config/portal/` when old path exists and new path does not
- [ ] Migration is a no-op when files are already at the correct path
- [ ] Migration does not overwrite existing files at the target path
- [ ] Migration handles partial state (some files at old path, some at new)
- [ ] Migration cleans up the old `~/Library/Application Support/portal/` directory if empty after file moves
- [ ] Migration failures log a warning to stderr and do not prevent `configFilePath` from returning the correct path
- [ ] All existing tests continue to pass

#### Tasks
<!-- status: approved | approved_at: 2026-03-28 -->

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| config-dir-wrong-path-macos-1-1 | Fix configFilePath to use XDG-compliant base directory | XDG_CONFIG_HOME set to empty string, XDG_CONFIG_HOME with trailing slash, os.UserHomeDir failure |
| config-dir-wrong-path-macos-1-2 | Add one-shot file migration from old macOS path | old file exists but new file also exists (no overwrite), old directory does not exist (no-op), target directory does not yet exist (must MkdirAll), rename failure (log warning and continue), old directory non-empty after partial migration (do not remove), old directory empty after migration (remove it), permission denied on old file |
