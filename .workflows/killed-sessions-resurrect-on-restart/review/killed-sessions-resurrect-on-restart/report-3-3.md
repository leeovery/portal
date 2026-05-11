TASK: killed-sessions-resurrect-on-restart-3-3 — Add integration test (real-tmux fixture): typed `exit` once in a restored pane closes the pane (AC5)

ACCEPTANCE CRITERIA:
- New file `internal/restore/exit_closes_pane_integration_test.go` gated `//go:build integration`.
- Sub-tests TestExitClosesRestoredPane_NoHook, TestExitClosesRestoredPane_WithHook, TestNoParkedShWrapperPostRestore.
- All three skip under -short and when tmux unavailable.
- Sub-tests 1 and 2 assert pane closure within 2s polling window after sending exit\n.
- Sub-test 3 asserts `pgrep` returns zero matches.

STATUS: Complete

SPEC CONTEXT: Spec § Fix 3 (lines 173-217) replaces sh -c wrapper with bare form. Side-effects: no parked sh parent; exit closes pane on first invocation. AC5 attributes these to Fix 3. Inner `sh -c '<HOOK>; exec $SHELL'` envelope preserved.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/restore/exit_closes_pane_integration_test.go (481 lines)
- Notes:
  - //go:build integration tag and testing.Short() skip honoured.
  - tmuxtest.SkipIfNoTmux precondition.
  - Package `restore_test` (black-box).
  - All three required sub-test names present.
  - Shared scaffolding extracted into `setupExitClosesPane` (line 290).
  - Hook registration uses public hooks.NewStore.Set with tmux.PaneTarget.
  - awaitPaneGone handles "pane index absent" and "session gone" cases.
  - SendKeys appends Enter automatically.

TESTS:
- Status: Adequate
- Coverage: All three plan-mandated sub-tests present. 2s pane-gone budget for sub-tests 1 and 2. Sub-test 3 enforces no-parked-wrapper via pgrep exit-code semantics.
- Notes:
  - Marker-clearance barrier uses 10s budget (mirrors WaitForSkeletonMarkersCleared standard).
  - Sub-test 3 uses `pgrep -fl` rather than plan-specified `-fa` (portability fix: -a is Linux-specific). Session-suffix scoping constrains pattern.
  - pgrep exit-code semantics handled rigorously (0=match=fail, 1=no-match=pass, else fail loudly).
  - With-hook test uses WaitForFileExists to confirm inner sh -c chain reached HOOK before sending exit.
  - Diagnostic quality excellent.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel.
- SOLID: Good. setupExitClosesPane / driveAndAwaitMarkerClear / awaitPaneGone / sanitizeSessionForTmux.
- Complexity: Low.
- Modern idioms: Yes. t.TempDir(), t.Setenv, t.Helper(), named time constants.
- Readability: Excellent. Top-of-file block explains gating, portability constraints.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Sub-test 3's diagnostic could include captured paneKey for context.
- [idea] 2s time.Sleep(exitClosesPaneBudget) at line 229 is unconditional settle wait; could be poll-until-no-match loop.
- [idea] Deviation from plan's `pgrep -fa` to `-fl` is well-justified but a one-line note in planning doc would prevent future readers from flagging as drift.
- [quickfix] awaitPaneGone's wantPane formatting uses local format (window:pane), not tmux.PaneTarget (session:window.pane). Correct but could confuse future readers.
