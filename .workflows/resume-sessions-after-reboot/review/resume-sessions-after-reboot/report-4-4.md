TASK: Group ExecuteHooks Parameters Into Composed Interfaces

ACCEPTANCE CRITERIA:
- `ExecuteHooks` takes exactly 3 parameters: `sessionName string`, `tmux TmuxOperator`, `store HookRepository`
- The small interfaces (`PaneLister`, `KeySender`, etc.) remain defined in the hooks package
- The call site in `cmd/hook_executor.go` passes `client` once and `store` once
- All existing test mocks compile and pass

STATUS: Complete

SPEC CONTEXT: The ExecuteHooks function is the core hook execution engine that checks each pane in a target session for hooks needing re-execution (persistent entry exists AND volatile marker absent). It was originally implemented with 7 individual interface parameters (the tmux client passed 4 times, the store passed twice). This task groups those into two composed interfaces while preserving the small interface definitions for interface segregation.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/hooks/executor.go:5-48 -- Small interfaces (PaneLister, KeySender, OptionChecker, HookLoader, AllPaneLister, HookCleaner) and composed interfaces (TmuxOperator, HookRepository)
  - /Users/leeovery/Code/portal/internal/hooks/executor.go:64 -- `func ExecuteHooks(sessionName string, tmux TmuxOperator, store HookRepository)` -- exactly 3 parameters
  - /Users/leeovery/Code/portal/cmd/hook_executor.go:13 -- `func buildHookExecutor(client hooks.TmuxOperator) HookExecutorFunc` -- passes `client` once as TmuxOperator
  - /Users/leeovery/Code/portal/cmd/hook_executor.go:19 -- `hooks.ExecuteHooks(sessionName, client, store)` -- passes `client` once and `store` once
- Notes: No drift. All six small interfaces remain individually defined (lines 6-34). TmuxOperator (lines 37-42) composes PaneLister, KeySender, OptionChecker, AllPaneLister. HookRepository (lines 45-48) composes HookLoader, HookCleaner. The `*tmux.Client` satisfies TmuxOperator (verified: has ListPanes, SendKeys, GetServerOption, SetServerOption, ListAllPanes). The `*hooks.Store` satisfies HookRepository (verified: has Load, CleanStale). No remaining references to the old 7-parameter signature in live code.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/internal/hooks/executor_test.go:115-148 -- Test mocks updated: `mockTmuxOperator` struct composes the four individual mocks via embedding, `mockHookRepository` composes the two store mocks. Helper constructors `noopTmux()` and `noopStore()` provide sensible defaults.
  - /Users/leeovery/Code/portal/internal/hooks/executor_test.go:150-569 -- All 15 test cases call `hooks.ExecuteHooks` with the 3-parameter signature (sessionName, tmux, store). Tests cover: hook execution with/without markers, pane scoping, error handling, multiple panes, cleanup integration, ListAllPanes/CleanStale errors.
  - Call sites in cmd tested via existing cmd tests (attach, open TUI, open path) that exercise `buildHookExecutor` indirectly.
- Notes: Test balance is good. The mock composition pattern (individual mocks embedded into composite structs) is clean and allows overriding specific behaviors per test. No over-testing -- each test verifies a distinct scenario. The `noopTmux`/`noopStore` helpers reduce boilerplate effectively.

CODE QUALITY:
- Project conventions: Followed. Uses the project's DI pattern (interface-based injection). Small interfaces defined at consumer site (hooks package). Follows the existing pattern of `buildSessionConnector` for the `buildHookExecutor` factory.
- SOLID principles: Good. Interface Segregation preserved -- six small single-method/few-method interfaces remain independently defined and usable. The composed interfaces (TmuxOperator, HookRepository) are a convenience layer that doesn't eliminate the small interfaces. Dependency Inversion maintained -- ExecuteHooks depends on abstractions, not the concrete tmux.Client or hooks.Store.
- Complexity: Low. The refactor simplified the call site from 7 parameters to 3 without adding any new logic.
- Modern idioms: Yes. Go interface composition via embedding is idiomatic.
- Readability: Good. The composed interfaces are well-documented with comments explaining their purpose. The function signature is now self-documenting: `ExecuteHooks(sessionName, tmux, store)` clearly communicates intent.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
