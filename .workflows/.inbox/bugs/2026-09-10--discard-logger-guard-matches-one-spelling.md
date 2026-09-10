# The discard-logger guard matches one spelling

`internal/log/discard_guard_test.go` is meant to keep every silent logger on the
`log.OrDiscard` / `log.Discard` route so that a suite's log records are never quietly
thrown away. Its rule is a raw substring match on one literal —
`slog.NewTextHandler(io.Discard` — over the file text, not an AST rule, so it forbids
exactly one spelling of one construction:

- `slog.New(slog.DiscardHandler)` — the idiomatic silent logger since Go 1.24, and the
  one a contributor reaching for the stdlib would write today — passes.
- `slog.NewJSONHandler(io.Discard, nil)` passes.
- `w := io.Discard; slog.NewTextHandler(w, nil)` passes.

The consequence: a suite silenced through any of those bypasses `log.SetTestHandler` /
`logtest.Install` with no failure and no review signal. Its records go nowhere, so a later
change that stops emitting a record the suite should observe is invisible there, and the
closed logging vocabulary the guard exists to protect erodes one silent suite at a time.

The sibling guard one directory over already does this properly:
`internal/logtest/install_guard_test.go` is an AST rule with sanctions keyed by file and
function, a reverse check that each sanction still names a live site, and a fatal on
scanning nothing. The discard guard should take the same shape — judge every
`*slog.Logger` construction by its handler argument, sanction the capture-backed loggers
that legitimately wrap a sink (`internal/logtest/capture.go`,
`internal/restoretest/logger.go`, and the `slog.New(...)` sites in `cmd/logging_capture_test.go`,
`cmd/state_hydrate_exec_failure_test.go`, `cmd/state_daemon_self_eject_log_test.go`,
`cmd/bootstrap/{bootstrap,latch,orphan_sweep,stale_marker_cleanup}_test.go`,
`internal/tmux/hooks_register*_test.go`, `internal/tmux/portal_saver_test.go`,
`internal/state/capture_test.go`, `internal/restore/session_hydrate_exe_test.go`,
`internal/hooks/store_test.go`, `internal/hookstest/hooks_lock_test.go`), and forbid the
rest. Medium-sized because each of those sites needs a sanction or a rewrite onto the
route.

CLAUDE.md's `log` row has been corrected separately to describe the guard as it stands.
