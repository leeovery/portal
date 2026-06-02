TASK: Big-bang rewrite of all production state.Logger call sites to log.For + slog attrs (portal-observability-layer-1-9)

ACCEPTANCE CRITERIA:
- No production file calls state.Logger.{Debug,Info,Warn,Error} or references state.Component*/OpenLogger/NopLogger/openNoRotateLogger.
- Each migrated package binds var logger = log.For("<component>") once; call sites use logger.{Level}(msg, attrs...) with closed-vocabulary keys.
- No fmt.Sprintf/%s/%v inside a log message argument.
- Daemon no longer opens a rotating *state.Logger; daemonDeps.Logger is *slog.Logger = log.For("daemon"); no deferred logger.Close().
- open.go passes log.For("preview") to NewPreviewAttachPipeline; preview open + nil-fallback + Close gone.
- All *Deps/config *state.Logger fields are *slog.Logger.
- go build/test green (with 1-8/1-10).

STATUS: Complete

SPEC CONTEXT:
Spec § Migration sweep — single big-bang PR; component bound via log.For; closed 49-key vocabulary; terse messages; error attr via slog.Any (wrapped chain, not .Error()). Reviewed at FINAL feature state (Phase 5/6 components capture/signal/saver/clean/aliases/projects/process legitimately present).

IMPLEMENTATION:
- Status: Implemented (exceeds task — legacy logger.go already deleted)
- Location: component bindings cmd/state_common.go:17-44, internal/hooks/store.go:29, internal/alias/store.go:28, internal/project/store.go:33, internal/state/signal_hydrate.go:26, fifo_sweep.go:21, internal/tmux/hooks_register.go:19, portal_saver.go:20, cmd/bootstrap/bootstrap.go:53. Daemon: state_daemon.go:32 (*slog.Logger), :592/:637, no rotating open, no defer Close (only sb.Close() file handle remains). Preview: open.go:460 passes previewLogger; WARN at preview_attach.go:140. *Deps retyped throughout (state_cleanup.go, state_hydrate.go, state_signal_hydrate.go, state_commit_now.go, state_migrate_rename.go; internal/state bodies).
- Notes: Daemon's old "daemon: starting" line dropped (covered by process: start + daemon: lock acquired, rationale comment state_daemon.go:594-599) — Phase-5-era re-homing, no info silently lost.

TESTS:
- Status: Adequate
- Coverage: all six bullets covered — daemon capture WARN under daemon with pane_key + wrapped error (state_daemon_capture_logging_test.go:282, asserts wrapped sentinel text proving not .Error()); preview select-window WARN under preview; bootstrap step lines; hooks-register eviction WARN under bootstrap with error_class=unexpected + errors.Is (NOT .Error()); migration guard walks production .go for legacy symbols; error-chain assertions.
- Notes: Behaviour-focused via logtest.Sink + log.SetTestHandler. grep for err.Error() in log calls returns zero. No over-testing.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; snake_case closed keys; error attr passed directly; window-index with no closed key correctly dropped/kept in phrase).
- SOLID: Good — nil-tolerance via loggerOrDiscard/hydrateLoggerOrDefault/signalLoggerOrDefault.
- Complexity: Low (mechanical conversion).
- Modern idioms: Yes (log/slog, log.For, log.OrDiscard, log.Took).
- Readability: Good — component bindings documented in state_common.go incl component-vs-process_role orthogonality.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/log/migration_guard_test.go:28 lists internal/state/logger.go in excludedFromGuard but that file is deleted; exclusion is dead and the comment is stale. Drop the entry + update comment.
- [idea] Phase-1 task said not to pre-introduce capture/saver/signal components, yet state_common.go declares them — NOT drift for the post-Phase-5/6 end-state; noting only to make the phase boundary explicit.
