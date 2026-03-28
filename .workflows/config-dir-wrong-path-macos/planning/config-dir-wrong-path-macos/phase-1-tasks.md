---
phase: 1
phase_name: Fix config path resolution and migrate existing files
total: 2
---

## config-dir-wrong-path-macos-1-1 | approved

### Task 1: Fix configFilePath to use XDG-compliant base directory

**Problem**: `configFilePath()` in `cmd/config.go` uses `os.UserConfigDir()` to determine the base config directory. On macOS, `os.UserConfigDir()` returns `~/Library/Application Support`, which produces paths like `~/Library/Application Support/portal/projects.json`. The project documents and expects `~/.config/portal/` on all platforms. The existing tests compare against `os.UserConfigDir()` itself, so they pass on all platforms without detecting the wrong path.

**Solution**: Replace the `os.UserConfigDir()` call in `configFilePath()` with XDG-compliant resolution: check `XDG_CONFIG_HOME` env var first; if unset or empty, fall back to `os.UserHomeDir()` + `/.config`. Keep the existing per-file env var override (`PORTAL_PROJECTS_FILE`, etc.) as the first check — that logic is unchanged. Update tests to assert against the literal expected path (`~/.config/portal/<file>`) rather than comparing against `os.UserConfigDir()`.

**Outcome**: `configFilePath("", "projects.json")` returns `$HOME/.config/portal/projects.json` on all platforms when no env var overrides are set. When `XDG_CONFIG_HOME` is set, it returns `$XDG_CONFIG_HOME/portal/projects.json`. Existing per-file env var overrides continue to work. Tests verify against concrete expected paths.

**Do**:
- In `cmd/config.go`, replace the `os.UserConfigDir()` call with:
  1. Check `os.Getenv("XDG_CONFIG_HOME")` — if non-empty, use it as the base config directory
  2. If empty, call `os.UserHomeDir()` and join with `.config` to produce the base config directory
  3. Return `filepath.Join(baseConfigDir, "portal", filename)` as before
- Remove the `"os"` import if it was only used for `os.UserConfigDir()` (it is still needed for `os.Getenv` and `os.UserHomeDir`)
- In `cmd/config_test.go`, rewrite the existing tests:
  - The env var override test can remain as-is (it already asserts a literal path)
  - The fallback test must assert against `filepath.Join(homeDir, ".config", "portal", "myfile")` using `os.UserHomeDir()` for `homeDir`, not `os.UserConfigDir()`
  - Add new test cases for `XDG_CONFIG_HOME` behavior and edge cases

**Acceptance Criteria**:
- [ ] `configFilePath("", "projects.json")` returns `$HOME/.config/portal/projects.json` when no env vars are set
- [ ] `configFilePath("", "projects.json")` returns `$XDG_CONFIG_HOME/portal/projects.json` when `XDG_CONFIG_HOME=/tmp/custom` is set
- [ ] `configFilePath("PORTAL_PROJECTS_FILE", "projects.json")` returns the env var value when `PORTAL_PROJECTS_FILE` is set (unchanged behavior)
- [ ] `configFilePath` returns an error when `os.UserHomeDir()` fails and no env vars are set
- [ ] All existing tests pass, updated to assert concrete paths rather than `os.UserConfigDir()` output
- [ ] `go build` succeeds with no compilation errors

**Tests**:
- `"returns ~/.config/portal/<file> when no env vars are set"` — unset both XDG_CONFIG_HOME and the per-file env var, assert path equals `$HOME/.config/portal/<file>`
- `"returns env var value when per-file env var is set"` — set PORTAL_PROJECTS_FILE to a custom path, assert that exact path is returned (existing test, kept as-is)
- `"respects XDG_CONFIG_HOME when set"` — set XDG_CONFIG_HOME to `/tmp/xdg-test`, assert path equals `/tmp/xdg-test/portal/<file>`
- `"treats empty XDG_CONFIG_HOME as unset"` — set XDG_CONFIG_HOME to `""`, assert path equals `$HOME/.config/portal/<file>` (same as default)
- `"per-file env var takes precedence over XDG_CONFIG_HOME"` — set both XDG_CONFIG_HOME and the per-file env var, assert the per-file env var value is returned
- `"XDG_CONFIG_HOME with trailing slash is normalized"` — set XDG_CONFIG_HOME to `/tmp/xdg-test/`, assert path does not contain double slashes (filepath.Join normalizes this)

**Edge Cases**:
- `XDG_CONFIG_HOME` set to empty string: treated as unset, falls back to `$HOME/.config`. Tested explicitly.
- `XDG_CONFIG_HOME` with trailing slash: `filepath.Join` normalizes this automatically. Tested explicitly.
- `os.UserHomeDir()` failure: returns a wrapped error. The spec notes this is an edge case. Test by verifying the error message format (cannot easily simulate `os.UserHomeDir` failure in unit tests without mocking, but the error path is straightforward — ensure the `fmt.Errorf` wrapping is present).

**Context**:
> The spec explicitly says "No special handling for trailing slashes or relative paths. `filepath.Join` normalizes trailing slashes, and matching Go's `os.UserConfigDir()` behavior (which doesn't validate either) is sufficient." This means we do NOT need to validate or reject relative paths in XDG_CONFIG_HOME.
>
> The spec also says "Why not just hardcode `~/.config`: On Linux, `os.UserConfigDir()` currently respects `XDG_CONFIG_HOME`. A naive fix that only uses `os.UserHomeDir() + '/.config'` would regress Linux users who have set a custom `XDG_CONFIG_HOME`."
>
> The existing per-file env var overrides (`PORTAL_PROJECTS_FILE`, `PORTAL_ALIASES_FILE`, `PORTAL_HOOKS_FILE`) bypass the directory logic entirely and are unaffected by this fix.

**Spec Reference**: `.workflows/config-dir-wrong-path-macos/specification/config-dir-wrong-path-macos/specification.md` — "Fix Approach" and "Testing > Config path tests" sections.

---

## config-dir-wrong-path-macos-1-2 | approved

### Task 2: Add one-shot file migration from old macOS path

**Problem**: Existing macOS users have real config data (`projects.json`, `aliases`, `hooks.json`) stored at `~/Library/Application Support/portal/`. After Task 1 fixes the path resolution, those files would be silently abandoned. Users would lose their projects, aliases, and hooks unless files are migrated from the old location to the new one.

**Solution**: Add migration logic inside `configFilePath()` that runs before returning the resolved path. For each call, it checks if the requested file exists at the old macOS path (`~/Library/Application Support/portal/<filename>`) and does not exist at the new path. If so, it moves the file via `os.Rename`. This is implicitly idempotent — once files are moved, the condition no longer holds. After moving, attempt to remove the old `~/Library/Application Support/portal/` directory if it is empty. Migration is best-effort: failures log a warning to stderr and do not block the caller. Platform detection is implicit — the old path simply won't exist on Linux.

**Outcome**: When `configFilePath()` is called and a file exists at `~/Library/Application Support/portal/<filename>` but not at the new XDG path, it is moved automatically. Existing files at the new path are never overwritten. The old directory is cleaned up when empty. Migration failures produce a stderr warning but do not return an error to the caller.

**Do**:
- In `cmd/config.go`, add a helper function `migrateConfigFile(oldPath, newPath string)` (unexported) that:
  1. Checks if `oldPath` exists (via `os.Stat`) — if not, return immediately (no-op)
  2. Checks if `newPath` already exists (via `os.Stat`) — if so, return immediately (do not overwrite)
  3. Calls `os.MkdirAll(filepath.Dir(newPath), 0o755)` to ensure the target directory exists
  4. Calls `os.Rename(oldPath, newPath)` to move the file
  5. If rename fails, log a warning to stderr via `fmt.Fprintf(os.Stderr, ...)` and return
  6. After successful move, attempt `os.Remove` on the old directory (`filepath.Dir(oldPath)`) — this only succeeds if the directory is empty, which is the desired behavior. Ignore the error from `os.Remove` (it will fail if directory is non-empty, which is fine).
- In `configFilePath()`, after computing the resolved new path (and before returning it), compute the old path as `filepath.Join(homeDir, "Library", "Application Support", "portal", filename)` and call `migrateConfigFile(oldPath, newPath)`. This must happen only on the fallback path (not when a per-file env var override is set).
- The `homeDir` variable (from `os.UserHomeDir()`) is already available in the fallback branch after Task 1's changes.
- In `cmd/config_test.go`, add migration test cases using `t.TempDir()` to create fake old/new directory structures. Override `os.UserHomeDir` behavior by using a helper that sets `HOME` env var (which `os.UserHomeDir` reads on Unix). Tests should:
  - Create a temp dir as the fake home
  - Create `Library/Application Support/portal/` under it with test files
  - Ensure `XDG_CONFIG_HOME` is unset so the new path resolves to `<fakeHome>/.config`
  - Call `configFilePath` and verify files were moved

**Acceptance Criteria**:
- [ ] Calling `configFilePath("", "projects.json")` when `projects.json` exists at `~/Library/Application Support/portal/` and not at `~/.config/portal/` moves the file to `~/.config/portal/projects.json`
- [ ] Calling `configFilePath("", "projects.json")` when `projects.json` exists at both old and new paths does NOT overwrite the new-path file
- [ ] Calling `configFilePath("", "projects.json")` when the old directory does not exist is a silent no-op
- [ ] After migrating the last file, the old `~/Library/Application Support/portal/` directory is removed if empty
- [ ] If the old directory still contains other files after migration, it is NOT removed
- [ ] If `os.Rename` fails, a warning is printed to stderr and `configFilePath` still returns the new path without error
- [ ] The target directory (`~/.config/portal/`) is created via `MkdirAll` if it does not exist before the rename
- [ ] Migration does NOT run when a per-file env var override is active
- [ ] [needs-info] Migration does NOT run when XDG_CONFIG_HOME is set (spec does not address this case — confirm intended behavior)
- [ ] `go test ./cmd/...` passes with all new and existing tests

**Tests**:
- `"migrates file from old macOS path to new path"` — create file at `<home>/Library/Application Support/portal/projects.json`, call `configFilePath`, verify file now exists at `<home>/.config/portal/projects.json` and is gone from old path
- `"migration is no-op when old directory does not exist"` — no old directory, call `configFilePath`, verify it returns correct new path without error
- `"migration does not overwrite existing file at new path"` — create files at both old and new paths with different content, call `configFilePath`, verify new path file content is unchanged
- `"migration handles partial state"` — create `aliases` at old path and `projects.json` at new path, call `configFilePath` for each, verify `aliases` is migrated but `projects.json` is not overwritten
- `"migration cleans up empty old directory"` — create old directory with one file, call `configFilePath` for that file, verify old directory is removed after migration
- `"migration preserves non-empty old directory"` — create old directory with two files, call `configFilePath` for only one, verify old directory still exists (other file remains)
- `"migration creates target directory if missing"` — ensure `<home>/.config/portal/` does not exist, create file at old path, call `configFilePath`, verify target directory was created and file was moved
- `"migration does not run when per-file env var is set"` — set `PORTAL_PROJECTS_FILE` to a custom path, create file at old macOS path, call `configFilePath`, verify old file is NOT moved
- `"migration does not run when XDG_CONFIG_HOME is set"` [needs-info] — set `XDG_CONFIG_HOME` to a custom path, create file at old macOS path, call `configFilePath`, verify old file is NOT moved
- `"migration logs warning on rename failure"` — this is difficult to test directly without OS-level mocking; consider testing `migrateConfigFile` directly with a read-only target directory to trigger a rename failure, and capture stderr to verify warning output

**Edge Cases**:
- Old file exists but new file also exists (no overwrite): checked via `os.Stat` on new path before attempting rename. Tested explicitly.
- Old directory does not exist (no-op): `os.Stat` on old path returns error, function returns immediately. Tested explicitly.
- Target directory does not yet exist (must MkdirAll): `os.MkdirAll` called before `os.Rename`. Tested explicitly.
- Rename failure (log warning and continue): `fmt.Fprintf(os.Stderr, ...)` with warning message, `configFilePath` returns the new path and nil error regardless. Tested via `migrateConfigFile` with permission-denied scenario.
- Old directory non-empty after partial migration (do not remove): `os.Remove` on a non-empty directory fails silently — error is ignored. Tested explicitly.
- Old directory empty after migration (remove it): `os.Remove` on empty directory succeeds. Tested explicitly.
- Permission denied on old file: `os.Stat` succeeds (file is visible) but `os.Rename` fails — handled by the rename failure path (log warning, continue).

**Context**:
> The spec states: "Migration runs inside `configFilePath()` itself — before returning the resolved path, it migrates only the single file it was called with." This means each caller triggers migration for its own file only, not all three files at once.
>
> The spec states: "Platform detection: Migration does not use `runtime.GOOS`. Instead, it simply checks whether the old path (`~/Library/Application Support/portal/`) exists." On Linux, `~/Library/Application Support/portal/` will not exist, so migration is implicitly a no-op.
>
> The spec states: "Use `os.Rename` for the move — both paths are under `$HOME`, always same volume." This means cross-volume rename failures are not a concern.
>
> The spec states: "Migration is best-effort. If a file move fails (e.g. permission denied), log a warning to stderr and continue with remaining files. A partial migration is acceptable — the next run will retry any files still at the old path. No user-visible output on success (silent migration)."
>
> The spec states: "`configFilePath()` only returns a path — it does not create directories. The existing callers already call `os.MkdirAll` before writing. Migration must also call `os.MkdirAll` on the target directory before moving files." This is about the migration code needing to create the target directory, not about changing `configFilePath`'s contract.

**Spec Reference**: `.workflows/config-dir-wrong-path-macos/specification/config-dir-wrong-path-macos/specification.md` — "Migration" and "Testing > Migration tests" sections.
