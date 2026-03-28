TASK: Centralize Volatile Marker Name Format

ACCEPTANCE CRITERIA:
- `hooks.MarkerName` is the single source of truth for the marker name format
- No hardcoded `@portal-active-` string remains in `cmd/hooks.go` or `executor.go`
- All three usage sites call `hooks.MarkerName`

STATUS: Complete

SPEC CONTEXT: The spec defines volatile markers as tmux server-level options named `@portal-active-{pane_id}`, used to detect whether a hook was registered during the current server lifetime. The marker is set on `hooks set`, removed on `hooks rm`, and checked during `ExecuteHooks`. Consistency across all three sites is critical -- a mismatch would silently break the two-condition execution check.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Definition: `internal/hooks/executor.go:50-55` -- `MarkerName(paneID string) string` returns `fmt.Sprintf("@portal-active-%s", paneID)`
  - Usage 1: `internal/hooks/executor.go:101` -- `markerName := MarkerName(paneID)` in `ExecuteHooks`
  - Usage 2: `cmd/hooks.go:108` -- `hooks.MarkerName(paneID)` in `hooksSetCmd`
  - Usage 3: `cmd/hooks.go:158` -- `hooks.MarkerName(paneID)` in `hooksRmCmd`
- Notes: All three acceptance criteria are met. The hardcoded format string exists only inside `MarkerName` itself (line 54), which is the definition -- not a duplicated usage site. `cmd/hooks.go` has zero occurrences of the raw `@portal-active-` string. The function is exported and properly documented with a godoc comment.

TESTS:
- Status: Adequate
- Coverage: No new dedicated test for `MarkerName` was added, but this is a trivial pure function (single `Sprintf`). All existing tests that exercise the set/rm/execute flows implicitly verify `MarkerName` by asserting the expected marker name values (e.g., `@portal-active-%3`) in mock captures. These tests in `cmd/hooks_test.go` and `internal/hooks/executor_test.go` would fail if `MarkerName` produced incorrect output.
- Notes: Not over-tested -- a dedicated unit test for a single `Sprintf` wrapper would be redundant given the integration coverage. Not under-tested -- the existing tests verify the format at all three usage points.

CODE QUALITY:
- Project conventions: Followed. Function placed in the `hooks` package alongside `ExecuteHooks` where it is contextually relevant. Uses the project's existing pattern of small exported functions.
- SOLID principles: Good. Single responsibility -- one function, one concern (name formatting). DRY -- eliminates the triple definition of the format string.
- Complexity: Low. One-liner function.
- Modern idioms: Yes. Standard `fmt.Sprintf` usage.
- Readability: Good. Clear function name, descriptive godoc comment stating it is "the single source of truth for the marker naming convention."
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
