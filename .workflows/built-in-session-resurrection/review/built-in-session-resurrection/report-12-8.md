# Review Report: built-in-session-resurrection-12-8

**TASK**: Wrap permission errors in `ReadIndex` with `ErrCorruptIndex`

**ACCEPTANCE CRITERIA**:
- Read-error branch in `ReadIndex` wraps with `ErrCorruptIndex` so downstream `errors.Is(err, ErrCorruptIndex)` classifies permission errors as soft warnings (matches Phase 5 task 5-2 / Phase 6 task 6-9 acceptance).
- Add Unix-only test (skip Windows; skip when running as root).
- Preserve existing return tuple shape `(Index, bool, error)`.

**STATUS**: Complete

**SPEC CONTEXT**:
- `ErrCorruptIndex` is the single sentinel covering "sessions.json exists but cannot be used" — malformed JSON, unsupported version, OR unreadable-but-present. Bootstrap surfaces these via `CorruptSessionsJSONWarning` (soft warning) without aborting.
- Absent file remains a clean-skip path (must NOT carry `ErrCorruptIndex`).

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/internal/state/index_reader.go:45`
- Read-error branch returns `fmt.Errorf("read sessions.json: %w: %w", ErrCorruptIndex, err)` (double-`%w`), matching the parse-error branch at line 50.
- `fs.ErrNotExist` short-circuits to `(Index{}, true, nil)` at lines 42-44, preserving the absent-file contract.
- Return tuple shape `(Index, bool, error)` unchanged.
- Doc-comment (lines 25-35) accurately describes the new behaviour.

**TESTS**:
- Status: Adequate
- Location: `/Users/leeovery/Code/portal/internal/state/index_reader_test.go`
- Coverage:
  - `TestReadIndex_PermissionDeniedWrapsErrCorruptIndex` (line 249) — chmods file to 0o000, asserts `errors.Is(err, ErrCorruptIndex)`. Skips on Windows (line 250) and when root (line 253). Cleanup restores 0o600.
  - `TestReadIndex_ReturnsSkipWithWrappedPermissionErrorWhenUnreadable` (line 213) — companion: asserts `skip=true`, error contains "read sessions.json", error is NOT `fs.ErrNotExist`.
  - Sibling `errors.Is` tests for parse (line 278) and version (line 291) branches plus negative test that absent file is NOT corrupt (line 308) form a complete classification matrix.
- Neither over- nor under-tested.

**CODE QUALITY**:
- Project conventions: Followed. No `t.Parallel()`. Standard Go multi-`%w` wrapping.
- SOLID: Good. Single sentinel, single classifier.
- Complexity: Low.
- Modern idioms: Yes. Multi-`%w` `fmt.Errorf`, `errors.Is`, `runtime.GOOS`/`os.Geteuid()`.
- Readability: Good.
- Issues: None.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] The two permission tests (`index_reader_test.go:213` and `:249`) duplicate ~15 lines of chmod-0o000 setup + cleanup. A small `seedUnreadableSessionsJSON(t, dir)` helper would dedupe.
- [idea] `TestReadIndex_PerformsNoStdoutOrStderrWrites` (line 320) doesn't exercise the permission-denied branch.
