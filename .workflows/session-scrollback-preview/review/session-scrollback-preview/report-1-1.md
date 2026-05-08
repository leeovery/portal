TASK: session-scrollback-preview-1-1 — Tail-N reverse-scan helper happy path

ACCEPTANCE CRITERIA:
- Helper exported from internal/state and resolvable from internal/tui adapter.
- File with K terminated lines where K > N → returns last N lines including final \n.
- File with fewer than N terminated lines → returns all available terminated lines.
- File whose last N lines span multiple read chunks → byte-identical to naive whole-file tail.
- Single os.File held across all chunk reads (no f.Close() in loop body, only via defer).
- Happy path return is (non-nil bytes, nil error).

STATUS: Complete

SPEC CONTEXT:
Spec § Read Pipeline mandates a tail-N idiom at the disk layer (open, seek-to-end, reverse-chunk scan, return only the last N newline-terminated bytes), decoupled from total file size. Single-fd invariant is explicit: helper opens once via os.Open, all Seek/Read go through that fd, close happens only after assembly — preventing torn reads under the daemon's atomic-rename. Three-outcome return contract ((bytes, nil) / (nil, nil) / (nil, err)) is established in spec § Architecture Summary > Test seams.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback_tail.go:55-127 (TailScrollback)
  - internal/state/scrollback_tail.go:132-144 (indexOfNthNewlineFromEnd helper)
  - internal/state/scrollback_tail.go:15 (chunk constant 64 KiB)
  - internal/state/scrollback_tail.go:20-31 (test seam openFileForTail / SetOpenFileForTest)
- Notes:
  - Signature matches plan: func TailScrollback(path string, n int) ([]byte, error). Exported from internal/state, callable from internal/tui without import cycles.
  - Single-fd invariant honoured: only one call to openFileForTail; defer func() { _ = f.Close() }() immediately after open (line 63); loop body (lines 85–115) contains no Close calls.
  - Reverse-scan algorithm correct: cursor walks backwards in 64 KiB strides via Seek(readAt, io.SeekStart) + io.ReadFull; chunks prepended (file-order) into growing tail buffer; loop terminates on bytes.Count(tail, '\n') >= n+1. Cut start = (n+1)th-from-last \n; slice end = last \n inclusive.
  - Fewer-than-N branch (lines 119–126) returns tail[:last+1] once loop exhausts file with fewer than n+1 newlines.
  - Implementation already includes task 1-2 (no-content) and task 1-3 (OS-error wrapping) branches — plan permits this layering.

TESTS:
- Status: Adequate
- Location: internal/state/scrollback_tail_test.go
- Coverage:
  - "returns the last N terminated lines when the file has more than N lines" (71–86) — 1500 lines, N=1000; byte-identity vs naiveTail oracle.
  - "returns all lines when the file has fewer than N" (88–102) — K < N case.
  - "returns exactly N lines when the file has exactly N lines" (104–118) — boundary; catches off-by-one.
  - "assembles the tail correctly when N lines span multiple chunk boundaries" (120–146) — multi-chunk path verified vs naiveTail.
  - "preserves the trailing newline on the returned bytes" (148–163).
  - "holds a single file descriptor across the reverse scan" (372–393) — asserts opens == 1 via the seam.
- Notes: Tests are focused, no redundancy. Each verifies one observable.

CODE QUALITY:
- Project conventions: Followed. Seam pattern consistent with portal's broader DI. Black-box package state_test. No t.Parallel().
- SOLID principles: Good. Single responsibility; seam respects open/closed.
- Complexity: Low. Reverse-scan loop reads cleanly with anchoring comments.
- Modern idioms: Yes. errors.Is(err, fs.ErrNotExist), %w wrapping, io.SeekStart/io.SeekEnd, io.ReadFull.
- Readability: Good. Comments anchor non-obvious choices (target = n+1 rationale, cut/last invariants, zero-newline collapse).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] merged := make([]byte, len(buf)+len(tail)) + double copy (lines 99–102) reallocates whole tail per iteration. Cumulative copy cost grows quadratically with chunk count for pathological inputs. Not actionable until task 1-4 benchmark proves it matters.
- [idea] indexOfNthNewlineFromEnd returns -1 as "unreachable given precondition". A defensive panic would fail loudly if a future refactor breaks the precondition.
- [idea] The "holds a single file descriptor" test uses 4000 lines × ~9 bytes (~36 KiB total — under one chunk), so it does not exercise the multi-iteration loop's single-fd behaviour. Combining the multi-iteration AND opens == 1 assertions into one test would be tighter; current coverage acceptable by code review.
