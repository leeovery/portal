TASK: Extract Shared Pane Structural-Key Resolution Helper in Hooks Commands

ACCEPTANCE CRITERIA:
- `resolveCurrentPaneKey` is defined once in `cmd/hooks.go`
- Neither `hooksSetCmd` nor `hooksRmCmd` contains inline pane-resolution logic -- both delegate to `resolveCurrentPaneKey`
- All existing hooks tests pass unchanged

STATUS: Complete

SPEC CONTEXT: The spec requires hook registration and removal to resolve the current pane's structural key (session:window.pane) from TMUX_PANE. Both `hooks set` and `hooks rm` previously duplicated this 12-line resolution block: read TMUX_PANE, resolve DI dependency, call ResolveStructuralKey, wrap error. This task extracts the duplication into a single helper.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/hooks.go:59-78 (resolveCurrentPaneKey definition)
- Location: /Users/leeovery/Code/portal/cmd/hooks.go:115 (hooksSetCmd delegates to helper)
- Location: /Users/leeovery/Code/portal/cmd/hooks.go:170 (hooksRmCmd delegates to helper)
- Notes: The helper correctly encapsulates: (1) requireTmuxPane() for env var validation, (2) hooksDeps nil-check with fallback to buildHooksTmuxClient(), (3) ResolveStructuralKey call, (4) descriptive error wrapping with %w. No inline pane-resolution logic remains in either command handler. requireTmuxPane is called only within resolveCurrentPaneKey. ResolveStructuralKey is called only within resolveCurrentPaneKey.

TESTS:
- Status: Adequate
- Coverage: resolveCurrentPaneKey is exercised indirectly through both hooksSetCmd and hooksRmCmd tests. The test suite covers: happy path resolution for both set and rm, TMUX_PANE missing error for both set and rm, ResolveStructuralKey failure for both set and rm, correct structural key propagation to store and volatile markers. No dedicated unit test for the helper itself is needed since it is thoroughly tested through command-level integration.
- Notes: Tests are focused and not over-tested. Each test verifies a distinct behavioral path. Mock injection via hooksDeps follows the project's established DI pattern.

CODE QUALITY:
- Project conventions: Followed -- uses the established hooksDeps DI pattern, unexported helper function, same error wrapping style as the rest of the codebase.
- SOLID principles: Good -- single responsibility (one function, one job), DI via interface (StructuralKeyResolver). The helper consolidates a cross-cutting concern without over-abstracting.
- Complexity: Low -- linear control flow with two early-return error checks. No branching beyond the nil-check for DI fallback.
- Modern idioms: Yes -- idiomatic Go error handling, fmt.Errorf with %w for error wrapping, interface-based DI.
- Readability: Good -- well-documented with a clear doc comment explaining purpose and behavior. Function name is descriptive. Code is self-documenting.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
