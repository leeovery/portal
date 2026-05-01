AGENT: standards
STATUS: findings
FINDINGS_COUNT: 1

FINDINGS:
- FINDING: AC #2 cobra Execute test stops short of asserting FIFO byte written
  SEVERITY: low
  FILES: cmd/state_signal_hydrate_test.go:622-647
  DESCRIPTION: Spec § "Acceptance Criteria" item 2 says the cobra-level argv parse test "must drive the cobra command tree via Execute() against a leading-dash positional argument and assert exit 0 + FIFO byte written". `TestStateSignalHydrate_AcceptsLeadingDashSessionViaCobraExecute` installs a no-op replacement for `signalHydrateRunFunc` that captures `cfg.Session`, asserts exit 0 and the captured session value, but does not exercise the FIFO byte-write path — the seam short-circuits `runSignalHydrate` before any FIFO is opened. The "FIFO byte written" half of AC #2 is satisfied indirectly by the integration test `TestRebootRoundTrip_LeadingDashSessionName` (which exec's the binary end-to-end and verifies `WaitForSkeletonMarkersCleared`), so combined coverage holds; the unit test alone does not match the literal AC wording.
  RECOMMENDATION: Either tighten the unit test to install a stub `OpenFIFO` and assert a byte was written through it (preserving the seam pattern but covering the byte-write clause), or add an explicit cross-reference comment in the test pointing to the integration test for the FIFO-byte half. No production code change required.

SUMMARY: Implementation conforms to the spec's two-part fix. Part 1 (`--` separator + one-shot migration with descending-index eviction, `command -v portal` + `portal state signal-hydrate` predicate, `--` absence requirement, INFO/WARN log shapes verbatim, idempotent on second bootstrap) and Part 2 (deletion of `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, `readIndexOption`) are both faithfully realised. One minor wording deviation on AC #2 split across unit + integration tests.
