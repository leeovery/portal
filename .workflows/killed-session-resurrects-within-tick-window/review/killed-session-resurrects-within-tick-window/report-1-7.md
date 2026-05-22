TASK: Non-regression tests for daemon merge and six-event eventual consistency (killed-session-resurrects-within-tick-window-1-7)

ACCEPTANCE CRITERIA:
- Regression test 1 asserts on set of session names in `sessions.json` after daemon's post-`commit-now` tick (semantic, not byte-equivalence).
- Regression test 1 fails clearly if killed session reappears in daemon's next-tick output.
- Regression test 2 asserts each of six non-`session-closed` events is registered with `notifyCommand` and not `commitNowCommand`.
- Regression test 2 asserts firing each of six events touches `save.requested` and writes nothing to `sessions.json` within a bounded window.
- Tests gated to appropriate test lane; no `t.Parallel()`.

STATUS: Complete

SPEC CONTEXT: Spec § Non-Regression items 11 and 13: daemon's tick after `commit-now` keeps killed session out of `sessions.json` (semantic, not byte-equivalence); six non-`session-closed` events remain on cheap `notify` path. Task splits coverage between real-tmux daemon-merge integration and unit-level firing-side gate on `notifyCommand`, supplemented by per-event registration-level gate.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now_daemon_merge_integration_test.go` (regression test 1)
  - `cmd/state_notify_six_event_eventual_consistency_test.go` (regression test 2, firing-side)
  - `internal/tmux/hooks_register_six_event_routing_test.go` (regression test 2, registration-side companion)
- Notes:
  - `TestCommitNowDaemonMergeStability` drives the symptom fixture: kills B, polls `sessions.json` until B absent, forces daemon tick via `state.TouchSaveRequested`, waits for daemon to consume `save.requested` (load-bearing readiness signal). Asserts the **set** of session names.
  - `TestNotifyCommand_TouchesSaveRequestedAndWritesNoSessionsJSON` exercises `runStateNotify` and polls `sessions.json` non-existence on 25ms cadence within 500ms window (under daemon's 1s tick period).
  - Registration-side companion test names match the deliverable strings verbatim ("session-created is registered with notifyCommand and not commitNowCommand", etc.).
  - Both files explicitly document absence of `t.Parallel()` per cmd-package convention.

TESTS:
- Status: Adequate
- Coverage:
  - Daemon-merge regression: kills B, observes commit-now removed it, forces daemon tick, asserts B still absent and A still present (semantic, not byte-equivalence — matches spec item 11).
  - Six-event eventual consistency: split into (a) registration-level per-event named subtests in `internal/tmux/`, (b) firing-side `notifyCommand`-body invariant in `cmd/`.
- Notes:
  - 500ms firing window under 1s daemon ticker period — observed sessions.json within it is necessarily a regression.
  - 4s `daemonTickBudget` comfortable for CI jitter.
  - Failure messages include `fixture.diagnostic()` + state-dir listings.
  - Graceful skip via `tmuxtest.SkipIfNoTmux` and `portalbintest.StagePortalBinary`.

CODE QUALITY:
- Project conventions: Followed. Documents no-`t.Parallel`, uses `runStateNotify` helper, follows `cmd_test` vs `cmd` package conventions.
- SOLID: Good. Test helpers (`waitForSaveRequestedConsumed`, `dumpStateDirForNotifyTest`) narrow.
- Complexity: Low.
- Modern idioms: `errors.Is(err, fs.ErrNotExist)`, `context.WithTimeout`, `t.Setenv`.
- Readability: Good. Top-of-file comments cite spec sections.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Firing-side test polls 25ms × 500ms (~20 stat calls). A single check immediately after `runStateNotify` returns would suffice — `notify` is synchronous. Loop is belt-and-braces; worth a one-liner justification or simplification.
- [idea] `dumpStateDirForNotifyTest` duplicates similar dump helpers — extract shared helper if a third copy emerges.
- [quickfix] Two named subtests in `TestCommitNowDaemonMergeStability` share `present` map computed once outside the subtests. Minor; consider per-subtest read if `ReadIndex` itself were a regression vector.
- [idea] `waitForSaveRequestedConsumed` uses `time.Ticker` with `<-ticker.C` after initial sample — first re-check happens after `daemonTickPollInterval` rather than immediately. Acceptable given 4s budget.
