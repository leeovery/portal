TASK: killed-sessions-resurrect-on-restart-4-6 — Promote shared WaitForFileExists sentinel-poll helper into internal/restoretest

ACCEPTANCE CRITERIA:
- restoretest.WaitForFileExists exists.
- pollUntilSentinelPresent and awaitSentinelExists deleted.
- Both call sites pass through the shared helper.
- Edge cases: choose 50ms canonical tick or make tick mandatory; diagnostic includes absolute path + elapsed time.

STATUS: Complete

SPEC CONTEXT: Phase 4 cycle 1 — duplicated sentinel-poll pattern across pollUntilSentinelPresent (cmd/bootstrap/phase2_hook_fire_integration_test.go:250-262) and awaitSentinelExists (internal/restore/exit_closes_pane_integration_test.go:461-476). Promotion creates a canonical primitive.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/restoretest/waitfor_file_exists.go
- Call sites:
  - /Users/leeovery/Code/portal/internal/restore/exit_closes_pane_integration_test.go:180
  - /Users/leeovery/Code/portal/cmd/bootstrap/phase2_hook_fire_integration_test.go:206
- Both pollUntilSentinelPresent and awaitSentinelExists are deleted.
- Doc entry in /Users/leeovery/Code/portal/internal/restoretest/doc.go:11-12.
- tick is mandatory (chosen alternative). Docstring records rationale.
- Diagnostic: "WaitForFileExists: %s did not appear within %v" — path + elapsed budget.
- Exported WaitForFileExists takes *testing.T; unexported waitForFileExists accepts a minimal `fataller` interface (Helper/Fatalf) so timeout branch is exercised in-process without aborting the runner.

TESTS:
- Status: Adequate
- Location: /Users/leeovery/Code/portal/internal/restoretest/waitfor_file_exists_test.go
- Coverage:
  - Happy path: file present at start.
  - Mid-poll arrival: file created after polling begins.
  - Timeout fatal: asserts Fatalf called, diagnostic contains both path and budget, Helper() invoked via fakeFataller.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Interface segregation via 2-method fataller seam.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Deadline loop uses time.Now().Before(deadline) then time.Sleep(tick); with very large tick relative to budget could overshoot by up to one tick.
- [idea] Fatalf diagnostic prefix is function name; including t.Name() would aid triage when multiple sentinel waits run in one test.
