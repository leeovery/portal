TASK: Corrections (killing-all-sessions-wipes-restore-state-3-4, tick-ec62c6) — make the `commit-now` subtest of `TestCommittersStandDownOnAFailedSessionListing` run with the committer's own tmux server set, so it stands down because of the failed listing rather than an unknown own server

ACCEPTANCE CRITERIA:
- With the committer's own tmux server set to the fake's answering pid, the `commit-now` subtest passes against the current tree. `commit-now` exits non-zero with an error wrapping `errCommitNowFailed`, and `sessions.json` and every scrollback file are unchanged. (§2.1, §6.1)
- Suppose the commit path read the failed listing as zero sessions. The subtest would then reach a confirmation the fake answers as the own server and commit, and it would fail its unchanged-state check. It would not stand down on an unknown own server. (§6.1)
- No production code changes, and no other test changes.

STATUS: complete

SPEC CONTEXT: §2.1 says a failed `list-sessions` is an error to the commit cycle: the cycle stands down instead of reading the failure as zero sessions, and this applies to all three committers (daemon tick, shutdown flush, `commit-now`). §2.2 says the own-server confirmation also stands a cycle down, and an unknown own server (zero) refuses every cycle. That second rule is why the old subtest proved nothing: under the package-wide poisoned `TMUX`, it stood down on "own server unknown" whatever the listing did. §2.5 sets `commit-now`'s stand-down route: `failCommitNow`, a non-zero exit wrapping `errCommitNowFailed`, with nothing written. §6.1 asks for a test of the failed listing on each of the three committers.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_commit_listing_failure_test.go:126 (`withOwnTmuxServer(t, fakeOwnServerPID)` as the first line of the `commit-now` subtest). The helper is at cmd/state_commit_own_server_test.go:17-20 and the pid at cmd/state_daemon_run_test.go:26.
- Notes:
  - The helper sets `TMUX` to "/nonexistent/portal-test-own-server,4242,0". `tmux.ServerPIDFromEnv` (internal/tmux/detect.go:17-27) reads the second-from-last field, so `ownTmuxServer()` (cmd/state_commit_now.go:128-131) returns 4242. The socket stays dead, so a tmux read the test forgot to inject still fails loudly.
  - AC1, traced on the current tree:
    - The fake's `list-sessions` returns a `*tmux.CommandError`. `ListSessionsProbe` (internal/tmux/tmux.go:164-170) passes it through as an error.
    - `captureStructure` wraps it in `ErrTmuxStoppedAnswering` (internal/state/capture.go:95-103). `classifyFailedCapture` returns it unchanged without sending a confirmation (internal/state/scrollback.go:380-383).
    - `RunCommitCycle` returns before the dump and the commit (internal/state/commit_cycle.go:127-130). `failCommitNow` then wraps `errCommitNowFailed` (cmd/state_commit_now.go:117-119, 143-149).
    - Nothing touches `sessions.json` or the scrollback dir. The `save.requested` touch lands outside the scrollback dir, which `assertUnchanged` does not read.
  - AC2, the counterfactual traced:
    - A swallowed listing gives no names, so `list-panes` is never read. No waiting tokens means no carry (capture.go:105-171), and the cycle ends with an empty index.
    - `confirmOwnServer` (scrollback.go:267-282) gets "4242" from the fake's `display-message`, because `answeringPID` is zero (cmd/state_daemon_run_test.go:128-138). That matches ownServer 4242, so it passes.
    - `commitOver` (internal/state/commit.go:35-61) sees a structural change from the two-session prior. It writes `sessions.json` and garbage-collects both seeded transcripts.
    - Both the `errCommitNowFailed` assertion and `assertUnchanged` would then fail. The subtest now discriminates.
  - AC3: commit f7354f32f is a one-line insertion in cmd/state_commit_listing_failure_test.go and touches no other file. Only that commit and the task 1-1 commit have touched this file, so HEAD holds exactly this change.

TESTS:
- Status: Adequate
- Coverage: The `commit-now` arm of the failed-listing rule now reaches the own-server confirmation, and a commit there would fail the test. With the tick and shutdown-flush subtests, which already ran with `OwnServer: fakeOwnServerPID` through `makeDeps`, all three committers in §6.1 are now covered for real.
- Notes: The change uses the same helper and pid as `TestCommitNowFailsOnAConfirmationFromAnotherServer` (cmd/state_commit_own_server_test.go:68-83). It adds no extra assertions and no new mocking.

CODE QUALITY:
- Project conventions: Followed. It uses the shared `withOwnTmuxServer` helper, which restores through `t.Setenv`. There is no `t.Parallel()` and no direct seam assignment.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
