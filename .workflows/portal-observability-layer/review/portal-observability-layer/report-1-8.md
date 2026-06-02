TASK: Migrate intermediate logging seams off *state.Logger to *slog.Logger (portal-observability-layer-1-8)

ACCEPTANCE CRITERIA:
- No intermediate seam type references *state.Logger / state.Logger / state.Component*.
- bootstrap.Logger/NoopLogger removed or redefined to slog shape; orchestrator holds real *slog.Logger, never nil-receiver no-op.
- tmux.SetBarrierLogger/SetVersionWriterLogger/SaverVersionSeams.WriterLogger + MigrationLogger seam typed *slog.Logger; export_test VersionWriterLoggerSeam returns **slog.Logger.
- restore.Orchestrator.Logger, bootstrapadapter adapters, tui.previewAttachPipeline.logger typed *slog.Logger.
- Canonical silent logger is slog.New(slog.NewTextHandler(io.Discard, nil)) / shared helper; no nil-receiver shim remains.
- go build ./... succeeds once 1-9 lands.

STATUS: Complete

SPEC CONTEXT:
Spec § Migration sweep — single big-bang PR deleting state.Logger, pipe format; component bound per-package via log.For; closed 15-component + 49-key vocabulary. 1-8 is the seam-retyping half; legacy type deferred to 1-10.

IMPLEMENTATION:
- Status: Implemented (call-site rewrites from 1-9 also present, consistent with "lands together")
- Location: cmd/bootstrap/bootstrap.go:219-231 (Orchestrator.Logger *slog.Logger; nil via log.OrDiscard); orphan_sweep.go/eager_signal_hydrate.go/stale_marker_cleanup.go Logger fields; internal/tmux/portal_saver.go:128,175 (SaverBarrierSeams.Logger/SaverVersionSeams.WriterLogger; SetBarrierLogger/SetVersionWriterLogger; noopBarrierLogger removed; default log.Discard()); hooks_register.go (MigrationLogger removed; *slog.Logger params); export_test.go:147,181 (**slog.Logger); restore.go:35/session.go:55; bootstrapadapter adapters.go; tui/preview_attach.go:73,87; internal/state bodies all *slog.Logger via state.loggerOrDiscard→log.OrDiscard.
- Notes: Repo-wide grep for `\*state.Logger` returns zero (prod + test). Import-cycle guard holds: log does not import state; state imports log.

TESTS:
- Status: Adequate
- Coverage: all five enumerated acceptance tests present and behaviour-verifying (orchestrator slog construction + Run no-panic; SetBarrierLogger/SetVersionWriterLogger accept *slog.Logger incl nil-ignore; *slog.Logger threaded through RegisterPortalHooks migration asserting eviction INFO under bootstrap; NewRestoreAdapter/NewOrphanSweeper/NewPreviewAttachPipeline slog construction; VersionWriterLoggerSeam returns **slog.Logger). Standing migration guard walks all production .go and fails on legacy symbols.
- Notes: Behaviour-focused, would fail if broken (type mismatch won't compile; guard goes red). No over-testing.

CODE QUALITY:
- Project conventions: Followed (var logger = log.For(...) pattern; no t.Parallel; t.Cleanup seam restore).
- SOLID: Good — nil-tolerance centralized in log.OrDiscard / state.loggerOrDiscard.
- Complexity: Low (retype-and-forward).
- Modern idioms: Yes — io.Discard-backed slog replaces nil-receiver shim.
- Readability: Good — docstrings describe *slog.Logger contract, nil-ignore, discard defaults.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] export_test.go still imports internal/state, now used solely for state.IdentifyResult; a one-line comment would pre-empt a "did the import get removed?" question. Cosmetic.
- [idea] Two near-identical recording slog.Handler test doubles (recordingBarrierLogger, recordingMigrationLogger) in the tmux package could be consolidated; test-only, judgment call, out of scope for 1-8.
