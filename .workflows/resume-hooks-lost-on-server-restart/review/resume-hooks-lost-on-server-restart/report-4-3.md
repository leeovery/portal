TASK: Rename Residual paneID Parameter Names to Structural-Key Terminology

ACCEPTANCE CRITERIA:
- No parameter named `paneID` or `livePaneIDs` exists in `internal/hooks/executor.go` or `internal/tmux/tmux.go` for these methods
- `SendKeys` parameter is named `target` in both the interface and the concrete implementation
- `CleanStale` parameter is named `liveKeys` in the interface (matching the concrete `Store` implementation)
- All tests pass unchanged

STATUS: Complete

SPEC CONTEXT: The spec requires structural keys (`session_name:window_index.pane_index`) to replace pane IDs throughout the codebase. Specifically, `CleanStale(liveKeys []string)` is called out as needing a parameter rename. `SendKeys` and other interfaces that previously dealt in pane IDs now deal in structural keys and their parameters should reflect that.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/executor.go:13` -- `SendKeys(target string, command string) error` (interface)
  - `internal/hooks/executor.go:35` -- `CleanStale(liveKeys []string) ([]string, error)` (interface)
  - `internal/tmux/tmux.go:258` -- `func (c *Client) SendKeys(target string, command string) error` (concrete)
  - `internal/hooks/store.go:130` -- `func (s *Store) CleanStale(liveKeys []string) ([]string, error)` (concrete)
- Notes: All four acceptance criteria for parameter naming are satisfied. The `ResolveStructuralKey(paneID string)` in `tmux.go:160` correctly retains `paneID` because it genuinely accepts a tmux pane ID (`%0`, `%1`) and converts it to a structural key. Similarly, `cmd/hooks.go` retains `paneID` in `requireTmuxPane()` and the `ResolveStructuralKey` interface, which is semantically correct since those values are actual pane IDs from `$TMUX_PANE`.

TESTS:
- Status: Adequate
- Coverage: The executor_test.go file has comprehensive tests using structural key values (`my-session:0.0`, etc.) throughout. The mock `CleanStale` uses `liveKeys` parameter name. The mock `SendKeys` uses `target` parameter name. Tests cover: basic execution, marker skipping, session filtering, error handling, multi-pane, orphaned keys, cleanup flow, and empty-pane guard.
- Notes: Tests pass with structural-key terminology. No test changes were needed beyond what earlier tasks already established (structural key values in test data).

CODE QUALITY:
- Project conventions: Followed -- small interfaces, DI via mocks, table-driven-style subtests
- SOLID principles: Good -- interfaces are focused (KeySender, HookCleaner), parameter names match their semantic meaning
- Complexity: Low -- pure rename of parameters, no logic changes
- Modern idioms: Yes -- standard Go naming conventions
- Readability: Good -- `target` and `liveKeys` communicate intent more clearly than the previous `paneID`/`livePaneIDs`
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The loop variable `paneID` in `executor.go:97` (`for paneID, events := range hookMap`) iterates over hookMap keys which are now structural keys, not pane IDs. Renaming this local variable to `key` or `structuralKey` would improve consistency with the rename effort. This is cosmetic and non-blocking -- the variable is purely local and the task scope explicitly says "parameters (not function/method names)".
- The test struct field `keySend.paneID` in `executor_test.go:31` and its references throughout the test file still use `paneID` naming even though the values stored are structural keys. Renaming to `target` would align with the interface rename. Non-blocking since it is test-internal.
- The mock field `mockHookCleaner.livePanesReceived` in `executor_test.go:100` retains "Panes" terminology despite receiving structural keys. Renaming to `liveKeysReceived` would be more consistent. Non-blocking.
