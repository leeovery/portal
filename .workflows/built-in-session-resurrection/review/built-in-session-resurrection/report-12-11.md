# Review Report: built-in-session-resurrection-12-11

**TASK**: Quick-fix — restore symmetry in `internal/state/logger.go` rename-failure reopen branch

**ACCEPTANCE CRITERIA**:
- Mirror diagnostic style of parallel branch in rename-failure reopen branch.
- Reopen failure on silent path needs forcing in test.
- Both branches consistently log diagnostic.

**STATUS**: Complete

**SPEC CONTEXT**:
Phase 12 cycle-1 review remediation. Original review finding (`internal/state/logger.go:184-188`) noted that the rename-failure reopen branch swallowed the reopen error silently while the parallel (post-rename) branch emitted a diagnostic.

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/internal/state/logger.go:183-201`
- Notes:
  - Rename-failure branch (lines 183-192): emits "portal: log rotation failed" then attempts reopen; on reopen failure emits "portal: log reopen failed" (line 188); assigns `l.f = f` (may be nil → next write no-ops, per docstring).
  - Successful-rename branch (lines 194-200): on reopen failure emits identical "portal: log reopen failed" diagnostic and sets `l.f = nil`.
  - Both branches now emit symmetric diagnostics.
  - Doc-comment at lines 158-162 accurately describes the post-failure invariants.

**TESTS**:
- Status: Adequate
- Coverage: New test `TestLogger_RotateRenameFailureReopenFailureEmitsBothDiagnostics` at `internal/state/logger_test.go:774-816` forces both failures simultaneously:
  - Seeds at threshold-50 so on-open rotation does NOT fire but the next 100-byte write triggers mid-write rotation.
  - chmod file to 0o000 (forces openAppendLog reopen to fail with EACCES).
  - chmod dir to 0o500 (forces os.Rename to fail with EACCES).
  - Captures stderr via os.Pipe and asserts BOTH "portal: log rotation failed" AND "portal: log reopen failed" diagnostics appear.
- Notes:
  - Skips on root (`os.Geteuid() == 0`) where chmod is bypassed.
  - t.Cleanup restores 0o700/0o600 perms.
  - Test docstring explicitly references the cycle/task ID.

**CODE QUALITY**:
- Project conventions: Followed — no t.Parallel(), idiomatic Go error handling.
- SOLID: Good — single-responsibility maintained.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — inline comment "Re-open the existing file so subsequent writes still land." clarifies intent.
- Issues: None.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] Both diagnostic strings ("portal: log rotation failed", "portal: log reopen failed") are duplicated as string literals across logger.go and logger_test.go. Consider extracting as unexported constants in a future cycle.
