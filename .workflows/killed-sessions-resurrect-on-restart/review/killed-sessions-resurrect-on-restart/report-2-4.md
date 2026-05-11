TASK: killed-sessions-resurrect-on-restart-2-4 — Unit test: runHydrate timeout fall-through with no registered hook still execs bare $SHELL via execShellOrHookAndExit

ACCEPTANCE CRITERIA:
- Unit test asserts runHydrate timeout fall-through with no registered hook still execs bare $SHELL via execShellOrHookAndExit.
- Edge cases: (1) nil HookStore degrades to bare shell, (2) lookup-not-found degrades to bare shell, (3) lookup-error degrades to bare shell with single WARN.

STATUS: Complete

SPEC CONTEXT: Spec § Fix 2 → Specific Changes → 2 requires timeout fall-through to route through execShellOrHookAndExit(cfg.HookKey). The execShellOrHookAndExit contract degrades silently to bare $SHELL on any of: nil HookStore, hooks.LookupOnResume returns ("", false, nil), or hooks.LookupOnResume returns an error. The lookup-error branch additionally emits a single WARN.

IMPLEMENTATION:
- Status: Implemented (test-only task)
- Location:
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1444 TestHydrate_Timeout_NoHookStore_ExecsBareShell (edge case 1)
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1474 TestHydrate_Timeout_LookupNotFound_ExecsBareShell (edge case 2)
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1504 TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning (edge case 3)
  - timeoutCfg helper at line 995, instantTimeoutOpenFIFO at line 988.
  - Production fall-through at cmd/state_hydrate.go:102-119.

TESTS:
- Status: Adequate
- Coverage:
  - Edge case 1: timeoutCfg leaves HookStore nil; assertion confirms bare-shell branch.
  - Edge case 2: seeds empty hooks.json; LookupOnResume returns ("", false, nil); bare-shell argv asserted.
  - Edge case 3: drives EISDIR via os.Mkdir on hooks path; asserts bare-shell argv + strings.Count == 1 single-WARN + hook-key in warning.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: reflect.DeepEqual, t.Setenv, strings.Count.
- Readability: Good. Each sub-test opens with 4-7 line doc-comment.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] EISDIR-via-mkdir fixture recurs at state_hydrate_test.go:1517-1521 and 1576-1580; could extract `seedUnreadableHookStore(t, dir)` helper.
- [idea] TestHydrate_Timeout_LookupError_ExecsBareShellAndLogsWarning and TestHydrate_LookupErrorDegradesToBareShellAndLogsWarning are near-identical except for the OpenFIFO seam; could be table-driven.
