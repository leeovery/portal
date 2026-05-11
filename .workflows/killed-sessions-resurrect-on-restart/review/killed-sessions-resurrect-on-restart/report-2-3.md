TASK: killed-sessions-resurrect-on-restart-2-3 — Flip TestHydrate_Timeout_NeverFiresHookEvenIfRegistered into TestHydrate_Timeout_FiresHookWhenRegistered, then route runHydrate timeout fall-through through execShellOrHookAndExit

ACCEPTANCE CRITERIA:
- runHydrate timeout fall-through (after cfg.HandleTimeout returns nil) calls execShellOrHookAndExit(cfg), not execShellAndExit(cfg).
- TestHydrate_Timeout_FiresHookWhenRegistered passes with exec.target == "/bin/sh" and exec.args == ["sh", "-c", "<hook>; exec <shell>"].
- No new flag, parameter, or struct field added to runHydrate, hydrateConfig, or stateHydrateCmd. cfg.HookKey threads from existing scope.

STATUS: Complete

SPEC CONTEXT: Spec § Fix 2 → Specific Changes → 2 mandates the timeout fall-through share the file-missing recovery exec contract: unset marker, fire registered on-resume hook, exec sh -c '<HOOK>; exec $SHELL' (or bare $SHELL on no-hook). On-resume hooks are command-launchers independent of scrollback replay (§ Hook-Firing Safety on Timeout). No new --hook-key plumbing — runHydrate already holds cfg.HookKey.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/state_hydrate.go:115 — timeout fall-through calls execShellOrHookAndExit(cfg) after the 100 ms settle sleep.
  - /Users/leeovery/Code/portal/cmd/state_hydrate.go:228-246 — execShellOrHookAndExit body unchanged; reads cfg.HookKey internally.
  - /Users/leeovery/Code/portal/cmd/state_hydrate.go:44-56 — hydrateConfig unchanged.
  - /Users/leeovery/Code/portal/cmd/state_hydrate.go:347-403 — stateHydrateCmd flag wiring unchanged.
- Notes: Plan text references execShellOrHookAndExit(cfg.HookKey) shorthand; actual implementation passes full cfg. Consistent with existing call sites and matches func signature. Ordering: cfg.HandleTimeout(cfg) → time.Sleep(hydrateSettleSleep) → execShellOrHookAndExit(cfg). 100ms settle sleep preserved.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1412-1442 — TestHydrate_Timeout_FiresHookWhenRegistered asserts exec.target == "/bin/sh" and exec.args == []string{"sh", "-c", "echo hi; exec /bin/zsh"} via reflect.DeepEqual.
  - Mirrors TestHydrate_FileMissing_ExecsHookChainWhenHookRegistered.
  - Old NeverFiresHookEvenIfRegistered name absent from cmd/.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(). DI via hydrateConfig preserved.
- SOLID: Good. Clean separation: execShellOrHookAndExit / execShellAndExit / resolveShell.
- Complexity: Low. One-token swap unifies three call sites.
- Modern idioms: Yes.
- Readability: Good. Anchor comment at lines 112-114 cites spec.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] TestHydrate_TimeoutDoesNotReadHooksFile (state_hydrate_test.go:1174) name reads as a global invariant but post-flip the contract is "no read when HookStore is nil". Consider rename to TestHydrate_Timeout_NilHookStore_DoesNotReadHooksFile.
- [idea] Planning text uses shorthand execShellOrHookAndExit(cfg.HookKey); implementation passes full cfg. Planning-document inaccuracy, not an implementation defect.
