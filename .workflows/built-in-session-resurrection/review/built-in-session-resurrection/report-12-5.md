# Review Report: built-in-session-resurrection-12-5

**TASK**: Fix task 6-2 logger migration in `state_migrate_rename` and `state_notify`

**ACCEPTANCE CRITERIA**:
- Open `*state.Logger` in `cmd/state_migrate_rename.go` (component: `state.ComponentHooks`).
- Open `*state.Logger` in `cmd/state_notify.go` (component: `state.ComponentNotify`).
- Replace stderr `fmt.Fprintf` calls with `Logger.Warn`/`Logger.Info`.
- WARN must fire on file-create failure in `notify`.
- `defer logger.Close()` in both files.
- Only fatal pre-logger errors permitted as stderr writes.
- `state.ComponentNotify` exists in `internal/state/logger.go` (or is added).

**STATUS**: Complete

**SPEC CONTEXT**:
Spec § Observability & Diagnostics → Log Rotation → Concurrent-writer discipline mandates that only the daemon rotates `portal.log`; all other writers append via `O_APPEND` with no size check. Spec § "Degrade locally, log, continue" requires diagnostics never to crash callers. `notify` and `migrate-rename` are the two short-lived non-daemon writers in the resurrection path, so they must use the non-rotating helper (`openNoRotateLogger`) introduced by task 6-2. Stderr is reserved for fatal pre-logger errors (e.g. EnsureDir failure in notify).

**IMPLEMENTATION**:
- Status: Implemented
- Locations:
  - `/Users/leeovery/Code/portal/cmd/state_migrate_rename.go:33-34` opens logger via `openNoRotateLogger()` and defers `Close()`.
  - `cmd/state_migrate_rename.go:66, 86, 93` route the three diagnostic sites through `logger.Warn(state.ComponentHooks, ...)`.
  - `/Users/leeovery/Code/portal/cmd/state_notify.go:34-39` keeps the EnsureDir fatal-pre-logger stderr path.
  - `cmd/state_notify.go:47-48` opens logger via `openNoRotateLogger()` and defers `Close()`.
  - `cmd/state_notify.go:53` emits `logger.Warn(state.ComponentNotify, "touch save.requested at %s: %v", path, err)` — the plan-mandated WARN-on-file-create-failure line.
  - `/Users/leeovery/Code/portal/internal/state/logger.go:34` already exports `ComponentNotify = "notify"`.
  - `/Users/leeovery/Code/portal/cmd/state_common.go:18-28` provides the shared `openNoRotateLogger`.
- Notes:
  - In `state_migrate_rename.go`, `loadHookStore()` runs before `openNoRotateLogger()`. Its only failure mode (no $HOME) returns a wrapped error, surfacing through cobra's printer to stderr — consistent with the fatal-pre-logger exception.
  - In `state_notify.go`, `state.EnsureDir()` is called explicitly (line 34) and again implicitly inside `openNoRotateLogger()` (line 47). EnsureDir is idempotent (`os.MkdirAll`).
  - Both files use `defer func() { _ = logger.Close() }()` immediately after open. `Close()` is nil-receiver-safe.
  - `logger, _ := openNoRotateLogger()` is intentional in both files — diagnostics-only failure must not abort the command.

**TESTS**:
- Status: Adequate
- Coverage:
  - `cmd/state_migrate_rename_test.go`:
    - `TestRunMigrateRename_CollisionLogsAndOverwrites` (line 158) asserts WARN level + `state.ComponentHooks` + collision message.
    - `TestRunMigrateRename_SaveFailurePropagatesAndWarns` (line 254) chmods parent dir to 0500 to force AtomicWrite failure.
    - `TestRunMigrateRename_EmitsHooksComponentToLogger` (line 352) is the explicit task 12-5 acceptance probe — drives the collision branch and asserts WARN + `| hooks |` column.
    - Eight non-warning paths use `state.NopLogger()`, confirming no spurious log lines on quiet paths.
  - `cmd/state_notify_test.go`:
    - `TestStateNotify_LogsWarnOnSaveRequestedCreateFailure` (line 204) plants a directory at the `save.requested` path so `os.OpenFile(O_WRONLY|O_CREATE|O_TRUNC)` fails with EISDIR, then asserts `portal.log` contains WARN + `state.ComponentNotify` + "save.requested".
    - Existing happy-path / EnsureDir-failure / mtime-bump / truncate / non-zero-exit tests round out the surface.
- Notes:
  - `newMigrateLogger` uses `t.Setenv("PORTAL_LOG_LEVEL", "info")` and a real `state.OpenLogger` — catches format drift at the byte level.
  - Per CLAUDE.md, no `t.Parallel()`.

**CODE QUALITY**:
- Project conventions: Followed. Cmd-package DI pattern; defer-Close idiom consistent with sibling commands.
- SOLID: Good. `runMigrateRename` accepts the logger as a parameter (DI).
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Godoc at both open sites explains rationale.
- Issues: None blocking.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] `cmd/state_notify.go:34-47` calls `state.EnsureDir()` twice. Idempotent so correctness-safe but mildly wasteful.
- [idea] Five files use `logger, _ := openNoRotateLogger()` discarding the open error. A helper centralising the policy would shrink duplication.
- [idea] `runMigrateRename` accepts a `*state.Logger` directly rather than a small interface. Couples tests to `state.NopLogger`/`state.OpenLogger`.
