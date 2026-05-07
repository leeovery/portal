---
phase: 1
phase_name: Read pipeline and structural enumeration foundations
total: 6
---

## session-scrollback-preview-1-1 | approved

### Task 1-1: Tail-N reverse-scan helper happy path

**Problem**: Preview's read pipeline must return at most the last N newline-terminated records from a `.bin` scrollback file in sub-millisecond time regardless of total file size. No existing helper performs a bounded tail read; daemon code reads whole files via `os.ReadFile`, which would balloon to ~3.7MB allocations per focus event for busy sessions and put the synchronous-in-`Update` decision out of budget.

**Solution**: Add a new helper in `internal/state` (alongside `ScrollbackFile`) that opens the target path, seeks to the end via `os.File` + `Seek(0, io.SeekEnd)`, reads backwards in fixed-size chunks against a single held file descriptor, counts `\n` bytes until N records are accumulated (or the file is exhausted), and returns the trailing terminated bytes. The helper takes `(path string, n int)` and returns `([]byte, error)` shaped per the three-outcome contract from the spec's `ScrollbackReader` description. Happy-path scope: this task delivers the core reverse-scan algorithm and validates it against fully-populated content. No-content and error-shape branches are tightened by tasks 1-2 and 1-3 respectively.

**Outcome**: A callable helper exists in `internal/state` that, given a `.bin` file with at least N newline-terminated lines, returns exactly the last N lines as raw bytes (preserving trailing `\n` on every returned line, no intermediate allocation of the whole file). Cost is decoupled from total file size — measurable only against N and chunk size.

**Do**:
- Create the helper in a new file under `internal/state/` (e.g. `internal/state/scrollback_tail.go`); name and exact filename are at the implementer's discretion per the spec's "Tail-N helper name and exact package location TBD" open item, but the helper must live alongside `ScrollbackFile` so import paths stay cohesive.
- Signature: `func TailScrollback(path string, n int) ([]byte, error)` (concrete name at implementer's discretion; whatever is chosen must be exported and callable from `internal/tui`'s production adapter for `ScrollbackReader.Tail`).
- Open the file once with `os.Open`; `defer` close. Do **not** close-and-reopen between chunk reads — the spec's single-fd invariant requires the entire reverse scan to run against the same fd so an atomic-rename mid-scan keeps reading the original inode.
- Use `Seek(0, io.SeekEnd)` to obtain the file size; if size is zero return `(nil, nil)` (no-content shape — fully validated in task 1-2).
- Choose a chunk size constant (e.g. 8 KiB or 64 KiB) and read backwards: maintain a cursor at the current end-of-tail, seek back by `min(chunkSize, cursor)`, read that slice, prepend it (logically) to an accumulating buffer, count `\n` bytes, repeat until either N+1 newlines have been seen (so the (N+1)th-from-last `\n` marks the split point) or the cursor reaches 0.
- Slice the assembled bytes from the byte immediately after the (N+1)th-from-last `\n` to the last `\n` inclusive, so all returned content is fully newline-terminated.
- If the file has strictly fewer than N terminated lines, return everything from start-of-file up to and including the last `\n` (a partial read is a successful read per spec § *Placeholder > Non-triggering condition*).
- Return `(bytes, nil)` for any successful read with at least one terminated line; defer the trailing-partial and zero-line shapes to task 1-2 (this task's tests must not exercise those branches).

**Acceptance Criteria**:
- [ ] Helper is exported from `internal/state` and resolvable from `internal/tui` adapter code (no circular imports).
- [ ] Given a file containing exactly K terminated lines where K > N, the helper returns the last N lines as a single `[]byte`, including the final `\n`.
- [ ] Given a file with fewer than N terminated lines (e.g. 5 lines, N=1000), the helper returns all available terminated lines without padding, error, or `(nil, nil)`.
- [ ] Given a file whose last N lines span multiple read chunks (i.e. the (N+1)th-from-last `\n` lies in an earlier chunk than the EOF), the assembled output is byte-identical to the naive whole-file `tail -n N`.
- [ ] A single `os.File` is held across all chunk reads (verifiable by code inspection — no `f.Close()` in the loop body, only at function return via `defer`).
- [ ] Return value for happy path is always `(non-nil bytes, nil error)`.

**Tests**:
- `"it returns the last N terminated lines from a file with more than N lines"` — fixture: 1500 distinct lines, N=1000; assert returned bytes equal lines 501..1500 joined with `\n` and trailing `\n`.
- `"it returns all lines when file has fewer than N"` — fixture: 5 lines, N=1000; assert all 5 lines returned, no error, non-nil bytes.
- `"it returns exactly N lines when file has exactly N lines"` — boundary: file has N=1000 lines and N=1000 requested; assert all 1000 returned, no off-by-one truncation.
- `"it correctly assembles tail when N lines span chunk boundaries"` — fixture: lines crafted so the (N+1)th-from-last newline is at least two chunk-sizes from EOF; assert output equals naive tail.
- `"it returns content with the trailing newline preserved"` — assert returned bytes end in `\n` for any non-empty success.
- `"it holds a single file descriptor across the reverse scan"` — assert via instrumentation (e.g. wrap `os.Open` in a test seam, or count opens by injecting an `openFunc`) that exactly one open is performed per call. If no clean seam is reasonable, omit this test and rely on code review against the single-fd invariant — note the choice in the test file.

**Edge Cases**:
- File with fewer than N lines returns all lines (not `(nil, nil)`).
- File with exactly N lines returns all N (no off-by-one).
- Tail spanning multiple chunk boundaries assembles correctly (the chunk stride is an implementation detail; the test must not assume a specific chunk size).

**Context**:
> Spec § *Read Pipeline*: "the read is implemented as a tail-N idiom at the disk layer: open the file, seek to end, read backwards in chunks until N newlines are collected, return only those bytes. Cost is decoupled from total `.bin` file size — a 3.7MB / 50k-line file reads at the same sub-millisecond cost as a small one. No full-file read, no `strings.Split` allocation of the whole file per cycle keypress."
>
> Spec § *Read Pipeline > Single-fd invariant*: "The helper opens the file once via `os.Open`, performs all `Seek` and `Read` calls against that single file descriptor, and closes only after the tail bytes are assembled. … a close-and-reopen between chunks would expose a torn-read window where the daemon's atomic rename could swap the inode mid-scan."
>
> Spec § *Read Pipeline > Definition of "line"*: "A line is a `\n`-terminated record … The reverse scan counts newline bytes. There is no re-wrap, no logical-line reconstruction, and no consultation of the source pane's display width."
>
> Spec open item: "Tail-N helper name and exact package location TBD by build phase."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *History Depth > Read Pipeline*, § *Cross-cutting Seams > State Package API Reuse*, § *Architecture Summary > Test seams > ScrollbackReader > Return contract*.

## session-scrollback-preview-1-2 | approved

### Task 1-2: Tail-N helper no-content shape

**Problem**: The preview placeholder ("(no saved content)") is gated on the helper returning `(nil, nil)` for ENOENT, zero-byte files, and files containing only an unterminated partial line. Without these branches the call site in `internal/tui` cannot distinguish "file missing" from "OS error" from "file present with content", and the placeholder vs error string branching collapses.

**Solution**: Extend the helper from task 1-1 so that all "no content available" outcomes converge on `(nil, nil)`: ENOENT on open, zero-byte file size after `Seek(0, io.SeekEnd)`, and any reverse scan that finds zero `\n` bytes (i.e. the entire file is one unterminated partial record). When the file has terminated lines plus a trailing partial fragment, the partial fragment must be excluded from the returned tail — the helper returns only the fully-terminated lines.

**Outcome**: Three "no content" cases (ENOENT, zero bytes, zero terminated lines) all return `(nil, nil)`. A file with K terminated lines plus a trailing partial returns only the K terminated lines as `(bytes, nil)`, never including the partial fragment.

**Do**:
- In the helper from task 1-1, when `os.Open` returns an error, check `errors.Is(err, fs.ErrNotExist)`; on match, return `(nil, nil)`. Other open errors flow through to task 1-3's branch.
- After `Seek(0, io.SeekEnd)`, if size == 0, return `(nil, nil)` immediately (do not attempt reverse scan).
- During reverse scan, if the cursor reaches 0 (start of file) and the total newline count is 0, return `(nil, nil)`.
- During reverse scan, ensure the slice end-point is the last `\n` byte (inclusive) in the file, not EOF. This guarantees any bytes after the last `\n` (the trailing partial) are excluded from the returned tail.
- For a file consisting of K terminated lines plus a trailing partial, return all K lines as `(bytes, nil)` — the trailing partial is dropped, the K lines are not affected by the partial's existence.

**Acceptance Criteria**:
- [ ] `Tail("/does/not/exist", 1000)` returns `(nil, nil)`, no error wrapping ENOENT.
- [ ] A zero-byte file returns `(nil, nil)`.
- [ ] A file containing only `partial line without newline` (no `\n`) returns `(nil, nil)`.
- [ ] A file containing `line1\nline2\npartial` returns `[]byte("line1\nline2\n")` and `nil` error — the partial is excluded.
- [ ] A file containing only `\n` (single empty terminated line) returns `[]byte("\n")` and `nil` — a terminated empty line is content.
- [ ] No path through the no-content branches returns a non-nil error.

**Tests**:
- `"it returns (nil, nil) for a missing file"` — call helper on a path that does not exist; assert nil bytes, nil error.
- `"it returns (nil, nil) for a zero-byte file"` — create empty file; assert nil bytes, nil error.
- `"it returns (nil, nil) for a file with only an unterminated partial line"` — write `"hello world"` (no `\n`); assert nil bytes, nil error.
- `"it excludes a trailing partial line from the returned tail"` — write `"line1\nline2\npartial"`; assert returned bytes equal `[]byte("line1\nline2\n")`, no `partial` substring.
- `"it preserves a single empty terminated line as content"` — write `"\n"`; assert returned bytes equal `[]byte("\n")`, nil error.
- `"it does not surface ENOENT as an error"` — assert error is literally `nil`, not an `fs.ErrNotExist`-wrapping value.

**Edge Cases**:
- ENOENT vs zero-byte must both produce `(nil, nil)` — they are observationally identical at the call site.
- File with only an unterminated partial line returns `(nil, nil)` — counts as zero terminated lines.
- File with terminated lines plus a trailing partial returns only the terminated lines.
- Single bare `\n` is one terminated line (not zero) and is returned as content.

**Context**:
> Spec § *Read Pipeline > Trailing-newline edge case*: "A file whose final bytes lack a trailing `\n` has those trailing bytes treated as a partial/in-progress record and excluded from the returned tail (the helper returns only fully-terminated records). A zero-byte file and a file containing only an unterminated partial line both render the placeholder under the zero-line outcome."
>
> Spec § *Architecture Summary > Test seams > ScrollbackReader > Return contract*: "`(nil, nil)` — 'no content available' — collapses ENOENT, zero-byte file, and zero-line result (file with only an unterminated partial line) into one shape. Caller renders the placeholder ('(no saved content)')."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *History Depth > Read Pipeline > Trailing-newline edge case*, § *Read-Failure Handling > Placeholder*, § *Architecture Summary > Test seams > ScrollbackReader > Return contract*.

## session-scrollback-preview-1-3 | approved

### Task 1-3: Tail-N helper OS-error shape

**Problem**: OS-level read errors (EACCES, EIO, etc.) must surface as `(nil, err)` so the call site in `internal/tui` renders the dedicated error string instead of the placeholder. Conflating these with the `(nil, nil)` no-content shape would silently hide legitimate failures behind the placeholder, making preview indistinguishable from "empty file" when the file is unreadable due to permissions or hardware faults.

**Solution**: Extend the helper from tasks 1-1 and 1-2 so any open error other than `fs.ErrNotExist`, and any read or seek error encountered mid-scan, propagates as `(nil, err)`. ENOENT must remain on the `(nil, nil)` branch from task 1-2 — this task explicitly does **not** reroute it.

**Outcome**: Permission-denied opens, mid-scan I/O failures, and seek failures all return `(nil, non-nil error)`. ENOENT continues to return `(nil, nil)`. The error wraps the underlying OS error so the caller can debug-log it without per-errno branching at the helper layer.

**Do**:
- In the helper, when `os.Open` returns an error that is **not** `errors.Is(err, fs.ErrNotExist)`, return `(nil, fmt.Errorf("tail scrollback %s: %w", path, err))` (or equivalent wrapping; the exact format is implementer choice but must use `%w` so `errors.Is` works on the returned error).
- During reverse scan, if any `Seek` or `Read` returns a non-nil error other than `io.EOF` at expected boundary, return `(nil, fmt.Errorf("tail scrollback %s: %w", path, err))`.
- Do **not** attempt retries; the spec mandates a single read attempt per focus event with no per-pane error cache (call-site retry on next focus change).
- Ensure the deferred `Close()` from task 1-1 still executes on the error path (use `defer` not explicit close).

**Acceptance Criteria**:
- [ ] An open error that is not ENOENT returns `(nil, non-nil err)`, with `errors.Is(err, originalOSError)` returning true via `%w` wrapping.
- [ ] A permission-denied file (mode 0000 or chown to another user) returns `(nil, non-nil err)`.
- [ ] A read error encountered mid-scan returns `(nil, non-nil err)` and does not leak the partial buffer into the result.
- [ ] ENOENT continues to return `(nil, nil)` — task 1-3 does not change task 1-2's behaviour.
- [ ] The file descriptor is closed even on the error path.

**Tests**:
- `"it returns an error for a permission-denied file"` — create file, `os.Chmod(path, 0o000)` (skip on Windows), `t.Cleanup(func() { os.Chmod(path, 0o600) })`; assert nil bytes, non-nil err. If the test is running as root (CI sometimes does), `t.Skip` with a clear reason since root bypasses permission checks.
- `"it wraps the underlying OS error so errors.Is works"` — assert `errors.Is(returnedErr, fs.ErrPermission)` (or equivalent) on the perm-denied case.
- `"it preserves the (nil, nil) shape for ENOENT — does not take the error branch"` — re-assert the no-content behaviour from task 1-2 to guard against regression in this task.
- `"it returns an error from a mid-scan read failure"` — inject a failing reader (e.g. via a test seam that wraps `os.Open` with a reader that returns an error after the first chunk). If no clean seam is available, this test may be omitted with a note; the perm-denied test alone covers the predominant OS-error branch.
- `"it closes the file descriptor on the error path"` — verify via the same instrumentation seam used in task 1-1 (if present) or via a `t.Cleanup` that asserts no fd leak via the OS — best-effort; omit if no clean seam exists.

**Edge Cases**:
- Permission-denied open returns error, not placeholder.
- Mid-scan read error returns error and does not leak partial buffer.
- ENOENT does **not** take the error branch — it remains on the `(nil, nil)` branch from task 1-2.
- Error wrapping uses `%w` so callers can `errors.Is` against `fs.ErrPermission` etc. without string matching.

**Context**:
> Spec § *Read-Failure Handling*: "OS-level read error (permissions, disk full, etc.). The viewport renders a brief error string in place of content. Should never occur given mode 0600 / same-user guarantees from the save daemon, but handled defensively rather than crashing the TUI."
>
> Spec § *Read-Failure Handling > Placeholder > Error string*: "OS-level read errors render a single short error string in the viewport rather than the placeholder. The wording is build-phase TBD; the same string is used for every error type (no per-errno differentiation, no EACCES vs EIO branching). Future focus changes onto the same pane retry the read fresh — there is no per-pane error cache."
>
> Spec § *Architecture Summary > Test seams > ScrollbackReader > Return contract*: "`(nil, err != nil)` — OS-level read failure (EACCES, EIO, etc.). Caller renders the error string."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Read-Failure Handling*, § *Architecture Summary > Test seams > ScrollbackReader > Return contract*.

## session-scrollback-preview-1-4 | approved

### Task 1-4: Tail-N performance benchmark

**Problem**: Preview's synchronous-in-`Update` decision rests on the tail-N read staying fast (p99 < 5ms on a 4 MB representative `.bin`). Without an automated benchmark guarding this budget, a future change to the helper (chunk size regression, accidental whole-file read, allocator-heavy refactor) could silently push read time into the 10s of milliseconds, making preview's keystroke handling perceptibly laggy. The spec explicitly requires a benchmark "the build phase ships … guarding this assertion. If a future change pushes p99 above the budget, the synchronous-read decision must be revisited."

**Solution**: Add a Go benchmark (`BenchmarkTailScrollback` or equivalent) under `internal/state` that generates a 4 MB fixture file (~50k lines of mixed-width content with ANSI escape sequences to mirror real `.bin` shape), resets the timer after fixture generation, and runs the tail-N helper with N=1000 in the measured region. The benchmark itself does not enforce the 5ms budget — Go's `testing.B` does not have built-in p99 assertions — but the build phase records the baseline numbers so regressions are visible in CI runs and the budget is auditable. A separate `_test.go` test that runs the helper on the same fixture and asserts wall-clock duration < 5ms (with a generous safety margin and `t.Skip` on slow CI runners) provides the regression guard.

**Outcome**: A benchmark exists and runs via `go test -bench=. ./internal/state/`. A regression-guard test exists that fails when the 4 MB tail-N read exceeds the budget under a representative load. The fixture-generation cost is excluded from the measured region.

**Do**:
- In the same package as the helper (likely `internal/state`), create `scrollback_tail_bench_test.go` (or merge into an existing tail-N test file).
- Write a benchmark function `BenchmarkTailScrollback(b *testing.B)` that:
  1. Builds a 4 MB fixture file in `b.TempDir()` containing ~50k lines, each ~80 characters wide, with synthetic ANSI escape sequences (e.g. `\x1b[31mred\x1b[0m`) at random intervals to mirror `tmux capture-pane -e` output shape.
  2. Calls `b.ResetTimer()` after fixture generation so setup cost is excluded.
  3. In the `for i := 0; i < b.N; i++` loop, calls the tail-N helper with N=1000 against the fixture path and consumes the result with `_ = bytes` to prevent dead-code elimination.
- Write a regression-guard test `TestTailScrollback_PerformanceBudget(t *testing.T)` that:
  1. Generates the same 4 MB fixture in `t.TempDir()`.
  2. Calls the helper once to warm any page cache.
  3. Times a second call with `time.Now()` / `time.Since`.
  4. Asserts the elapsed time is < 5 ms.
  5. Includes a `t.Skip("performance test skipped: $PORTAL_SKIP_PERF=1")` guard or similar so the test can be opted out on known-slow CI runners.
- Document the budget in a comment on both the benchmark and the regression-guard test, citing the spec section.

**Acceptance Criteria**:
- [ ] `go test -bench=BenchmarkTailScrollback ./internal/state/` runs without error and reports per-op times.
- [ ] `b.ResetTimer()` is invoked after fixture generation; the reported per-op time reflects only the helper's work, not the fixture build.
- [ ] `TestTailScrollback_PerformanceBudget` passes on a developer machine with `time.Since(start) < 5*time.Millisecond` for a 4 MB fixture after warmup.
- [ ] The regression-guard test skips cleanly when `PORTAL_SKIP_PERF` (or equivalent env opt-out) is set, so flaky CI hardware does not block merges.
- [ ] Fixture content includes ANSI escape sequences and varied line widths so the benchmark is representative of real `tmux capture-pane -e` output.

**Tests**:
- `"BenchmarkTailScrollback runs and reports a per-op time"` — meta-assertion: a smoke-level test (or just `go test -bench`) that invokes the benchmark and asserts it completes without panicking.
- `"it reads tail-N from a 4 MB fixture in under 5ms after warmup"` — the regression-guard test described above.
- `"the benchmark excludes fixture generation from the measured region"` — verify by code inspection: `b.ResetTimer()` appears after the fixture-build call. Add a comment-level test note rather than a runtime assertion.

**Edge Cases**:
- Fixture generation cost must be excluded — `b.ResetTimer()` after build, not before.
- The regression-guard test runs the helper twice (once to warm, once to measure) so disk-cache warmth is a controlled variable, not a source of flake.
- The 5ms budget is for warmed-cache reads (the realistic case during preview cycling); cold-cache reads may be slower and are out of scope for the budget.

**Context**:
> Spec § *History Depth > Read Pipeline > Performance budget*: "The synchronous-in-`Update` decision rests on the read staying fast. Pinned target: tail-N read p99 < 5ms on a 4 MB `.bin` file (representative of a busy-session worst case). The build phase ships a benchmark guarding this assertion. If a future change pushes p99 above the budget, the synchronous-read decision must be revisited (likely by deferring the read via `tea.Cmd`); the budget is the audit threshold, not a soft aspiration."
>
> Spec § *History Depth*: "Per-pane `.bin` files on disk can be large (~3.7MB / 50k+ lines for busy sessions); preview never feeds the full file into the renderer."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *History Depth > Read Pipeline > Performance budget*.

## session-scrollback-preview-1-5 | approved

### Task 1-5: Window-grouped pane enumeration on tmux.Client

**Problem**: Preview's chrome (window M of N, pane X of Y, window name) and structural cycling (`]`, `[`, `Tab`) need a single call that returns all windows-with-panes for a session, ordered by `window_index` then `pane_index`, with each window's name attached. No existing `tmux.Client` method composes this — `ListPanesInSession` returns flat panes without window-name correlation, and there is no list-windows wrapper. Composing two calls (option (b) in the spec) is mechanically valid but spreads the call across two error sites; the spec prefers option (a): a single new read-only method on `tmux.Client`.

**Solution**: Add a new method to `tmux.Client` (e.g. `ListWindowsAndPanesInSession(session string) ([]WindowGroup, error)`) that runs `tmux list-panes -t <session> -s -F "#{window_index}|#{window_name}|#{pane_index}"` (the `-s` flag scopes to the session and emits panes across all its windows in `window_index, pane_index` order), parses the pipe-delimited output, and groups results by `window_index` into a `[]WindowGroup` shape that exposes both per-window name and the ordered list of pane indices. Tests inject a mock `Commander` to return canned stdout fixtures — no real tmux server required.

**Outcome**: A new `ListWindowsAndPanesInSession(session)` method exists on `tmux.Client` and is callable from any TUI adapter. The returned `[]WindowGroup` is sorted by `window_index` ascending, with each group's panes sorted by `pane_index` ascending. The window name is captured per group. Happy-path scope: tmux call succeeds and returns ≥1 windows with ≥1 panes each. Failure modes and empty-result handling are tightened by task 1-6.

**Do**:
- Define a `WindowGroup` struct in `internal/tmux/` (or wherever existing structural enumeration types live — match the package's existing conventions). Suggested shape:
  ```go
  type WindowGroup struct {
      WindowIndex int    // raw tmux window_index (e.g. 0, 2, 5 under non-contiguous gaps)
      WindowName  string // tmux #{window_name}
      PaneIndices []int  // raw tmux pane_index values, sorted ascending
  }
  ```
  The exact field set is at the implementer's discretion; what matters is that callers can read `WindowName`, the count of windows in the slice (for chrome's "Window M of N"), the count of panes per window (for "Pane X of Y"), and the raw index values needed to compute pane keys via `state.SanitizePaneKey`.
- Add the method to `tmux.Client` (in the appropriate existing file under `internal/tmux/`):
  ```go
  func (c *Client) ListWindowsAndPanesInSession(session string) ([]WindowGroup, error)
  ```
- Use `c.cmd.Run(...)` (the trim variant) — pipe-delimited output does not need verbatim preservation, and trimming the trailing newline is desirable. Match the pattern used by `ListPanesInSession`.
- Build the tmux command: `tmux list-panes -s -t <session> -F "#{window_index}|#{window_name}|#{pane_index}"`. The `-s` flag scopes to the session and emits all panes across all its windows.
- Parse the output line-by-line; for each line, split on `|` with exactly 2 splits expected (3 fields: window_index, window_name, pane_index). If a window name contains a literal `|`, this naive split corrupts it — see edge-case handling below.
- Group lines by `window_index` (preserving first-seen window name per group), build the `[]WindowGroup` slice sorted by `window_index` ascending, and within each group sort `PaneIndices` ascending.
- Handle the pipe-in-window-name edge case: tmux allows `|` in window names. The implementer should choose either (i) use a non-printable separator like `\x1f` (US — unit separator) which cannot appear in tmux names in practice, or (ii) parse with `strings.SplitN(line, "|", 3)` and accept that a `|` in the **first or second** position would break parsing while a `|` later in the name is harmless because the third field (pane_index) is numeric. Option (i) is preferred for robustness; document the choice in the method's comment.
- Tests use the existing `Commander` mock pattern (other tests in `internal/tmux/` show this — inject a fake `Commander` that returns canned stdout for the expected command).

**Acceptance Criteria**:
- [ ] `ListWindowsAndPanesInSession(session)` returns `([]WindowGroup, error)` and is exported from `internal/tmux`.
- [ ] On a session with windows `[0, 1, 2]`, each having panes `[0, 1]`, the call returns 3 `WindowGroup`s in order, each with two pane indices in order.
- [ ] On a session with non-contiguous `window_index` values (e.g. `[0, 2, 5]`), the returned slice has length 3 and `WindowIndex` fields are `[0, 2, 5]` in order. The slice **does not** pad gaps — preview's chrome computes 1-based ordinal positions from slice length, not from raw indices.
- [ ] Under `set -g base-index 1` and `set -g pane-base-index 1` (raw indices start at 1), the returned slice still preserves the raw indices verbatim — preview's chrome layer handles the 1-based ordinal mapping, not this method.
- [ ] Window names containing whitespace (`my window`) are returned intact.
- [ ] Window names containing the chosen delimiter character require careful handling; the chosen approach (non-printable separator OR `SplitN(_, _, 3)`) is documented in the method comment and exercised by a test fixture.
- [ ] Multiple panes per window are grouped into the correct `WindowGroup` and sorted ascending by `pane_index`.
- [ ] The method uses `c.cmd.Run` (or equivalent existing trim wrapper) — no direct `os/exec.Command` call.
- [ ] No changes to `tmux.Client.CapturePane` or any other existing capture wrapper. The diff to `internal/tmux/` is purely additive.

**Tests**:
- `"it returns window-grouped panes ordered by window_index then pane_index"` — Commander mock returns `"0|main|0\n0|main|1\n1|logs|0\n"`; assert two groups: (0, "main", [0,1]), (1, "logs", [0]).
- `"it preserves non-contiguous window_index values verbatim"` — mock returns `"0|a|0\n2|b|0\n5|c|0\n"`; assert returned `WindowIndex` values are `[0, 2, 5]` and slice length is 3 (no gap padding).
- `"it preserves base-index 1 raw values"` — mock returns `"1|x|1\n1|x|2\n2|y|1\n"`; assert window indices `[1, 2]` and pane indices `[1, 2]` and `[1]` respectively.
- `"it preserves window names containing whitespace"` — mock returns `"0|my window|0\n"`; assert `WindowName == "my window"`.
- `"it preserves window names containing the pipe delimiter"` — mock returns a line with a `|`-bearing window name (using the chosen separator strategy); assert `WindowName` equals the original. If the implementer chose `\x1f`, the fixture uses `\x1f` between fields and the test confirms `|` in the name passes through. If the implementer chose `SplitN(_,_,3)`, the test confirms the documented limitation (e.g. `|` early in the name would break) is at least documented and a `|` mid-name passes through.
- `"it groups multiple panes within the same window correctly"` — mock returns four lines for one window with pane_index `[0, 1, 2, 3]`; assert single group with all four pane indices ascending.
- `"it uses the Commander.Run interface"` — assert via mock that `Run` (not `RunRaw`) is called and the command vector includes `list-panes -s -t <session> -F ...`.

**Edge Cases**:
- Non-contiguous `window_index` (e.g. `0, 2, 5`) returns slice without padding gaps.
- Base-index 1 / pane-base-index 1 returns raw values verbatim (preview maps to 1-based ordinals at the chrome layer).
- Window names containing `|` require either a non-printable separator or documented `SplitN` parsing; pick one and document.
- Window names containing whitespace pass through unchanged.
- Multiple panes per window are grouped and sorted.

**Context**:
> Spec § *Multi-pane Rendering Shape > Concrete enumeration call*: "No existing `tmux.Client` method returns window-grouped panes plus window names for a single session in one call. The build phase therefore composes the enumeration from one of: (a) Add a new `tmux.Client` method (e.g. `ListWindowsAndPanesInSession(session) ([]WindowGroup, error)`) that runs `tmux list-panes -t <session> -F "#{window_index}|#{window_name}|#{pane_index}"` and groups results by `window_index`. Preferred — keeps the call cohesive in `internal/tmux`."
>
> Spec § *Multi-pane Rendering Shape > Chrome Floor > Counter semantics*: "`M` and `X` in 'Window M of N' / 'Pane X of Y' are 1-based ordinal positions in enumeration order, not the tmux `window_index` / `pane_index` values."
>
> Spec § *Architecture Summary*: "No new methods on `tmux.Client`. … The 'no new tmux wrapper' rationale … applies specifically to **capture** wrappers (i.e. avoiding `CapturePaneTail`); a new listing method is a different category and does not contradict it."
>
> Existing pattern: `internal/tmux/` already exposes `ListPanesInSession`, `ListAllPanesWithFormat`, etc. — match the existing parsing and Commander-injection conventions.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Multi-pane Rendering Shape > Concrete enumeration call*, § *Multi-pane Rendering Shape > Chrome Floor*, § *Architecture Summary*.

## session-scrollback-preview-1-6 | approved

### Task 1-6: Enumeration failure and empty-result handling

**Problem**: Preview's open path treats two outcomes identically — "tmux call failed" and "tmux call succeeded but returned zero windows or zero panes" — by silently aborting the open and leaving the user on the Sessions list. The new `ListWindowsAndPanesInSession` method must surface both shapes correctly: a non-nil error for tmux non-zero exit (e.g. session disappeared mid-call), and an empty `[]WindowGroup` slice (with nil error) for empty stdout. Conflating them at the `tmux.Client` layer would force preview to second-guess errors versus content shape.

**Solution**: Pin the failure and empty-result contract on the method from task 1-5: any non-zero exit from `tmux list-panes` is wrapped and returned as a non-nil error; empty stdout (zero lines) returns an empty `[]WindowGroup{}` slice with nil error. The call site in `internal/tui` will treat both as "do not open preview" per the spec, but the method itself preserves the distinction so callers (current and future) can differentiate.

**Outcome**: When tmux returns an error (session gone, server crash, etc.), the method returns `(nil, non-nil err)`. When tmux returns successfully but stdout is empty, the method returns `([]WindowGroup{}, nil)` — a usable empty slice, not nil. No new capture wrapper is introduced (the spec's "no new tmux wrapper" constraint applies to capture, not listing — already discharged in task 1-5; this task re-asserts the diff stays additive on the listing side only).

**Do**:
- In the method from task 1-5, when `c.cmd.Run` returns a non-nil error, wrap it with the session name for traceability: `return nil, fmt.Errorf("list windows and panes for session %s: %w", session, err)`. Use `%w` so callers can `errors.Is` against any sentinel errors `Commander.Run` returns.
- When `c.cmd.Run` returns `(nil, nil)` (i.e. successful but empty stdout — e.g. zero panes), return `([]WindowGroup{}, nil)` explicitly, not `(nil, nil)`. The empty-but-non-nil slice signals "session exists but has no panes/windows" cleanly to callers.
- When stdout has only whitespace (e.g. trailing `\n` from tmux), parse it as zero lines and return `([]WindowGroup{}, nil)` — same as the strictly-empty case.
- Add a code-comment assertion (or a CI-time grep test, if the project has any) that the diff to `internal/tmux/` adds **only** the new `ListWindowsAndPanesInSession` method and `WindowGroup` type — no changes to `CapturePane`, `CapturePane*` variants, or any other existing method. This is a guard against scope creep into capture wrappers, which the spec explicitly forbids.

**Acceptance Criteria**:
- [ ] When the underlying `Commander.Run` returns an error, the method returns `(nil, non-nil err)` and the error wraps the original via `%w`.
- [ ] When `Commander.Run` returns `("", nil)` (empty stdout, success), the method returns `([]WindowGroup{}, nil)` — empty but non-nil slice, nil error.
- [ ] When `Commander.Run` returns `("\n", nil)` or only-whitespace, the method returns `([]WindowGroup{}, nil)`.
- [ ] The error message includes the session name for debuggability.
- [ ] No `tmux.Client.CapturePane*` method signature is modified by this task or task 1-5; verifiable via `git diff` review on `internal/tmux/` against pre-phase baseline — the only additions are the new method, the new type, and any test files.
- [ ] Callers can use `errors.Is(err, anySentinel)` against the returned error if `Commander.Run` exposes sentinels.

**Tests**:
- `"it returns an error when tmux exits non-zero"` — Commander mock returns `("", errors.New("exit status 1: no such session"))`; assert returned bytes-slice is nil, error is non-nil and wraps the underlying error.
- `"it returns an empty slice when stdout is empty and exit is zero"` — Commander mock returns `("", nil)`; assert returned slice is non-nil, length 0, error is nil.
- `"it returns an empty slice for whitespace-only stdout"` — Commander mock returns `("\n", nil)`; assert returned slice is non-nil, length 0, error is nil.
- `"the wrapped error includes the session name"` — assert `strings.Contains(err.Error(), sessionName)`.
- `"the wrapped error preserves the original via errors.Is"` — define a sentinel `var errSentinel = errors.New("sentinel")`; have the mock return it; assert `errors.Is(returnedErr, errSentinel)` is true.
- `"the diff to internal/tmux/ adds no capture wrappers"` — code review-level check; document in the test file as a comment referencing the spec § *Source of Preview Bytes > Single read path consequences > No new tmux capture wrapper*. Optional: add a literal `grep`-style test that imports the package and asserts a known set of `CapturePane*` method names is unchanged. Practical level: a comment in the test file pointing to the constraint is sufficient; the human reviewer enforces it.

**Edge Cases**:
- Tmux exit non-zero (session disappeared between `Space` and the call) → `(nil, err)`.
- Empty stdout vs error must be distinguishable: empty stdout → `([]WindowGroup{}, nil)`, error → `(nil, err)`.
- Whitespace-only stdout treated as empty (zero lines).
- No new capture wrapper introduced — the additive diff stays on the listing side only.

**Context**:
> Spec § *Multi-pane Rendering Shape > Chrome Floor > Enumeration failure handling*: "If the enumeration call itself fails at preview-open (e.g. session disappeared between `Space` press and the call), preview returns to the Sessions list silently — no preview page is shown. The Sessions list re-fetches on return per § *Cross-cutting Seams > Externally-Killed Session During Preview*."
>
> Spec § *Refresh Semantics > Read Trigger Events > Initial-open ordering*: "Structural enumeration call runs first (synchronous; see § *Multi-pane Rendering Shape > Concrete enumeration call*). If enumeration fails → return to Sessions list silently; no preview page is shown. If enumeration succeeds but returns an empty result (zero windows, or a window with zero panes — e.g. session is being torn down between `Space` and the call) → treated identically to enumeration failure: return to Sessions list silently; no preview page is shown."
>
> Spec § *Source of Preview Bytes > Single read path consequences*: "No new tmux capture wrapper. The existing `tmux.Client.CapturePane` hardcodes `-S -` (full scrollback) and is shared with save-daemon semantics; a bounded variant (e.g. `CapturePaneTail(target, n)`) would have been net-new code. Always-disk avoids that addition entirely."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Multi-pane Rendering Shape > Chrome Floor > Enumeration failure handling*, § *Refresh Semantics > Read Trigger Events > Initial-open ordering*, § *Source of Preview Bytes*, § *Architecture Summary*.
