# Config directory resolves to wrong path on macOS

Portal's specification, README, and discussion documents all state that configuration files live under `~/.config/portal/` following XDG conventions. The three config files — `projects.json`, `aliases`, and `hooks.json` — are documented at that path, and environment variable overrides (`PORTAL_PROJECTS_FILE`, `PORTAL_ALIASES_FILE`, `PORTAL_HOOKS_FILE`) exist as alternatives.

The implementation in `cmd/config.go` uses Go's `os.UserConfigDir()` as the base directory. On Linux this correctly returns `~/.config`, but on macOS it returns `~/Library/Application Support` — the Apple-native config path, not the XDG-compliant one. This means Portal silently writes all config to `~/Library/Application Support/portal/` on every Mac, diverging from what the spec promises.

The bug has been invisible because everything works fine at the wrong path — sessions are tracked, aliases resolve, hooks fire. The problem surfaces when users look for their config where the docs say it should be and find nothing, or when tooling (like the Claude Code resume hook script) checks `~/.config/portal/` and concludes Portal has never been used. The CLAUDE.md project instructions also reference `~/.config/portal/` which compounds the confusion during development.

Since Portal has been in use on macOS for weeks, real user data already exists at the `~/Library/Application Support/portal/` path. Simply changing the base directory to `~/.config/portal/` would silently abandon that data — projects, aliases, and hooks would vanish on the next run. The fix needs to include an auto-migration step: on startup, detect files at the old macOS-native path, move them to the correct XDG path, and proceed. This should be a one-shot operation with a brief log message so users know their config was relocated.

Relevant files: `cmd/config.go` (the `configFilePath` function that calls `os.UserConfigDir()`), `cmd/config_test.go`, the specification at `.workflows/v1/specification/portal/specification.md`, and `README.md`.
