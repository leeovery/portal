AGENT: duplication
FINDINGS:

- FINDING: Six-branch CleanStale algorithm duplicated across bootstrap adapter and `portal clean` RunE
  SEVERITY: medium
  FILES: cmd/bootstrap_production.go:129-180, cmd/clean.go:65-141
  DESCRIPTION: Both callsites implement the same logical algorithm: open no-rotate logger → load hooks store → enumerate live panes → emit entry-point Debug → apply zero-live-panes hazard guard → call store.CleanStale → emit completion Debug. The format strings (`"stale-hook cleanup: live=%d persisted=%d"`, `"stale-hook cleanup: removed=%d"`, `"stale-hook cleanup: list-panes failed: %v"`, `"stale-hook cleanup: hookStore.Load failed: %v"`, and the multi-line hazard-guard warn) are repeated verbatim. The plan explicitly flagged this as intentional duplication on the grounds that the two callsites differ along three small axes: (i) `portal clean` has a pre-enumeration `persisted == 0` early-exit, (ii) `portal clean` swallows the `ListAllPanes` error while the bootstrap adapter propagates it, and (iii) `portal clean` writes user-facing "Removed stale hook:" lines. The implementation supports those differences faithfully, but the invariant parts are byte-identical. Drift risk is real: the integration tests in both `cmd/cleanstale_transient_listpanes_integration_test.go` and `cmd/cleanstale_transient_listpanes_clean_integration_test.go` substring-match on `"live=0"`, `"persisted=3"`, `"zero live panes"`, `"3 hook(s) present"`, `"mass-deletion hazard"`, and `"removed="` — if either callsite's wording drifts, the parallel assertion silently passes against the un-drifted callsite.
  RECOMMENDATION: Extract a single `runStaleHookCleanup(lister AllPaneLister, store *hooks.Store, logger bootstrap.Logger, policy listErrorPolicy, onRemoved func(string)) error` helper private to package `cmd`. Two policy axes (`policy=returnError` for the bootstrap adapter, `policy=swallow` for `portal clean`) and the optional `onRemoved` callback (nil for adapter, stdout writer for `portal clean`) cover the full delta. The `persisted==0` pre-enumeration early-exit stays at the callsite. Net effect: every format string declared once; future log-wording changes touch one site.

- FINDING: Phase 3-1 transient-list-panes scaffolding duplicated in Phase 3-2 (~280 lines)
  SEVERITY: medium
  FILES: cmd/bootstrap/transient_listpanes_helpers_integration_test.go:60-296, cmd/cleanstale_transient_listpanes_integration_test.go:95-297
  DESCRIPTION: `failureMode`, `transientListPanesCommander` (with `shouldIntercept`/`applyPolicy`/`intercept`/`Run`/`RunRaw`), `resolveHooksFilePathFromEnv`, `seedHooksJSON`, `hooksJSONBytes` are declared once in `package bootstrap_test` and re-declared in `package cmd` because Go does not export `_test.go` symbols across packages. The 3-3 file shares the `package cmd` copy. Bodies are near-duplicates, not byte-identical (the cmd copy strips OneShot and the smoke fixtures). Drift risk: argv-matching predicate and simulated error message are load-bearing for assertions in both packages.
  RECOMMENDATION: Promote `failureMode`, `transientListPanesCommander`, and helpers to a new test-only package (e.g., `internal/transienttest`) analogous to `tmuxtest`/`portaltest`/`restoretest`/`portalbintest`. Both packages import the shared symbols. Cost: one new package; benefit: zero duplication, drift impossible at compile time, OneShot available to both callsites.

- FINDING: `recordingLogger` test scaffolding overlaps with bootstrap package's recording logger
  SEVERITY: low
  FILES: cmd/bootstrap_production_test.go:33-68
  DESCRIPTION: `recordingLogger` + `recordedLog` reproduces a recording-logger pattern almost certainly present in `cmd/bootstrap/*_test.go` (notably `stale_marker_cleanup_test.go`). Shallow duplication (~40 lines); crossing the `cmd` ↔ `cmd/bootstrap` package boundary in test code is awkward, so cost-of-extraction is unfavourable.
  RECOMMENDATION: No action this cycle. If a future change touches both files, promote a `bootstraptest.RecordingLogger` helper alongside the integration-test extraction.

- FINDING: `cleanStaleAdapterT.CleanStale` mirrors production `cleanStaleAdapter.CleanStale` verbatim
  SEVERITY: low
  FILES: cmd/bootstrap_production.go:129-180, cmd/bootstrap_production_test.go:90-133
  DESCRIPTION: Test copies the 50-line `CleanStale` algorithm verbatim because production holds `client *tmux.Client` directly instead of an `AllPaneLister` interface. Drift mitigated only by integration coverage.
  RECOMMENDATION: Subsumed by the first finding's extraction. Once `runStaleHookCleanup` exists, `cleanStaleAdapter.CleanStale` becomes a one-line delegate and `cleanStaleAdapterT` is deleted; the four subtests retarget the shared helper directly.

SUMMARY: Three real extraction candidates plus one low-severity logger-scaffolding observation. The largest is the Phase 3-1 helpers duplicated in 3-2 (~280 lines, mechanically collapsible via a `internal/transienttest` package). The six-branch hazard-guard algorithm is duplicated in three places (two production callsites + test mirror); the integration tests substring-match the log strings, so drift between the two production callsites would silently pass.
