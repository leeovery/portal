TASK: session-tagging-and-grouping-3-2 — Resolve prefs.json via configFilePath + migrateConfigFile

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: per-file env-var override (PORTAL_PREFS_FILE) wins; XDG_CONFIG_HOME path; ~/.config fallback; migrate from old macOS path only when new absent; never overwrite existing; participates in migrateConfigFile.

SPEC CONTEXT: spec §206-210 prefs.json resolved through configFilePath like other config files; participates in migrateConfigFile. CLAUDE.md: prefs is NOT in closed audit-trail/log-component set, so migrate emission suppressed for it.

IMPLEMENTATION: Implemented.
- clean.go:213-218 prefsFilePath delegates to configFilePath("PORTAL_PREFS_FILE","prefs.json"); loadPrefsStore:205-211. config.go:96-118 configFilePath (env early-return precedes migration; XDG via xdg.ConfigBase; ~/.config fallback). config.go:18-30 configFileComponents maps "prefs.json":"" (suppress emission, still move). config.go:57-88 migrateConfigFile stat-old/stat-new guards (migrate only when new absent, never overwrite). Wired at open.go:464.

TESTS: Adequate. cmd/prefs_path_test.go — env override wins, XDG, ~/.config fallback; migrate-when-absent (content preserved), never-overwrite, no-migrate-with-env; TestPrefsMigrateSuppressesLog (empty-component mapping + move happens + zero log records); TestLoadPrefsStore round-trip. Behavioural.

CODE QUALITY: Conventions followed (reuses canonical configFilePath/xdg.ConfigBase; audit-trail exclusion precise; prefs avoids internal/log); SOLID good (thin wrappers, generic migrate reused); low complexity; os.IsNotExist/t.Setenv/t.TempDir idiomatic. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
