TASK: session-scrollback-preview-1-2 — Tail-N helper no-content shape

ACCEPTANCE CRITERIA:
- Tail("/does/not/exist", 1000) returns (nil, nil), no error wrapping ENOENT.
- Zero-byte file returns (nil, nil).
- File containing only an unterminated partial line returns (nil, nil).
- File "line1\nline2\npartial" returns []byte("line1\nline2\n"), nil — partial excluded.
- File containing only "\n" returns []byte("\n"), nil — terminated empty line is content.
- No no-content path returns a non-nil error.

STATUS: Complete

SPEC CONTEXT:
Spec § Read Pipeline > Trailing-newline edge case + § Architecture Summary > Test seams > ScrollbackReader > Return contract require collapsing ENOENT / zero-byte / zero-line into (nil, nil) so the placeholder/error branching at the internal/tui call site does not collapse. Files with terminated lines plus trailing partial return only the terminated portion. A bare \n is one terminated empty line and counts as content.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/scrollback_tail.go:55-127
- Notes:
  - ENOENT branch at lines 57-60: errors.Is(err, fs.ErrNotExist) returns (nil, nil); non-ENOENT open errors fall through to the wrap branch.
  - Zero-byte branch at lines 69-71: explicit size == 0 short-circuits before any reverse-scan work.
  - Zero-newline branch at lines 119-122: after exhausting the file, bytes.LastIndexByte(tail, '\n') < 0 returns (nil, nil).
  - Trailing-partial exclusion: both return paths (line 113, 126) slice to last+1.
  - Single bare \n: size == 1, reverse scan reads the byte, slice is "\n".

TESTS:
- Status: Adequate
- Location: internal/state/scrollback_tail_test.go
- Coverage:
  - "returns (nil, nil) for a missing file" — lines 165-176
  - "returns (nil, nil) for a zero-byte file" — lines 178-188
  - "returns (nil, nil) for a file with only an unterminated partial line" — 190-200
  - "excludes a trailing partial line from the returned tail" — 202-213
  - "preserves a single empty terminated line as content" — 215-226
  - "does not surface ENOENT as an error" — 228-236
- Notes: Each verifies a distinct branch; no redundancy. Belt-and-braces ENOENT-vs-error guard appropriately defensive.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(). Standard Go test helper style. Set*ForTest test seam pattern is idiomatic.
- SOLID: Good — single function, single responsibility (tail-N + three-shape return contract).
- Complexity: Low. Three early-return no-content branches are flat; reverse-scan loop is one branch deep.
- Modern idioms: Yes. errors.Is + fs.ErrNotExist, %w wrapping, io.SeekEnd/io.SeekStart named constants, defer close, no error-string matching.
- Readability: Good. Doc comment (33-54) enumerates the three "no content" outcomes and ties them to the spec contract.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] merged := make(...) allocates O(stride) per chunk-read iteration. Out of scope for 1-2; on radar if task 1-4's perf budget regresses.
- [idea] indexOfNthNewlineFromEnd is invoked exactly once at the found-enough branch where bytes.LastIndexByte already locates the final \n. Could be folded into a single backwards-walk.
- [quickfix] Doc comment example list mentions os.ErrClosed alongside fs.ErrPermission as errors.Is-recoverable wrapped errors. Cosmetic only.
