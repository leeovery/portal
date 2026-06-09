---
phase: 1
phase_name: Migrate status reader to slog text format
total: 4
---

## status-recent-warnings-pipe-format-1-1 | approved

### Task 1: Add ParseLogLine helper to internal/log

**Problem**: The writer (`internal/log`) emits the slog text format `<RFC3339Nano> <LEVEL> <component>: <msg> <attrs k=v…> pid=… version=… process_role=…`, but no exported inverse exists, so the status reader in `internal/state/status.go` re-derives the format independently (and incorrectly, against the legacy pipe layout). Defining the parse in one place, alongside the writer, is what removes the "writer and reader each define the format independently" defect that caused the drift.

**Solution**: Add a new exported `ParseLogLine` helper in a new file `internal/log/parse.go` that is the single inverse of `textHandler.Handle`'s line format, returning a `LogLine` struct (Time, Level, Component, Message) and an `ok` boolean. It lives in the writer's package so any future writer-format change forces the parser to change too.

**Outcome**: `log.ParseLogLine(line) (LogLine, bool)` correctly extracts Time/Level/Component/Message from a real writer line, drops contextual attrs and the pid/version/process_role baselines from the Message, and returns `ok=false` for any line that does not match the layout — verified by direct unit tests against lines produced by (or matching) the real writer.

**Do**:
- Create `/Users/leeovery/Code/portal/internal/log/parse.go` in package `log`.
- Define the exported `LogLine` struct exactly per the spec contract:
  - `Time time.Time` — parsed from the first whitespace-delimited token with `time.RFC3339Nano`.
  - `Level string` — the second whitespace-delimited token, verbatim (`"DEBUG"|"INFO"|"WARN"|"ERROR"`).
  - `Component string` — text between the level token and the first `:` in the line, surrounding whitespace trimmed; `""` if absent.
  - `Message string` — human message only; contextual attrs and the pid/version/process_role baselines excluded.
- Implement `func ParseLogLine(line string) (parsed LogLine, ok bool)` following these rules:
  - **Time**: take the first whitespace-delimited token; `time.Parse(time.RFC3339Nano, token)`. On error → return `ok=false`.
  - **Level**: take the second whitespace-delimited token verbatim. If there are fewer than two whitespace-delimited tokens → `ok=false`.
  - **Component**: find the first `:` in the line; the component is the substring between the end of the level token and that `:`, `strings.TrimSpace`'d. If the line contains no `:` → `ok=false`. An all-whitespace or empty run yields `Component == ""` with `ok == true`.
  - **Message**: the text after `<component>: ` (the first `:` then a single space the writer emits), up to but excluding the first whitespace-delimited token matching `^[A-Za-z_][A-Za-z0-9_.]*=` (the attr-key boundary). Trim trailing whitespace introduced by the boundary split. This single boundary rule drops both contextual attrs and the trailing baselines in one pass. Compile the boundary regex once at package level (e.g. `var attrKeyToken = regexp.MustCompile(...)`).
  - A message containing later `:` characters keeps them — only the *first* `:` delimits the component.
  - An empty message (component colon-space immediately followed by the first attr token) yields `Message == ""` with `ok == true`.
- Add a doc comment on `ParseLogLine` stating it is the single inverse of the writer's line format and listing the three `ok=false` triggers (no `:`, fewer than two tokens, unparseable timestamp).
- Do not modify `handler.go` in this task.

**Acceptance Criteria**:
- [ ] `ParseLogLine` is exported from package `log` and returns `(LogLine, bool)`.
- [ ] A well-formed writer line parses to the correct Time, Level, Component, and Message.
- [ ] Component has its trailing `:` and surrounding whitespace removed; an empty-component line yields `Component == ""` with `ok == true`.
- [ ] The Message boundary drops both contextual attrs and the `pid`/`version`/`process_role` baselines in one pass.
- [ ] A message with no attrs is preserved whole (no truncation).
- [ ] A message containing later `:` characters retains them.
- [ ] A quoted multi-word attr value (e.g. `version="3.6 beta"`) does not shift the boundary earlier than the first real attr key.
- [ ] `ok == false` for: a line with no `:`, a line with fewer than two whitespace-delimited tokens, an unparseable first token, and the empty string.
- [ ] Time parses for both whole-second and fractional-second (RFC3339Nano) timestamps.

**Tests** (new `internal/log/parse_test.go`):
- `"it parses a well-formed line into Time, Level, Component, and Message"`
- `"it strips contextual attrs and pid/version/process_role baselines from Message"`
- `"it preserves a message with no attrs whole"`
- `"it preserves later colons in the message (component is delimited by the first colon only)"`
- `"it does not shift the boundary when a quoted multi-word attr value contains key=value-shaped text"`
- `"it returns Component empty and ok true for an empty-component line"`
- `"it returns Message empty and ok true for an empty message"`
- `"it returns ok false for an unparseable timestamp"`
- `"it returns ok false for a line with no colon"`
- `"it returns ok false for a line with fewer than two whitespace tokens"`
- `"it returns ok false for the empty line"`
- `"it parses both whole-second and fractional-second timestamps"`

**Edge Cases**:
- Unparseable timestamp → `ok=false`.
- No colon → `ok=false`.
- Fewer than two whitespace tokens → `ok=false`.
- Empty line (`""`) → `ok=false` (covered by no-tokens / no-colon).
- Empty component → `Component == ""`, `ok == true`.
- Empty message → `Message == ""`, `ok == true`.
- Message with later colons preserved.
- Boundary regex drops contextual attrs and the pid/version/process_role baselines in one pass.
- Quoted multi-word attr value does not shift the boundary (the boundary keys off the first matching token; a genuine attr key always precedes any value content).
- No-attrs message preserved whole.

**Context**:
> Spec §1 "Shared parse helper in `internal/log`" defines the `LogLine`/`ParseLogLine` contract verbatim, including the boundary regex `^[A-Za-z_][A-Za-z0-9_.]*=` and the three `ok=false` triggers. The writer line format is built in `internal/log/handler.go` `textHandler.Handle` (lines ~147-179): time via `r.Time.Format(time.RFC3339Nano)`, then `' '`, level via `r.Level.String()`, then `' '`, then the component, then `": "`, then the message, then contextual attrs (each ` key=value`), then ` pid=… version=… version=… process_role=…`. The component is rendered as a literal prefix before the colon and never as a key=value pair, so the first `:` reliably ends the component (component names carry no spaces or colons). The "documented assumption" (spec §1) is that log messages contain no `key=value`-shaped token; if violated, the only effect is the displayed `LastWarning` summary truncating early — it never affects the count or the health signal.

**Spec Reference**: `.workflows/status-recent-warnings-pipe-format/specification/status-recent-warnings-pipe-format/specification.md` (§1 Shared parse helper)

## status-recent-warnings-pipe-format-1-2 | approved

### Task 2: Factor a producer-coupled render seam out of textHandler.Handle

**Problem**: The producer-coupled regression test (Task 3) and the migrated cmd-layer tests (Task 4) need to obtain *real-writer* `portal.log` output, but the real handler (`textHandler`) is unexported and `log.Init` mutates the process-wide handler and writes through a rotating sink + symlink. Without an explicit seam, those tests would have to re-implement the format — exactly the parallel-definition mistake the fix is closing.

**Solution**: Factor the line-building logic out of `textHandler.Handle` into a shared render path, then expose a test-only seam in `internal/log` that renders a single record to its canonical `portal.log` text line through the **same** rendering path `Handle` uses in production. The seam is `*testing.T`-first (test-only, like `SetTestHandler`) and performs no process-global handler mutation and no sink I/O.

**Outcome**: A `*testing.T`-first helper in `internal/log` returns the exact bytes `textHandler.Handle` would write for a given record (level, component, message, attrs) including the pid/version/process_role baselines and trailing newline, so downstream tests source fixtures from the single production format source rather than a parallel definition.

**Do**:
- In `/Users/leeovery/Code/portal/internal/log/handler.go`, extract the line-building portion of `textHandler.Handle` (the `strings.Builder` sequence at lines ~132, ~147-181, ending with the trailing `'\n'`) into a reusable method/function on `textHandler` (e.g. `func (h *textHandler) render(r slog.Record) string`). `Handle` then calls `render` and passes the result to `bestEffortWrite`, leaving the level-filter/bypass gate (lines ~142-145) and the best-effort write behaviour exactly as they are. The extraction must be behaviour-preserving — production output stays byte-identical.
- Add a test-only render seam. Prefer adding it to a new test-support file (e.g. `/Users/leeovery/Code/portal/internal/log/rendertest.go`) so the `*testing.T` import marks it test-only at the package boundary, mirroring `testhandler.go`'s `SetTestHandler`. Signature, `*testing.T`-first:
  - `func RenderLineForTest(t *testing.T, level slog.Level, component, message string, attrs ...slog.Attr) string`
  - It constructs a `textHandler` via the production `newTextHandler` constructor (choose representative baselines — e.g. `os.Getpid()`, a version string, a process role — or pin documented fixed test baselines), builds a `slog.Record` with the given time/level/message and attrs (component supplied as the `component` attr via `r.AddAttrs(slog.String("component", component))` or through the constructor's accumulated attrs so the existing component-resolution path renders it as the literal prefix), and returns `h.render(record)` — the same path Handle uses.
  - It must NOT call `setHandler` / `Init` / `SetTestHandler` (no process-global handler mutation) and must NOT perform any sink write.
  - Allow the caller to supply the record time (add a time parameter, or document that the helper stamps a caller-chosen time) so Task 3/4 can place lines inside or outside the one-hour window. If a single fixed signature is cleaner, accept a `time.Time` as the first value parameter after `t`. The caller-supplied time is rendered through the same `r.Time.Format(time.RFC3339Nano)` path `Handle` uses, so the fixture's timestamp is `RFC3339Nano` — exactly what `ParseLogLine` (Task 1) expects. Tests must always source the line from this seam rather than hand-formatting a timestamp, so the format cannot drift back to the legacy `time.RFC3339`.
- Add a doc comment stating the helper renders through the real handler's production render path, is the only sanctioned way to obtain real-writer output in tests, and is `*testing.T`-first to structurally mark it test-only.

**Acceptance Criteria**:
- [ ] `textHandler.Handle` produces byte-identical output before and after the extraction (existing handler tests still pass; production format unchanged including baselines and trailing newline).
- [ ] The seam renders a record via the same path `Handle` uses — not a re-implementation of the format.
- [ ] The rendered line includes the `component:` prefix, contextual attrs, the `pid`/`version`/`process_role` baselines, and a trailing `'\n'`.
- [ ] The seam performs no process-global handler mutation (does not call `setHandler`/`Init`/`SetTestHandler`) and no sink write.
- [ ] The seam takes `*testing.T` as its first parameter, so it cannot be referenced from non-test code.

**Tests** (new `internal/log/rendertest_test.go` or fold into `handler` tests):
- `"it renders a record byte-identical to textHandler.Handle output including baselines and trailing newline"` — drive a record through both `Handle` (capturing the sink via the test writer/`stderrFallback` seam or a constructed `textHandler` with a buffer `w`) and the new seam, and assert equality.
- `"it does not mutate the process-global handler"` — capture `currentHandler()` before/after and assert identity unchanged.
- `"the seam parameter ordering is testing.T-first"` — compile-time guaranteed; assert via a smoke call.

**Edge Cases**:
- Byte-identical to `Handle` output including baselines and trailing newline.
- No process-global handler mutation.
- `*testing.T`-first test-only marker (structurally prevents non-test import).

**Context**:
> Spec "Testing requirements → Producer-coupled test seam": the real text handler is unexported and `log.Init` mutates the process-wide handler and writes through the rotating sink + `portal.log` symlink, so the producer-coupled test needs an explicit seam that renders a single record to its canonical `portal.log` text line via the **same rendering path `textHandler.Handle` uses in production** (factor the line-building out of `Handle` into a render function, or a test-only render helper — `*testing.T`-first, like `SetTestHandler` — that drives the real handler), never a re-implementation of the format. This keeps fixtures byte-identical to production output without leaking process-global handler state. The `*testing.T`-first marker mirrors `SetTestHandler` (`internal/log/testhandler.go`) and `portaltest.IsolateStateForTest`. The level-filter/bypass gate in `Handle` (handler.go ~142-145) is intentionally left out of the extracted render path — the seam renders a line regardless of level so a WARN fixture is always produced.

**Spec Reference**: `.workflows/status-recent-warnings-pipe-format/specification/status-recent-warnings-pipe-format/specification.md` (Testing requirements → Producer-coupled test seam)

## status-recent-warnings-pipe-format-1-3 | approved

### Task 3: Migrate status reader to ParseLogLine

**Problem**: `internal/state/status.go`'s `logEntryQualifies` splits each `portal.log` line on the legacy `" | "` separator expecting 4 fields; the writer now emits slog text with no `" | "`, so every line yields `len < 4` and the reader returns `false` unconditionally — `RecentWarnings` is always 0 and `LastWarning` always `""`, silently masking real warnings.

**Solution**: Migrate the reader to parse each line once via `log.ParseLogLine`, count a line when `ok && (Level == "WARN" || Level == "ERROR") && !Time.Before(cutoff)`, and store `LastWarning` as the composed `<LEVEL> <component>: <msg>` summary (last-wins). Remove the pipe-format constants and refresh the doc comments so nothing references the old format.

**Outcome**: Given a `portal.log` written by the real `internal/log` writer, `CollectStatus` reports the actual WARN/ERROR count within the last hour and a trimmed `LastWarning` summary; a producer-coupled regression test (sourced from the Task 2 render seam) fails red if the writer's line format ever drifts again.

**Do**:
- In `/Users/leeovery/Code/portal/internal/state/status.go`:
  - Add the import for the log package (`github.com/leeovery/portal/internal/log`). Confirm `log` does not import `state` (it does not — coupling is legal and cycle-free per spec §Overview).
  - **Remove** the `logFieldSeparator = " | "` (line ~19) and `expectedLogFieldCount = 4` (line ~23) constants and their doc comments.
  - Rewrite `scanRecentWarnings` (lines ~186-202) so each line is parsed once via `log.ParseLogLine`. A line qualifies when `ok == true` **and** `Level` is `WARN` or `ERROR` **and** `!Time.Before(cutoff)`. On a qualifying line: `rep.RecentWarnings++` and `rep.LastWarning = composeLastWarning(parsed)` (last-wins, positional top-to-bottom). Non-qualifying / unparseable lines are silently skipped (swallow-and-skip contract preserved).
  - Add the `LastWarning` composition (a small helper or inline): when `Component != ""` render `"<LEVEL> <component>: <msg>"`; when `Component == ""` render `"<LEVEL>: <msg>"` (no stray space before the colon). An empty `Message` renders as `"<LEVEL> <component>:"` / `"<LEVEL>:"` with no trailing space.
  - Fold `logEntryQualifies` into the parse-once flow or rewrite it to operate on the parsed `LogLine` (function boundary is the implementer's choice) — but its body and doc comment must no longer reference `logFieldSeparator` / `expectedLogFieldCount` / "wrong field count".
  - Replace the `StatusReport.LastWarning` doc comment (status.go:65-66), currently:
    ```go
    // LastWarning is the full text of the most recent qualifying WARN/ERROR
    // log line (last-wins). Empty when there are no qualifying entries.
    ```
    with:
    ```go
    // LastWarning is the most recent qualifying entry rendered as
    // `<LEVEL> <component>: <msg>` — timestamp prefix and trailing
    // attrs/baselines omitted. Empty when there are no qualifying entries.
    ```
  - Refresh `scanRecentWarnings`'s doc comment (lines ~181-185) to drop the pipe-format concepts ("wrong field count"); describe it as parsing each line via `log.ParseLogLine` and counting WARN/ERROR entries at or after cutoff.
- In `/Users/leeovery/Code/portal/internal/state/status_test.go`:
  - **Remove** the `writeLogLine` helper (lines ~13-29) — it constructs the pipe format from an independent format string and must not survive.
  - Add a `*testing.T`-first fixture helper that writes a real-writer line to `portal.log`, sourcing the rendered bytes from the Task 2 seam (`log.RenderLineForTest`). The helper appends the rendered line(s) to `state.PortalLog(dir)` (a plain file matching `PortalLog(dir)`; no rotation/symlink needed) in append (chronological) order so "last in file" == "most-recent timestamp". The seam output already ends with the trailing `'\n'` (Task 2 contract), so the helper appends it verbatim — do **not** add a second newline.
  - Re-express the retained cases in the new format using that helper: window-cutoff (inside vs outside the last hour — `TestCollectStatus_SkipsEntriesOlderThanCutoff`, `TestCollectStatus_UsesCallerSuppliedNowForWindow`), level-filter (`TestCollectStatus_IgnoresInfoAndDebugEntries`, `TestCollectStatus_CountsWarnAndErrorEntriesInWindow` — both WARN and ERROR counted, INFO/DEBUG excluded), last-wins (`TestCollectStatus_LastWarningHoldsLastValidEntry`), missing-`portal.log` → zero (`TestCollectStatus_RecentWarningsZeroWhenLogMissing`), `portal.log.old` ignored (`TestCollectStatus_DoesNotScanPortalLogOld`).
  - Re-express the malformed-line case (`TestCollectStatus_ToleratesMalformedLogEntries`, lines ~346-384) as lines that do **not** match the new layout (e.g. a line with no colon, a line with fewer than two whitespace tokens, a line whose first token is not an RFC3339Nano timestamp) plus one valid WARN sourced from the seam — asserting the malformed lines are skipped and the scan continues.
  - Add a `LastWarning`-composition assertion: a WARN with a non-empty component renders `WARN <component>: <msg>`; an empty-component WARN renders `WARN: <msg>` (no stray space before colon); an empty-message WARN renders `WARN <component>:` (no trailing space).
  - Add at least one producer-coupled end-to-end regression test at the `CollectStatus` layer: drive the real `internal/log` writer (via the seam) to emit a WARN line into the status dir's `portal.log`, run `CollectStatus`, and assert `RecentWarnings >= 1` and the trimmed `LastWarning`. This is the guard that turns red on writer-format drift.

**Acceptance Criteria**:
- [ ] WARN and ERROR lines within the window are counted; INFO and DEBUG are not.
- [ ] A qualifying-level line older than the one-hour window is not counted; one inside is.
- [ ] Missing `portal.log` → `RecentWarnings == 0`, `LastWarning == ""`, no error.
- [ ] A malformed / non-matching line is silently skipped without aborting the scan of remaining lines.
- [ ] `LastWarning` holds the composed summary of the last qualifying line in append order (last-wins, positional).
- [ ] `LastWarning` renders `<LEVEL> <component>: <msg>` when Component is present, `<LEVEL>: <msg>` when Component is empty (no stray space before colon).
- [ ] An empty Message renders with no trailing space (`<LEVEL> <component>:` / `<LEVEL>:`).
- [ ] `logFieldSeparator` and `expectedLogFieldCount` constants are removed from `status.go`.
- [ ] No doc comment or constant in `status.go` references the pipe format (`" | "`, "field count", "field separator").
- [ ] The `writeLogLine` helper is removed from `status_test.go`; no test constructs a log line from an independently-defined format string.
- [ ] `portal.log.old` is ignored (only `portal.log` is scanned).
- [ ] A producer-coupled `CollectStatus`-layer regression test sources its WARN fixture from the real writer via the Task 2 seam.

**Tests** (`internal/state/status_test.go`, migrated + new):
- `"it counts WARN and ERROR entries within the window"`
- `"it excludes INFO and DEBUG entries"`
- `"it excludes entries older than the one-hour cutoff and includes ones inside it"`
- `"it returns zero counts and empty LastWarning when portal.log is missing"`
- `"it skips a malformed / non-matching line without aborting the scan"`
- `"it sets LastWarning to the last qualifying line in append order (last-wins)"`
- `"it renders LastWarning as LEVEL component: msg when a component is present"`
- `"it renders LastWarning as LEVEL: msg with no stray space when the component is empty"`
- `"it renders LastWarning with no trailing space when the message is empty"`
- `"it ignores portal.log.old"`
- `"it counts a real-writer WARN line end-to-end (producer-coupled regression)"`

**Edge Cases**:
- WARN+ERROR counted; INFO+DEBUG excluded.
- One-hour window cutoff: inside counted, outside not.
- Missing `portal.log` → zero, no error.
- Malformed / non-matching line skipped without aborting the scan.
- Last-wins is positional in append (chronological) order.
- `LastWarning` rendering: component-present vs empty-component (no stray space before colon).
- Empty-message → no trailing space.
- `logFieldSeparator` / `expectedLogFieldCount` removed.
- No doc comment references the pipe format.
- `writeLogLine` helper removed.
- `portal.log.old` ignored.

**Context**:
> Spec §2 "Reader migration — `internal/state/status.go`" defines the qualifying condition (`ok && WARN/ERROR && !Time.Before(cutoff)`), the `LastWarning` composition (component-present vs empty-component, empty-message no-trailing-space), the positional last-wins semantics (the writer only ever appends in chronological order, so fixtures must be written in append order so "last in file" == "most-recent timestamp"), and the doc-comment/constant hygiene rule (no reference to the removed pipe format). Spec §Overview confirms `state → log` already exists and `log` never imports `state`, so the coupling is legal and cycle-free. Spec "Testing requirements" mandates removing the `writeLogLine` helper and adding at least one producer-coupled end-to-end regression test at the `CollectStatus` (state) layer that drives the real writer via the seam. Behaviour unchanged (spec §"Behaviour unchanged"): window (last hour), level filter, last-wins, missing-log → zero, best-effort no-error-propagation. `PortalLog(dir)` is `internal/state/paths.go:113`; `PortalLogOld(dir)` is line 116.

**Spec Reference**: `.workflows/status-recent-warnings-pipe-format/specification/status-recent-warnings-pipe-format/specification.md` (§2 Reader migration; Acceptance criteria; Testing requirements)

## status-recent-warnings-pipe-format-1-4 | approved

### Task 4: Migrate cmd-layer status tests to real-writer fixtures

**Problem**: The two warning-bearing cases in `cmd/state_status_test.go` (`TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` ~182-203 and `TestStateStatusExitNonZeroWhenRecentWarningsPresent` ~243-259) hand-author pipe-format log strings (`now.Format(time.RFC3339) + " | WARN | daemon | …"`) — a parallel format definition that validated the parser against input only the parser understood. They must source their log line from the real writer instead, and their asserted `(last: …)` suffix must update to the trimmed `<LEVEL> <component>: <msg>` form now that `LastWarning` arrives pre-trimmed.

**Solution**: Migrate (not delete) the two cmd-layer cases so each writes its `portal.log` line via the Task 2 render seam, and update the asserted `Recent warnings: N (last: …)` suffix to the trimmed `<LEVEL> <component>: <msg>` form. Retain both assertions: the rendered suffix and the `ErrStatusUnhealthy` non-zero exit when warnings are present. `cmd/state_status.go` is not changed.

**Outcome**: Both cmd-layer warning cases assert against real-writer output sourced from the single production format source, render `Recent warnings: N (last: <LEVEL> <component>: <msg>)`, and still assert `ErrStatusUnhealthy` (non-zero exit) when `RecentWarnings > 0` — no independently-defined format string remains in the file.

**Do**:
- In `/Users/leeovery/Code/portal/cmd/state_status_test.go`:
  - In `TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` (~182-203): remove the hand-authored `logLine := now.Format(time.RFC3339) + " | WARN | daemon | flush failed: disk full"`. Source the line from the real writer via the Task 2 seam (`log.RenderLineForTest(t, slog.LevelWarn, "daemon", "flush failed: disk full", …)` with a timestamp inside the one-hour window) and write it to `state.PortalLog(dir)`. The seam output already ends with a trailing `'\n'` (Task 2 contract), so write it verbatim — drop the `+"\n"` the legacy fixture appended; do not double the newline. Update the asserted suffix to `"  Recent warnings: 1 (last: WARN daemon: flush failed: disk full)\n"` (the trimmed `<LEVEL> <component>: <msg>` form — note the message contains a later colon which is preserved). **Keep the existing `strings.Contains(outBuf.String(), want)` matcher** — `portal state status` prints a multi-line report, so the warnings line is asserted as a substring, not via full-output equality. Retain the `ErrStatusUnhealthy` assertion (warnings > 0 → unhealthy).
  - In `TestStateStatusExitNonZeroWhenRecentWarningsPresent` (~243-259): remove the hand-authored `logLine := now.Format(time.RFC3339) + " | ERROR | daemon | crashed"`. Source an ERROR line from the seam (`log.RenderLineForTest(t, slog.LevelError, "daemon", "crashed", …)`, timestamp inside window), write it to `state.PortalLog(dir)`, and retain the `ErrStatusUnhealthy` assertion.
  - If sourcing fixtures from the seam is repetitive, add a local `*testing.T`-first helper in this test file that wraps `log.RenderLineForTest` + write to `state.PortalLog(dir)` (or reuse a shared `internal/state`-test helper if practical) — but it must delegate to the seam, never define a format string.
  - Add the `log/slog` and `github.com/leeovery/portal/internal/log` imports to `cmd/state_status_test.go` (the file currently imports neither). The `now.Format(time.RFC3339)` calls are removed with the hand-authored strings; `time` remains needed for the one-hour window math.
  - Verify no remaining hand-authored pipe-format string (`" | "`) exists anywhere in `cmd/state_status_test.go`.
- Do not modify `cmd/state_status.go` — `warningsLine` already renders `LastWarning` directly via `"%d (last: %s)"`; the displayed line follows from the pre-trimmed `LastWarning` produced in Task 3.

**Acceptance Criteria**:
- [ ] `TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` asserts `Recent warnings: 1 (last: WARN daemon: flush failed: disk full)` and sources its line from the real writer seam.
- [ ] `TestStateStatusExitNonZeroWhenRecentWarningsPresent` asserts `ErrStatusUnhealthy` (non-zero exit) when `RecentWarnings > 0` and sources its line from the real writer seam.
- [ ] Both cases are migrated, not deleted; their existing assertions (rendered suffix + non-zero exit) are retained.
- [ ] No independently-defined format string (no `" | "` pipe layout) remains in `cmd/state_status_test.go`.
- [ ] `cmd/state_status.go` is unchanged.

**Tests** (`cmd/state_status_test.go`, migrated):
- `"it renders Recent warnings: 1 (last: WARN daemon: flush failed: disk full) from a real-writer WARN line"`
- `"it exits non-zero (ErrStatusUnhealthy) when a real-writer warning is present"`

**Edge Cases**:
- Rendered `(last: <LEVEL> <component>: <msg>)` suffix (message later-colon preserved: `flush failed: disk full`).
- `ErrStatusUnhealthy` returned when `RecentWarnings > 0`.
- No independently-defined format string remains.
- Existing assertions (rendered suffix, non-zero exit) retained.

**Context**:
> Spec "Testing requirements → Where the producer-coupled assertion lives, and the cmd-layer tests": the two `cmd/state_status_test.go` pipe-format fixtures are migrated, not deleted — their assertions are retained (the rendered `Recent warnings: N (last: …)` suffix and the non-zero exit when warnings are present), but each must source its log line from the real `internal/log` writer rather than a hand-authored string, and the asserted `(last: …)` suffix updates to the trimmed `<LEVEL> <component>: <msg>` form. No cmd-layer test may construct a log line from an independently-defined format string. Spec §3 "Consumer — `cmd/state_status.go`": no change — `warningsLine` already renders `LastWarning` directly via `"%d (last: %s)"`, so the displayed line reads e.g. `Recent warnings: 1 (last: WARN daemon: tick complete)` once `LastWarning` arrives pre-trimmed from Task 3. Depends on Task 3 (production reader must already render the trimmed `LastWarning`) and Task 2 (sources fixtures from the seam).

**Spec Reference**: `.workflows/status-recent-warnings-pipe-format/specification/status-recent-warnings-pipe-format/specification.md` (§3 Consumer; Testing requirements → cmd-layer tests)
