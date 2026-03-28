TASK: Tmux Server Option Methods

ACCEPTANCE CRITERIA:
- SetServerOption calls `tmux set-option -s <name> <value>` via the Commander
- GetServerOption calls `tmux show-option -sv <name>` via the Commander and returns the value
- GetServerOption returns ErrOptionNotFound when the Commander returns an error (option does not exist)
- DeleteServerOption calls `tmux set-option -su <name>` via the Commander
- DeleteServerOption succeeds (no error) when the option does not exist
- ErrOptionNotFound is exported from the tmux package
- All tests pass: `go test ./internal/tmux/...`

STATUS: Complete

SPEC CONTEXT: The "Volatile Marker Mechanism" section describes using tmux server-level user options as volatile storage. On hook registration, `set-option -s @portal-active-{pane_id} 1` is called. On execution check, the marker is queried with `show-option -sv`. On deregistration, the marker is removed with `set-option -su`. These markers die with the tmux server (tmux-resurrect does not restore options), so their absence indicates a server restart since registration.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tmux/tmux.go
  - Line 12: `ErrOptionNotFound` sentinel error
  - Lines 186-192: `SetServerOption(name, value string) error`
  - Lines 196-202: `GetServerOption(name string) (string, error)`
  - Lines 254-260: `DeleteServerOption(name string) error`
- Notes: All three methods follow the existing codebase patterns for error wrapping and Commander delegation. Each method matches its acceptance criteria exactly. `SetServerOption` passes `"set-option", "-s", name, value`. `GetServerOption` passes `"show-option", "-sv", name` and returns `ErrOptionNotFound` on any Commander error. `DeleteServerOption` passes `"set-option", "-su", name` and relies on tmux's natural no-op behavior for non-existent options.

TESTS:
- Status: Adequate
- Coverage: All 7 required tests are present and correctly verify behavior:
  1. "runs set-option -s with name and value" -- verifies exact args passed to Commander (line 575)
  2. "returns error when tmux command fails" -- verifies error wrapping includes method context and option name (line 595)
  3. "returns value when option exists" -- verifies return value and exact args (line 614)
  4. "returns ErrOptionNotFound when option does not exist" -- uses `errors.Is` to verify sentinel error, checks empty string return (line 637)
  5. "runs set-option -su with name" -- verifies exact args (line 653)
  6. "succeeds when option does not exist" -- verifies no error (line 673)
  7. "returns error when tmux command fails" -- verifies error wrapping includes context and option name (line 684)
- Notes: Tests use the existing `MockCommander` pattern consistently with other tests in the file. Tests verify both the args passed to the Commander and the return values. The `errors.Is` check in the GetServerOption error test ensures callers can properly detect the sentinel error. No over-testing or redundancy detected.

CODE QUALITY:
- Project conventions: Followed -- uses the same error wrapping pattern (`fmt.Errorf("failed to ... %q: %w", name, err)`) as existing methods like `KillSession`, `RenameSession`, `SwitchClient`. Uses `MockCommander` with `Calls` field inspection for arg verification. Doc comments on all exported symbols.
- SOLID principles: Good -- each method has a single responsibility, follows existing `Client` interface patterns, methods are small and focused.
- Complexity: Low -- each method is 3-5 lines, no branching except the single error check in `GetServerOption`.
- Modern idioms: Yes -- `errors.New` for sentinel, `errors.Is` in tests, proper error wrapping with `%w`.
- Readability: Good -- methods are self-documenting, naming is clear and consistent with existing patterns.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- `GetServerOption` maps all Commander errors to `ErrOptionNotFound`, which means a tmux connection failure would also appear as "option not found" rather than a transport error. This follows the plan's explicit instruction and is acceptable for the current use case (volatile markers are advisory), but worth noting if the method is reused in contexts where distinguishing "option missing" from "tmux unreachable" matters.
