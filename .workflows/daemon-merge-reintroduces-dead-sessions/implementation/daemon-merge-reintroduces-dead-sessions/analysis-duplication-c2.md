AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1

FINDINGS:

- FINDING: `staleClientStub` declared verbatim in two adjacent test files in `internal/bootstrapadapter`
  SEVERITY: low
  FILES: internal/bootstrapadapter/adapters_test.go:76-100, internal/bootstrapadapter/adapters_internal_test.go:87-115
  DESCRIPTION: The same struct (seven fields: showOut, showErr, listOut, listErr, listFormat, unsetCalls, unsetErr) and three methods (ShowAllServerOptions, ListAllPanesWithFormat, UnsetServerOption) are declared in both files. The white-box file's docstring explicitly acknowledges the duplication ("mirrors the stub in adapters_test.go but is duplicated here so the white-box test file stays self-contained — Go test files in the same package share symbols, but the external _test.go package's symbols are not visible from this internal-package test"). The acknowledgement is correct about the visibility constraint between `package bootstrapadapter` (white-box) and `package bootstrapadapter_test` (black-box). Consolidation is still possible: promote the stub to a `package bootstrapadapter` (non-`_test`) test fixture file with an exported name (e.g. `StaleClientStub`); the black-box file already imports `internal/bootstrapadapter` and could reference it as `bootstrapadapter.StaleClientStub`. ~25 lines saved + drift hazard mitigation if `staleMarkerClient` interface grows a method.
  RECOMMENDATION: Either (a) promote `staleClientStub` to an exported `StaleClientStub` in a shared file under `internal/bootstrapadapter` and have the black-box test file consume it via the existing import, or (b) leave as documented and accept the duplication (the in-source comment makes the trade-off explicit and ~25 lines sits at the low end of extraction thresholds).

SUMMARY: Cycle 1's three findings (daemon-tick helper, eleven-site Orchestrator literal, stateDir/logger preamble) are all cleanly resolved by the new shared helpers `runDaemonTick`, `buildIntegrationOrchestrator`, `openTestLogger`, and `newIntegrationStateDir` in `cmd/bootstrap/`. The cycle-1 refactor introduced no new cross-file duplication. The only remaining cross-file duplication is `staleClientStub` declared verbatim in two `internal/bootstrapadapter` test files because of Go's white-box/black-box package visibility split — already acknowledged in source comments.
