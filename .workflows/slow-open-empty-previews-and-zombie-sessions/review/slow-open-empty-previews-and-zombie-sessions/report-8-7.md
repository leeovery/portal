TASK: 8-7 — Rename killBarrierLogger to saverBarrierLogger

STATUS: Complete

SPEC CONTEXT: c2 Finding 3 — `killBarrierLogger` was reused by `waitForSaverDaemonReady` (Component F readiness barrier). "kill" prefix lied about scope.

IMPLEMENTATION:
- Status: Implemented (commit `dd8efc3228`)
- Location:
  - `internal/tmux/portal_saver.go:80-99` — `BarrierLogger` interface + `noopBarrierLogger` (interface name kept; already scope-accurate)
  - `internal/tmux/portal_saver.go:127-138` — `SaverBarrierSeams.Logger` field with doc explicitly calling out shared scope
  - `internal/tmux/portal_saver.go:298-303` — `SetBarrierLogger` setter writes to `saver.Barrier.Logger`
  - `internal/tmux/portal_saver.go:427, 439, 489` — three production WARN sites route through `saver.Barrier.Logger.Warn(...)`
  - `internal/bootstrapadapter/adapters.go:86` — production wiring `tmux.SetBarrierLogger(r.Logger)` unchanged
  - `internal/tmux/export_test.go:134-139` — `BarrierLoggerSeam` returns `&saver.Barrier.Logger`
- T8-6 had already eliminated bare package var; T8-7 landed killBarrierLogger-name-elimination on top
- Interface (`BarrierLogger`) and setter (`SetBarrierLogger`) retained verbatim — both already scope-accurate; renaming would have been pure churn

TESTS:
- Status: Adequate
- ~30 call sites for `recordingBarrierLogger`/`installBarrierLogger`/`BarrierLoggerSeam`
- `TestSetBarrierLogger_RoutesWarnOnTimeoutThroughInstalledLogger` (1861); `TestSetBarrierLogger_IgnoresNilLogger` (1899)
- ~50 test invocation sites use renamed plumbing without leakage

CODE QUALITY:
- Project conventions: Followed; seam-struct pattern preserved; nil-tolerance idiom
- SOLID: Good; small (1-method) interface
- Complexity: Low; pure rename / struct-field migration
- Readability: Improved; doc comments explicitly call out shared scope
- Grep of `internal/` and `cmd/` for `killBarrierLogger` returns zero matches outside historical workflow markdown

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Test-side identifiers use `Barrier` (not `SaverBarrier`) prefix matching production interface — consistent; flag only because future contributors searching for "saver" per spec's literal wording won't find these helpers
