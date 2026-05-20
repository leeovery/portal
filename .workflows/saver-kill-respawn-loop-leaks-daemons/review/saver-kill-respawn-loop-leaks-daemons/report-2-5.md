TASK: Integration test: daemon mid-tick + SIGHUP exits within bounded window (real-tmux fixture)

ACCEPTANCE CRITERIA: real-tmux integration test; tmuxtest fixture; no t.Parallel; multi-pane synthetic scrollback above 2s aggregate; fresh wall-time measurement anchoring the threshold (documented); assert exit within anchored threshold; assert exit within unchanged 5s killBarrierTimeout; assert clean (zero-status) exit; existing tests remain green; killBarrierTimeout unchanged in production.

STATUS: Complete

SPEC CONTEXT: Spec §Acceptance Criteria #7 mandates SIGHUP-to-exit latency bounded by one pane's capture wall time inside the unchanged 5s killBarrierTimeout. Spec §Testing Requirements > Integration tests #2 calls out the 2s figure as a heuristic and requires the implementation to take a fresh measurement and adjust. Pins Defect 2's end-to-end responsiveness contract that Tasks 2-1..2-4 were introduced to uphold.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon_integration_test.go (new file, ~608 lines). Test function at line 174.
- Notes:
  - Uses tmuxtest.SkipIfNoTmux + portalbintest.StagePortalBinary skip semantics matching the existing internal/tmux/portal_saver_integration_test.go pattern.
  - Spawns real `portal state daemon` subprocess with TMUX env pointing at the test socket, PORTAL_STATE_DIR at t.TempDir, and PORTAL_LOG_LEVEL=DEBUG for failure diagnostics.
  - bootstrapTmuxServer creates an _anchor session and sets global history-limit=1000000 before populatePanes runs.
  - populatePanes creates 12 panes each running `sh -c 'seq 1 500000; sleep infinity'` and polls capture-pane line count until ≥ lines for deterministic readiness.
  - measureSinglePaneCapture times one capture-pane -e -p -S - matching the argv shape used by state.CaptureAndHashPane.
  - anchorThreshold = max(2s, 2 × singlePaneWallTime); on derived threshold > 5s, halves scrollbackLines and rebuilds (max 1 retry); fatal if still over.
  - Sets @portal-restoring server-option before SIGHUP so defaultShutdownFlush takes the skip-flush branch — load-bearing for exercising the cancellation path rather than the non-cancellable final-flush path.
  - Uses daemon.Wait (not Process.Wait) on a goroutine bounded by killBarrierTimeoutCeiling+500ms so cmd.ProcessState is populated for the clean-exit assertion.
  - t.Cleanup SIGKILLs leaked daemons and emits a descriptive error.
  - Three assertions: latency < anchored threshold (line 439); latency < 5s ceiling (line 450); clean exit via exitErr == nil && ProcessState.Success (lines 460, 464).
  - Production killBarrierTimeout is untouched (5s); mirrored locally as a constant with comment.
  - No t.Parallel.

TESTS:
- Status: Adequate
- Coverage: primary anchored-threshold latency, 5s ceiling regression guard, clean exit, comprehensive failure-time diagnostics (portal.log dump, scrollback file count, daemon stderr). Measurement values logged unconditionally.
- Notes:
  - Optional "recycle-induced sweep pressure" test not implemented; flagged optional in the task brief.
  - Pre-SIGHUP save.requested existence is logged informationally rather than asserted.
  - Mid-tick timing relies on a static 1.2s sleep; worst-case timing analysis comment (lines 319-339) is thorough but slow-CI hosts could still flake into the idle-select window.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel per CLAUDE.md. Skip-on-missing-tmux mirrors existing integration tests.
- SOLID principles: Good. populatePanes, measureSinglePaneCapture, anchorThreshold, waitForDaemonAlive, bootstrapTmuxServer are single-responsibility helpers.
- Complexity: Acceptable. ~300-line test body is linear, numbered, and heavily commented.
- Modern idioms: Yes. errors.Is(err, os.ErrProcessDone), strings.Builder, time.NewTimer + deferred Stop.
- Readability: Good. Top-of-file docblock explains purpose, skip behaviour.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Optional "recycle-induced sweep pressure does not block cancellation" test from Task 2-5's Tests section is unimplemented. Adding it would explicitly pin spec §Defect 2 self-amplifying property by looping os.WriteFile(state.SaveRequested(...)) during the tick window.
- [idea] tickStartDelay is a static 1.2s sleep against the 1s ticker. On very slow CI cold-start, SIGHUP could land in the idle outer select rather than mid-tick. A deterministic gate would be more robust.
- [idea] On a capture-pane-fast host where aggregate < 2s, the test only logs a WARNING (line 252) instead of t.Skip — could pass trivially without exercising the load-bearing scenario.
- [quickfix] anchorThreshold docstring could read more explicitly as "minimum threshold 2s, scaling to 2 × singlePaneWallTime when measurement is large".
- [idea] killBarrierTimeoutCeiling mirrored locally as 5 * time.Second rather than imported from internal/tmux. An exported constant imported here would close the silent-desync gap.
