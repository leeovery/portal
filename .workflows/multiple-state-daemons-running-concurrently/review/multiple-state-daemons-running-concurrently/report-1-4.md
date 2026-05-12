# Review Report — Task 1.4

TASK: Flock-loser recovery via tolerant-kill-and-recreate

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- [x] `TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession` exists; asserts 1 new-session, 1 set-option, 0 kill-session (empty-session aftermath).
- [x] `TestBootstrapPortalSaver_RecoversFromFlockLoserDeadPaneSession` exists; asserts 1 kill-session, 1 new-session, 1 set-option, with kill ordered before new (dead-pane aftermath).
- [x] Both tests in `internal/tmux/portal_saver_test.go`; use existing `MockCommander` + `BootstrapAliveCheck` seams; no new seam introduced.
- [x] Tests pass without launching real daemons or real tmux.
- [x] No `t.Parallel()`.

SPEC CONTEXT:
Spec § Fix Part 1 → "Loser-daemon session aftermath" mandates two recovery shapes: (1) default tmux closes the session when the loser exits → next bootstrap sees `HasSession == false` → falls through to `createPortalSaverWithRetry`; (2) `remain-on-exit` keeps the session present with a dead pane → next bootstrap hits the stale-pidfile recovery branch and tolerant-kill-and-recreates. Spec § Test Strategy requires unit-seam coverage via `MockCommander` + `BootstrapAliveCheck`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/portal_saver_test.go:226-254` — empty-session test
  - `internal/tmux/portal_saver_test.go:265-309` — dead-pane test
- Notes:
  - Empty-session test scripts `hasSession` returning `("", errors.New("can't find session: _portal-saver"))`, with `newSession`+`setOption` returning success and `killSession` left nil (any unexpected kill-session would `t.Fatalf`).
  - Dead-pane test scripts `hasSession` returning `("", nil)` (present), stubs `BootstrapAliveCheck` to false via `stubAliveCheck(t, false)`, provides `killSession`/`newSession`/`setOption`.
  - Both tests use `shrinkRetryDelay(t)` and document spec sections in doc-comments.
  - Dead-pane test asserts kill-before-new ordering.

TESTS:
- Status: Adequate
- Coverage:
  - Both aftermath shapes covered.
  - Call-count + ordering assertions match acceptance criteria verbatim.
  - `portalSaverScript`'s nil-handler-fatal pattern guards against unexpected commands.
- Notes:
  - Phase 2 has landed: `BootstrapPortalSaver` now routes the dead-pane path through `killSaverAndWaitForDaemonFn`. The dead-pane test passes `stateDir = "/tmp/portal-state"` and relies on `/tmp/portal-state/daemon.pid` not existing at runtime so the helper short-circuits. Works on a clean dev machine but is environmentally coupled.

CODE QUALITY:
- Project conventions: Followed. Reuses existing helpers and seams. No new seam, no new helper.
- SOLID: Good — each test single-responsibility.
- Complexity: Low.
- Modern idioms: Idiomatic Go test style.
- Readability: Good. Doc-comments link each test to spec section.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Both new tests (and the precedent dead-daemon test at line 171) pass literal `"/tmp/portal-state"` to `BootstrapPortalSaver`. Now that Phase 2 wired `killSaverAndWaitForDaemonFn` into the dead-pane code path, the dead-pane test's correctness depends on `/tmp/portal-state/daemon.pid` not existing at test-run time. Two safer alternatives: (a) pass `t.TempDir()` instead, or (b) install `tmux.KillSaverAndWaitForDaemonFnSeam()` to a recording stub. Not required for correctness today.
- [quickfix] Both new tests' `hasSession` handlers ignore the `call int` argument. Matches existing pattern, harmless.
