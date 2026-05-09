AGENT: standards
STATUS: findings
FINDINGS_COUNT: 2

FINDINGS:

- FINDING: Residual step-number drift in `cmd/bootstrap_production.go` (`cleanStaleAdapter` labelled "Step 8")
  SEVERITY: low
  FILES: cmd/bootstrap_production.go:51
  DESCRIPTION: Cycle 1 corrected the nine-step nomenclature in `cmd/bootstrap/bootstrap.go` and `CLAUDE.md`, but a sibling docstring in `cmd/bootstrap_production.go` was missed. The `cleanStaleAdapter` (which satisfies `bootstrap.StaleCleaner` — the `CleanStale` step) is documented as "Step 8 of the bootstrap sequence" but per the post-fix sequence it is now step 9 (CleanStaleMarkers=7, SweepOrphanFIFOs=8, CleanStale=9). Wiring is correct — only the docstring diverges.
  RECOMMENDATION: Change "Step 8 of the bootstrap sequence" → "Step 9 of the bootstrap sequence" on `cmd/bootstrap_production.go:51`.

- FINDING: Residual step-number drift in `adapters_test.go` (FIFOSweeper test labelled "step-7")
  SEVERITY: low
  FILES: internal/bootstrapadapter/adapters_test.go:41
  DESCRIPTION: The doc comment on `TestFIFOSweeper_PropagatesListSkeletonMarkersError` says "the orchestrator's step-7 Warn-and-swallow path can log it uniformly" — but `FIFOSweeper` is now step 8. The matching production-side docstrings in `internal/bootstrapadapter/adapters.go:107` and `:131` were correctly updated to "step-8 Warn-and-swallow"; only the test comment escaped the rename. Line 139 of the same file correctly references "step-7" for the `StaleMarkerCleaner` test and should not change.
  RECOMMENDATION: Change "step-7 Warn-and-swallow path" → "step-8 Warn-and-swallow path" on `internal/bootstrapadapter/adapters_test.go:41`.

SUMMARY: Cycle 1's nine-step renaming correctly landed in `cmd/bootstrap/bootstrap.go` and `CLAUDE.md`, but two sibling docstrings escaped the rename. Production wiring and behaviour conform to spec — only comments drift. No new violations were introduced by cycle 1.
