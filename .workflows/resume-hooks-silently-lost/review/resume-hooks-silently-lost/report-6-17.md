TASK: resume-hooks-silently-lost-6-17 — Register The State-Dir Teardown Guard On The Saver-Hosting Hook-Cleanup Fixture

ACCEPTANCE CRITERIA:
- The fixture registers the guard in the prescribed position.
- No saver-hosting integration fixture in the repo lacks the guard.
- The suite passes 5/5 consecutive runs.

STATUS: complete

SPEC CONTEXT: This is a phase-6 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context). The rule it enforces is CLAUDE.md's: "Fixtures whose tmux server can
host writers at teardown (a saver daemon's SIGHUP flush, session-closed hook subprocesses) should also call
portaltest.RegisterStateDirTeardownGuard(t, stateDir) — registered after IsolateStateForTest, before tmuxtest.New — so
the bounded quiescence wait runs between kill-server and the TempDir RemoveAll". The hazard is the reported
"TempDir RemoveAll: directory not empty" teardown flake: assertions pass, then the framework unlinks a state dir the
saver-hosted daemon is still flushing into.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_daemon_hook_cleanup_integration_test.go:90 — the named fixture's registration, sitting between
    portaltest.IsolateStateForTest at :80 and tmuxtest.New at :92, exactly the prescribed position. Teardown LIFO is
    therefore: kill-session _portal-saver (:129, registered after the socket, so it SIGHUPs the daemon first) →
    tmuxtest's kill-server → the guard's bounded wait → the isolated HOME TempDir RemoveAll and the fingerprint backstop.
  - The sweep (Do #2) landed the registration across the tree; today's set is 55 call sites, and each one I checked sits
    after its isolate call and before its tmuxtest.New: cmd/abridged_integration_test.go:34,
    cmd/bootstrap/composition_abc_integration_test.go:32, cmd/bootstrap/orphan_sweep_integration_test.go:60,117,172,
    cmd/bootstrap/upgrade_path_integration_test.go:43,119,165, cmd/concurrent_coldboot_integration_test.go:44,372,
    cmd/state_daemon_integration_test.go:77, cmd/state_daemon_hysteresis_measurement_test.go:237,
    cmd/state_daemon_self_supervision_integration_test.go:49,221,416,571,
    cmd/bootstrap/transient_listpanes_helpers_integration_test.go:67,160,
    internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:61.
  - Fixtures that take their state dir from a shared arrange carry the registration in that arrange:
    cmd/bootstrap/helpers_integration_test.go:25 (newIntegrationStateDir — covers reboot_roundtrip, eager_signal_hydrate,
    phase2_hook_fire, phase5_*, scrollback_resumption), cmd/doctor_fix_transient_listpanes_shared_integration_test.go:30
    (isolateCleanStaleTestEnv — covers the doctor --fix transient subtests; this was the fix-round finding, and it is
    applied), internal/restore/reboot_fixture_test.go:78 (newRebootFixture — covers multipane_legacy and the
    rename_reboot suites), cmd/bootstrap/composition_e2e_harness_integration_test.go:121
    (RegisterStateDirTeardownGuardWithPIDSource, since that harness deliberately overwrites daemon.pid).
- Notes: The task also added a structural guard, internal/portaltest/teardown_guard_coverage_test.go, which is more than
  the task body asked for but is the only thing that keeps criterion 2 true over time. It has since been reworked by
  later tasks (7-3, 8-11, 8-20, 10-1) from the original per-file check into a per-function rule that judges presence AND
  order and follows one hop into a same-package arrange; those revisions belong to their own tasks. Two of this task's
  own edits were also superseded downstream and correctly so: the three direct registrations it added to
  cmd/bootstrap/reboot_roundtrip_test.go moved into newIntegrationStateDir, and its plain-guard replacement in the
  composition e2e harness became the PID-source variant. Coverage is preserved in both cases, which I verified by
  reading the current call sites rather than the commit.

TESTS:
- Status: Adequate
- Coverage: The change under test is a test-harness fix, so its verification is (a) the tree-wide structural guard
  described above, which fails naming the offending file/function if a registration is dropped or misordered, and (b) the
  guard's own rule tests in internal/portaltest/teardown_guard_coverage_rule_test.go, which drive it over staged source
  fixtures for the missing-guard, missing-isolate, wrong-order, arrange-hop and nothing-qualifies cases. The guard is
  unit-lane and parses ignoring build tags, so it polices the integration-tagged files from the fast lane — correct per
  CLAUDE.md's lane rule, since it compiles and runs nothing.
- Notes: Criterion 3 ("passes 5/5 consecutive runs") is not verifiable by reading, and I ran nothing. The recorded
  evidence in fix-tracking-resume-hooks-silently-lost-6-17.md is stronger than the 5/5 anyway: the reviewer instrumented
  the guard and measured it closing a repeatable ~105ms window on this exact fixture (daemon PID alive ~52ms past
  kill-server, dir needing a further ~55ms to quiesce, stable across 3 runs). No new assertion was added to the fixture
  itself, which is right — the fixture's subject is the throttled hook cleanup, not its own teardown.

CODE QUALITY:
- Project conventions: Followed. Registration position matches CLAUDE.md's prescription at every site; the shared-arrange
  sites keep the same relative order; the guard test is stdlib + sourceguardtest only and unit-lane, matching the other
  repo-wide source guards.
- SOLID principles: Good. The hand-rolled partial wait previously inlined in the composition e2e harness was deleted in
  favour of the one shared helper, so there is a single implementation of the wait.
- Complexity: Low at the fixture sites (one call plus one comment line).
- Modern idioms: Yes.
- Readability: Good. The one-line "// LIFO runs this wait between kill-server and the TempDir RemoveAll." comment is
  repeated at the registration sites; it is accurate everywhere I checked, and the repetition is a matter of taste rather
  than a defect.
- Issues: None found.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
