TASK: session-scrollback-preview-1-3 — Tail-N helper OS-error shape

ACCEPTANCE CRITERIA:
- Open error that is not ENOENT returns (nil, non-nil err); errors.Is(err, originalOSError) true via %w wrapping.
- Permission-denied file (mode 0000) returns (nil, non-nil err).
- Mid-scan read error returns (nil, non-nil err) and does not leak partial buffer.
- ENOENT continues to return (nil, nil) — task 1-3 does not change task 1-2's behaviour.
- File descriptor closed even on the error path.

STATUS: Complete

SPEC CONTEXT:
Per spec § Read-Failure Handling and § Architecture Summary > Test seams > ScrollbackReader > Return contract, the helper must return (nil, err != nil) for OS-level read failures so the call site in internal/tui renders the dedicated error string rather than the placeholder. ENOENT must remain on the (nil, nil) no-content branch from task 1-2. Errors must wrap via %w. No retries — single attempt per call.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/scrollback_tail.go:55-127
- Notes:
  - Open error branch (56-62): errors.Is(err, fs.ErrNotExist) returns (nil, nil); all other open errors wrap via fmt.Errorf("tail scrollback %s: %w", path, err).
  - Deferred close (63): defer func() { _ = f.Close() }() — runs on every return path including error sites.
  - Mid-scan errors: Seek (lines 65, 91) and io.ReadFull (line 95) all wrap with the unified prefix.
  - On error, partial tail buffer is discarded.
  - ENOENT preserved on (nil, nil) branch from task 1-2.
  - No retries; single attempt.

TESTS:
- Status: Adequate
- Location: internal/state/scrollback_tail_test.go:238-393
- Coverage:
  - "returns an error for a permission-denied file" (238-258) — chmod 0o000, asserts non-nil err, nil bytes; correctly skips on Windows and on root EUID.
  - "wraps the underlying OS error so errors.Is works" (260-289) — asserts errors.Is(err, fs.ErrPermission), plus tail scrollback prefix and path inclusion.
  - "preserves the (nil, nil) shape for ENOENT" (291-302) — explicit regression guard.
  - "returns an error from a mid-scan seek/read failure" (304-327) — uses SetOpenFileForTest seam to return a pre-closed *os.File.
  - "closes the file descriptor on the error path" (329-370) — captures *os.File via the seam, asserts subsequent Close() returns os.ErrClosed.
- Notes: All five acceptance criteria tested with focused, distinct assertions; no redundancy.

CODE QUALITY:
- Project conventions: Followed (standard Go, no t.Parallel(), external state_test package).
- SOLID: Good — single responsibility, indexOfNthNewlineFromEnd cleanly extracted, seam dependency-inverted.
- Complexity: Low — single linear scan; all three error sites use the same wrap prefix (DRY).
- Modern idioms: errors.Is, fmt.Errorf with %w, io.ReadFull (correctly surfaces short reads as errors).
- Readability: Good — doc comment (33-54) explicitly enumerates the three return shapes verbatim per the spec.
- Security: Good — read-only open, no info leakage in error wrap beyond caller-supplied path.
- Performance: Good — 64 KiB chunks, no allocations in error path beyond fmt.Errorf.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] merged := make(...); copy; copy reallocates the entire accumulated tail on every chunk read. For typical N=1000 this is fine; for pathological inputs O(n^2) in tail bytes. Out of scope for this task.
- [idea] io.ReadFull returns io.ErrUnexpectedEOF on short read, which would be wrapped — defensible if a future fixture concurrent-truncates.
- [quickfix] Test name "returns an error from a mid-scan seek/read failure" describes both seek and read but the test only exercises seek. Cosmetic only.
