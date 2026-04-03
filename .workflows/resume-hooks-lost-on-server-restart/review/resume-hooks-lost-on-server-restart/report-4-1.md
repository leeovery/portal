TASK: Consolidate Duplicate Hooks JSON Test Helpers

ACCEPTANCE CRITERIA:
- No duplicate hooks JSON helpers exist across cmd test files
- `writeHooksJSON` and `readHooksJSON` are defined exactly once in `cmd/testhelpers_test.go`
- All existing tests in `cmd/hooks_test.go` and `cmd/clean_test.go` pass unchanged

STATUS: Complete

SPEC CONTEXT: This task is a code quality improvement identified during Analysis (Cycle 1). The broader work unit migrated hook storage from pane-ID-based keys to structural keys (`session:window.pane`). During that migration, duplicate test helpers (`writeHooksJSON`/`readHooksJSON` in hooks_test.go and `writeCleanHooksJSON`/`readCleanHooksJSON` in clean_test.go) were identified as identical and targeted for consolidation.

IMPLEMENTATION:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/cmd/testhelpers_test.go:1-33`
- Notes: `writeHooksJSON` (line 10) and `readHooksJSON` (line 22) are defined exactly once in the new shared file. Both properly use `t.Helper()` for correct stack trace reporting. The old duplicate functions (`writeCleanHooksJSON`/`readCleanHooksJSON`) have been fully removed -- grep confirms no source files contain those names.

TESTS:
- Status: Adequate
- Coverage: The helpers themselves are exercised extensively by callers in both `cmd/hooks_test.go` (15 call sites) and `cmd/clean_test.go` (6 call sites). The task's acceptance test is `go test ./cmd/...` passing -- this is a refactoring task, not a behavioral change, so no new tests are needed.
- Notes: No concerns. Test coverage of the helpers is implicit through their widespread use by existing tests.

CODE QUALITY:
- Project conventions: Followed. File is `_test.go` suffixed, in `package cmd`, uses `t.Helper()` per Go conventions. No `t.Parallel()` used (per project rule).
- SOLID principles: Good. Single responsibility -- file contains only shared test helpers. No unnecessary abstractions.
- Complexity: Low. Two short, straightforward functions.
- Modern idioms: Yes. Uses `json.MarshalIndent`/`json.Unmarshal`, `os.WriteFile`/`os.ReadFile`, `0o644` octal literal.
- Readability: Good. Clear function names, doc comments, and error messages that include context about what failed.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
