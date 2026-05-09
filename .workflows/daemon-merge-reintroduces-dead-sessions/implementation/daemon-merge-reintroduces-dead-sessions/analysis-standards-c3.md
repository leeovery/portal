AGENT: standards
STATUS: findings
FINDINGS_COUNT: 3

FINDINGS:

- FINDING: Stale FIFOSweeper "step 7" docstring drift in adapters.go
  SEVERITY: low
  FILES: internal/bootstrapadapter/adapters.go:77
  DESCRIPTION: The RestoreAdapter docstring says "...has been promoted to its own bootstrap step (FIFOSweeper / step 7)" — but per the renumbering, FIFOSweeper is step 8. The same file's FIFOSweeper docstring at line 93 correctly says "Step 8 of the bootstrap sequence". Cycle 1 addressed nine-step nomenclature drift; this site escaped.
  RECOMMENDATION: Update line 77 from "FIFOSweeper / step 7" to "FIFOSweeper / step 8".

- FINDING: Stale "step 7" references for FIFOSweeper in phase5_integration_test.go
  SEVERITY: low
  FILES: cmd/bootstrap/phase5_integration_test.go:225, 231, 272, 317
  DESCRIPTION: TestPhase5_FIFOSweeperRemovesOrphansAfterRestore's docstring and inline comments refer to FIFOSweeper as "step 7" four times. Per the new spec ordering, FIFOSweeper is step 8. Note: scrollback_resumption_test.go's "step 7" references are correct (CleanStaleMarkers); only the phase5_integration_test.go FIFOSweeper-attributed "step 7" references are wrong.
  RECOMMENDATION: Replace the four "step 7" references in phase5_integration_test.go with "step 8".

- FINDING: Stale incremental-task docstring in buildLiveStructure
  SEVERITY: low
  FILES: internal/state/capture.go:153-155
  DESCRIPTION: `buildLiveStructure`'s docstring says "Window/pane levels are populated now to keep the helper's shape stable as additional filtering levels land in subsequent tasks." This text was written when window/pane filtering had not yet landed; per the now-merged Fix Component A all three filtering levels are implemented in the same function. The forward-reference is misleading.
  RECOMMENDATION: Rewrite the trailing sentence to describe the helper's current responsibility: it builds the nested live-truth lookup consumed by the three-level filter in `mergeSkippedPanes`. No code change needed.

SUMMARY: Three low-severity docstring drifts after earlier cycles — one in internal/bootstrapadapter/adapters.go:77, four in cmd/bootstrap/phase5_integration_test.go, and one stale forward-reference in internal/state/capture.go's `buildLiveStructure` docstring. Implementation behaviour and spec conformance are otherwise clean.
