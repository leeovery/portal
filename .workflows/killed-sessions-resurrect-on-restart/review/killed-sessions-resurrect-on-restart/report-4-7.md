TASK: killed-sessions-resurrect-on-restart-4-7 — Replace stale sh -c wrapper documentation in three integration-test comments

ACCEPTANCE CRITERIA:
- Replace stale `respawn-pane -k 'sh -c portal state hydrate ...; exec $SHELL'` doc strings with bare form.
- Drop `; exec $SHELL` trailer reference.
- Only update comments describing live behaviour; preserve historical-context notes.
- Target sites: cmd/bootstrap/eager_signal_hydrate_integration_test.go ~L117-118, ~L158-159; cmd/reattach_integration_test.go ~L73-74.

STATUS: Complete

SPEC CONTEXT: Spec § Fix 3 makes the bare hydrate invocation load-bearing for AC5. Comment-only cleanup with no behavioural surface.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:116-117
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:157-158
  - /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:73-74
  All three now read bare `respawn-pane -k 'portal state hydrate --fifo X --file Y --hook-key Z'` with no sh -c envelope and no trailer.
- Notes:
  - Edge case honoured: historical-context references in internal/restore/exit_closes_pane_integration_test.go correctly preserved — they describe the pre-fix wrapper as motivation for TestNoParkedShWrapperPostRestore.
  - Unrelated live sh -c references correctly untouched: cmd/state_hydrate.go:42,186,221-222 describe the inner hook-firing wrapper inside execShellOrHookAndExit (spec § Fix 3 → "Inner Hook-Firing Wrapper Is Untouched").
  - internal/tmux/tmux_test.go and internal/restore/session_build_hydrate_test.go references are exact-string fixtures / negative-assertion regression guards.

TESTS:
- Status: Adequate (no new tests required)
- Coverage: Pre-existing guards lock the invariant: session_build_hydrate_test.go:30-36 (negative strings.Contains(got, "sh -c")) and exit_closes_pane_integration_test.go:212-270 (TestNoParkedShWrapperPostRestore pgrep runtime guard).

CODE QUALITY:
- Project conventions: Followed.
- Readability: Improved.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
