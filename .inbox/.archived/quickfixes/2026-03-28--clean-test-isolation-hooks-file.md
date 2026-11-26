# Improve Test Isolation for Clean Command's Hook File Path

The existing project-only clean tests in `cmd/clean_test.go` don't set the `PORTAL_HOOKS_FILE` environment variable. This means if a developer happens to have a real `~/.config/portal/hooks.json` on disk, those tests could interact with it — reading real hook data or potentially modifying it during cleanup.

The newer hook-specific clean tests do set `PORTAL_HOOKS_FILE` to a temp path, so the gap is only in the older project-only tests that predate the hooks feature. Adding `t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))` to those tests would close the isolation gap and prevent any accidental interaction with real hook data.

Relevant file: `cmd/clean_test.go`, the pre-existing clean tests that only exercise project cleanup.
