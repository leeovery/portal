TASK: Delete the legacy internal/state logger and migrate its tests off OpenLogger/NopLogger (portal-observability-layer-1-10)

ACCEPTANCE CRITERIA:
- internal/state/logger.go deleted; internal/state exposes no Logger/Level/Component*/OpenLogger/NopLogger/LogRotateThreshold.
- No _test.go or production file references those symbols.
- restoretest.OpenTestLogger returns *slog.Logger writing to <stateDir>/portal.log; its test passes.
- Tests asserting pipe-delimited format now assert slog text format (or deleted).
- Tests depending on LevelWarn default updated to INFO default.
- go build/test green.

STATUS: Complete

SPEC CONTEXT:
Spec § Migration sweep — big-bang deletion of state.Logger type, Component* constants, pipe format, NopLogger(); silent loggers use slog.New(slog.NewTextHandler(io.Discard, nil)). WARN default replaced by INFO; "warning" alias dropped.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/logger.go deleted (only logger_nil.go remains); logger_nil.go:16-18 loggerOrDiscard forwards to log.OrDiscard; restoretest/logger.go:50-67 OpenTestLogger *slog.Logger over appended *os.File + t.Cleanup (later hardened by 11-2 to mirror production portal.log.<date>+symlink); restoretest/doc.go updated; recording fakes migrated to slog.Handler in cmd/bootstrap_production_test.go, cmd/bootstrap/bootstrap_test.go, tmux/hooks_register_test.go, tmux/hooks_migration_test.go.
- Notes: Grep over *.go for state.{Logger,OpenLogger,NopLogger,Level*,Component*,LogRotateThreshold} returns zero real usages (only migration_guard_test.go string literals). slog.LevelWarn hits are stdlib. log does not import state — no cycle.

TESTS:
- Status: Adequate
- Coverage: restoretest/logger_test.go (TestOpenTestLogger_WritesToPortalLog asserts *slog.Logger + slog text shape; TestOpenTestLogger_ProducesProductionSinkShape symlink + dated file). Deletion guarded by migration_guard_test.go::TestNoLegacyLoggerInProductionSource. Full retargeted suite compiles against *slog.Logger.
- Notes: grep-guard satisfies the "no source references deleted surface" requirement; no over-testing.

CODE QUALITY:
- Project conventions: Followed — io.Discard text handlers / log.OrDiscard; idiomatic slog.Handler capture fakes; centralized nil-tolerance.
- SOLID: Good — single-direction state→log dependency.
- Complexity: Low.
- Modern idioms: Yes — full log/slog adoption.
- Readability: Good — rationale comments on logger_nil.go and OpenTestLogger.
- Issues: One stale comment (see notes).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [bug] internal/state/status.go (scanRecentWarnings / logEntryQualifies) still parses the legacy pipe-delimited format (logFieldSeparator = " | "), and cmd/state_status_test.go + internal/state/status_test.go still write pipe-format lines. Production now writes slog TEXT format to portal.log, so `portal state status` "Recent warnings" will count zero against real daemon output — a latent functional regression in the health check. OUT OF SCOPE for 1-10 (the status reader is not the deleted logger and not in the task's file list) and not tracked by any other plan task. Flag for its own follow-up; does not block 1-10.
- [quickfix] migration_guard_test.go:24-29 excludedFromGuard + comment still exclude internal/state/logger.go "kept until its dedicated deletion task" — that task is done and the file is gone, so the exclusion is dead and the comment stale. Drop the entry + update comment.
