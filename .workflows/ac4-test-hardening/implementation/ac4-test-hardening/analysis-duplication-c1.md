AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1

FINDINGS:
- FINDING: AC4 negative-control duplicates ~35 lines of positive-case setup
  SEVERITY: low
  FILES: cmd/bootstrap/eager_signal_hydrate_integration_test.go:289-343, cmd/bootstrap/eager_signal_hydrate_integration_test.go:427-486
  DESCRIPTION: TestPhase1Integration_DaemonSkipsCaptureWithoutEagerSignal_AC4NegativeControl reproduces the entire fixture/wiring block from its positive sibling (TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4) verbatim — BuildPortalBinaryDir + PrependPATH + newIntegrationStateDir + SeedSessionsJSON(["alpha","beta"]) + tmuxtest.New + EnsureServer + has-session pre-condition loop + OpenTestLogger + buildIntegrationOrchestrator + Run + list-sessions sanity loop. The only load-bearing difference is the EagerSignaler field (auto-default vs NoOpEagerHydrateSignaler{}). With the AC1 sub-test's runEagerSignalMultiSessionAC1 helper, the same shape now appears three times in this file. The spec explicitly excludes new test helpers ("No new exports, no new test helpers — both atoms reuse existing scaffolding") and this duplication is structurally intentional under that constraint — each test is self-contained and readable end-to-end, and divergence risk is bounded (negative-control's whole purpose is to drift from the positive only at the EagerSignaler wiring).
  RECOMMENDATION: Accept as-is for this work unit — extraction is out of scope per the specification. If a fourth AC4-shaped integration test is ever added, revisit by extracting a runAC4Fixture(t, opts) helper that takes only the EagerSignaler choice (or a functional-option) as input; until then the explicitness aids reading each test as a standalone gate.

SUMMARY: One in-scope duplication observation: the new negative-control reuses the positive AC4 case's full fixture/wiring block almost verbatim. The specification explicitly forbids extracting a shared helper for this work unit, so the duplication is bounded by design; flagging for low-severity visibility only.
