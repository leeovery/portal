TASK: Extract Atomic JSON Write Utility

ACCEPTANCE CRITERIA:
- `internal/fileutil/atomic.go` exists with `AtomicWrite` function
- `hooks.Store.Save` and `project.Store.Save` both delegate to `fileutil.AtomicWrite`
- No duplicated temp-file-rename pipeline in either store
- Error messages in the stores remain descriptive (wrap `AtomicWrite` errors with context if needed)

STATUS: Complete

SPEC CONTEXT: The specification states that the hooks store "Reuses the atomic write pattern from project/store.go". Both stores need atomic write (MkdirAll/CreateTemp/Write/Close/Rename) to safely persist JSON data. Extracting this into a shared utility eliminates duplication identified in analysis cycle 1.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/fileutil/atomic.go:1-42` — `AtomicWrite(path string, data []byte) error`
- `internal/hooks/store.go:11` imports `fileutil`, line 61 calls `fileutil.AtomicWrite(s.path, data)`
- `internal/project/store.go:12` imports `fileutil`, line 65 calls `fileutil.AtomicWrite(s.path, data)`
- No residual `CreateTemp`, `TempFile`, or `os.Rename` calls in either `internal/hooks/` or `internal/project/` packages
- Notes: Implementation is clean. The `AtomicWrite` function handles the full pipeline: `MkdirAll` -> `CreateTemp` -> `Write` -> `Close` -> `Rename`, with temp file cleanup on every error path. Both stores marshal JSON themselves and then delegate the raw bytes to `AtomicWrite`, which is the correct separation of concerns. Error messages from `AtomicWrite` are descriptive on their own ("failed to create directory", "failed to create temp file", "failed to write temp file", "failed to close temp file", "failed to rename temp file"), and the store `Save` methods add domain context for marshal errors ("failed to marshal hooks", "failed to marshal projects").

TESTS:
- Status: Adequate
- Coverage: `internal/fileutil/atomic_test.go` has 5 focused test cases:
  - writes data to file (happy path)
  - creates parent directories if missing (edge case from acceptance criteria)
  - overwrites existing file (idempotency)
  - leaves no temp files on success (cleanup verification)
  - writes empty data (edge case)
- The tests use `t.TempDir()` for isolation and verify both file contents and filesystem state
- The existing `hooks` and `project` store tests exercise `AtomicWrite` indirectly through `Save` calls, providing integration-level coverage
- Notes: Tests are well-balanced. They cover the documented edge cases (parent directory creation, temp file cleanup on success). The test for error paths (e.g., unwritable directory) is not present, but this is acceptable — the error wrapping is straightforward and the existing store tests exercise the happy path end-to-end. Not over-tested: each test verifies a distinct behavior.

CODE QUALITY:
- Project conventions: Followed. Package under `internal/`, uses standard Go error wrapping with `fmt.Errorf("%w", err)`, test file uses `_test` package suffix for black-box testing. No `t.Parallel()` (matching project convention).
- SOLID principles: Good. Single responsibility — `AtomicWrite` does exactly one thing. The stores are now cleaner with marshaling separated from persistence mechanics.
- Complexity: Low. Linear control flow with explicit error handling at each step. No branching complexity.
- Modern idioms: Yes. Uses `os.CreateTemp` (not deprecated `ioutil.TempFile`), `filepath.Dir`, standard error wrapping.
- Readability: Good. Function is self-documenting, temp file cleanup pattern is immediately clear, doc comment explains the strategy.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- Could add `data = append(data, '\n')` to ensure files end with a newline, but this is purely cosmetic and not required by any consumer.
