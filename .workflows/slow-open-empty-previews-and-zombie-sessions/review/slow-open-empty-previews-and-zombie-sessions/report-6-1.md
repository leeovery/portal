TASK: 6-1 — Build composite test harness with three-daemon scenario setup

STATUS: Issues Found (non-blocking)

SPEC CONTEXT: Composite End-to-End Verification mandates one integration test reconstructing reporter's three-daemon failure across A+B+C+D+E+F. Task 6-1 is foundational harness for 6-2..6-8.

IMPLEMENTATION:
- Status: Implemented with documented design adaptation
- Location: `cmd/bootstrap/composition_e2e_harness_integration_test.go` (469 lines)
- Adaptation: orphans spawned with their OWN PORTAL_STATE_DIR via `SpawnIsolatedDaemon`, then legitimate stateDir's daemon.pid OVERWRITTEN with orphan1's PID via `state.WritePIDFile`. Plan suggested shared stateDir but that would have orphans immediately losing daemon.lock (Component C) and exiting. Chosen pattern decouples flock from pgrep visibility; mirrors 4-5/orphan_sweep pattern
- Returns a struct rather than 6-tuple — cleaner for downstream consumers
- Seed-script divergence (deliberate): `while sleep 0.1; do echo "hello $RANDOM"; done` vs plan's `printf 'seed-output\n'; exec sh`

TESTS:
- Status: Under-tested relative to plan
- `TestCompositeHarness_PreState` covers: pgrep == 3; daemon.pid == orphan1PID; three distinct PIDs; both orphans alive; saver pane PID unchanged; user sessions observable
- Missing relative to plan:
  - explicit ppid check (`ps -o ppid= -p <pid>`) that orphan parents == test process — structurally guaranteed by `exec.Command` but not literally asserted
  - meta-test that `t.Cleanup` kills daemons even on assertion failure
  - explicit test of pgrep-skip behavior

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; harness single responsibility
- Complexity: Low; linear 8-step procedure
- Modern idioms: `t.Setenv`, `t.TempDir`, `t.Cleanup`, `t.Helper`
- Readability: Excellent; thorough file-header

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Add meta-test proving all three daemon PIDs dead after `t.Cleanup`
- [idea] Add `ps -o ppid= -p <orphan PID>` to close literal parent-PID divergence criterion
- [quickfix] Add sentinel constant `compositeHarnessOrphanCount = 2`
- [idea] Seed-string divergence from plan — keep but document, or revert if 6-2..6-8 depend on bounded output
- [quickfix] Consolidate shared helpers into `cmd/bootstrap/integration_helpers_test.go`
- [quickfix] Plan refers to `NewIsolatedStateEnv`; actual is `IsolateStateForTest`
