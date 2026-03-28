AGENT: architecture
FINDINGS:
- FINDING: ExecuteHooks has 7 positional parameters where the same concrete object satisfies 4 of them
  SEVERITY: medium
  FILES: internal/hooks/executor.go:43, cmd/hook_executor.go:25
  DESCRIPTION: ExecuteHooks takes 7 interface parameters: lister, loader, sender, checker, allLister, cleaner. At the call site in buildHookExecutor, the tmux client is passed 4 times (lister, sender, checker, allLister) and the store is passed twice (loader, cleaner). This makes the function signature hard to read and the call site fragile -- the caller must pass the same object in the right positional slots. The 7-parameter count also violates the code-quality guidance on long parameter lists (4+).
  RECOMMENDATION: Group the tmux-side interfaces into a single composed interface (e.g., TmuxOperator embedding PaneLister, KeySender, OptionChecker, AllPaneLister) and the store-side into another (e.g., HookStore embedding HookLoader, HookCleaner). This reduces ExecuteHooks to 3 parameters: sessionName, tmux operator, hook store. The composed interfaces live in the hooks package alongside the small interfaces they embed, preserving ISP at definition boundaries while cleaning up the call site.
- FINDING: Volatile marker name format string duplicated across packages
  SEVERITY: medium
  FILES: cmd/hooks.go:107, cmd/hooks.go:152, internal/hooks/executor.go:80
  DESCRIPTION: The marker name format "@portal-active-%s" (or its string-concatenation equivalent "@portal-active-"+paneID) is constructed independently in cmd/hooks.go (set and rm commands) and internal/hooks/executor.go (ExecuteHooks). If the naming convention ever changes, all three sites must be updated in sync. This is a correctness-sensitive string -- if cmd and executor disagree on the format, the two-condition check breaks silently (markers set by hooks-set would never be found by ExecuteHooks, or vice versa).
  RECOMMENDATION: Define a single MarkerName(paneID string) function in the hooks package and use it from both cmd/hooks.go and executor.go. This makes the convention self-contained.
- FINDING: AllPaneLister interface defined identically in two packages
  SEVERITY: low
  FILES: cmd/clean.go:12, internal/hooks/executor.go:27
  DESCRIPTION: cmd/clean.go defines its own AllPaneLister interface identical to hooks.AllPaneLister. While Go's structural typing means this works, it introduces a maintenance risk -- if the method signature changes in one place, the other silently becomes a different interface. The cmd package already imports internal/hooks (via hook_executor.go), so there is no circular dependency concern.
  RECOMMENDATION: Use hooks.AllPaneLister in cmd/clean.go instead of re-declaring it. Alternatively, since both are single-method interfaces satisfied by tmux.Client, consider defining the canonical interface in the tmux package where the implementation lives and importing it where needed.
SUMMARY: The main structural issue is the 7-parameter ExecuteHooks signature that passes the same objects multiple times -- it should be grouped into composed interfaces. The duplicated marker name format string across packages is a latent correctness risk worth centralizing. The duplicated AllPaneLister interface is minor but easy to clean up.
