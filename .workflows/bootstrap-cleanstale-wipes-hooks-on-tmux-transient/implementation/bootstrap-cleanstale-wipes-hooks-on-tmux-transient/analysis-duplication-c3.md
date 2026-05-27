AGENT: duplication
FINDINGS:

- FINDING: `stubAllPaneLister` (T4.1 add) shadows the pre-existing same-package `mockCleanPaneLister`
  SEVERITY: low
  FILES: cmd/bootstrap_production_test.go:65-73, cmd/clean_test.go:676-684
  DESCRIPTION: T4.1 introduced `stubAllPaneLister` as a `{panes []string, err error}` `AllPaneLister` stub. The pre-existing `mockCleanPaneLister` in `cmd/clean_test.go` is structurally identical — same package, same interface, same two fields, same body. Drift risk minimal; readers grep'ing for `AllPaneLister` stubs see two answers.
  RECOMMENDATION: Delete `stubAllPaneLister` and rewrite callsites to use `mockCleanPaneLister`. Alternative: accept the ~10-line duplication (below cycle-3 actionability bar).

SUMMARY: One residual low-severity test-stub duplication. Cycle-1/2 extractions otherwise hold cleanly.
