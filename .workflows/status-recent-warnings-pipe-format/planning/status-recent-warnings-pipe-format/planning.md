# Plan: Status Recent Warnings Pipe Format

## Phases

### Phase 1: Migrate status reader to slog text format
status: approved
approved_at: 2026-06-09

**Goal**: Fix `portal state status` so the "Recent warnings" line reflects the actual WARN/ERROR lines in `portal.log`, by migrating the status reader (`internal/state/status.go`) from the legacy pipe-delimited layout to the slog text format the writer now emits — and defining that format in exactly one place via a new `ParseLogLine` helper exported from `internal/log` (the writer's package) and consumed by the reader.

**Why this order**: This is a single-root-cause bug with a contained fix. The reader assumes `timestamp | level | component | message` while the writer emits slog text (`<RFC3339Nano> <LEVEL> <component>: <msg> <attrs…>`), so every line fails the parse and warnings always count zero. The shared parse helper, the reader migration, the no-op consumer, and the producer-coupled regression suite all serve verifying the same correction — there is no independently valuable intermediate state, so splitting would create trivial phases. Per bugfix guidance, reproduce-then-fix-with-regression-tests fits one phase.

**Acceptance**:
- [ ] `internal/log` exports `ParseLogLine(line string) (LogLine, ok bool)` — the single inverse of the writer's line format, co-located with the writer — returning `Time`/`Level`/`Component`/`Message` per the spec's parsing rules
- [ ] `ParseLogLine` unit coverage passes: well-formed line yields correct fields; the message-boundary rule strips contextual attrs and the `pid`/`version`/`process_role` baselines; a no-attr message is preserved whole; an empty-component line parses with `Component == ""` and `ok == true`; an unparseable timestamp yields `ok == false`
- [ ] `internal/state/status.go` removes `logFieldSeparator` / `expectedLogFieldCount` and parses each line once via `log.ParseLogLine`; no doc comment or constant references the removed pipe format
- [ ] A `WARN`/`ERROR` line written by the real `internal/log` writer with a timestamp inside the last hour causes `CollectStatus` to report `RecentWarnings ≥ 1`
- [ ] `LastWarning` is the trimmed `<LEVEL> <component>: <msg>` summary of the most recent qualifying entry (last-wins), rendering `<LEVEL>: <msg>` when component is empty and dropping the trailing space when the message is empty; `portal state status` prints `Recent warnings: N (last: <LEVEL> <component>: <msg>)`
- [ ] `isUnhealthy` returns true and the process exits non-zero when `RecentWarnings > 0`
- [ ] Preserved behaviour is verified in the slog text format: a qualifying-level line older than the one-hour window is not counted; `INFO`/`DEBUG` lines are not counted; missing `portal.log` yields `RecentWarnings = 0`, `LastWarning = ""`, no error; a line not matching the new layout is silently skipped without aborting the remaining scan
- [ ] No test constructs a log line from a format string defined independently of the production writer: the `writeLogLine` helper in `internal/state/status_test.go` is removed, the two `cmd/state_status_test.go` cases are migrated (assertions retained, log line sourced from the real writer, suffix updated to the trimmed form), and a producer-coupled seam in `internal/log` renders via the same path `textHandler.Handle` uses in production
- [ ] At least one producer-coupled end-to-end regression test drives the real writer to emit a WARN line into the status directory's `portal.log`, runs `CollectStatus`, asserts `RecentWarnings` / trimmed `LastWarning` / the non-zero-exit consequence, and would turn red on any future writer-format drift
- [ ] `go test ./...` passes — no regressions in existing status, log, or cmd tests

#### Tasks
status: approved
approved_at: 2026-06-09

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| status-recent-warnings-pipe-format-1-1 | Add ParseLogLine helper to internal/log | unparseable timestamp → ok=false, no colon → ok=false, <2 whitespace tokens → ok=false, empty line → ok=false, empty component → Component="" ok=true, empty message → Message="" ok=true, message with later colons preserved, key=value boundary drops contextual attrs + pid/version/process_role baselines, quoted multi-word attr value does not shift boundary, no-attrs message preserved whole |
| status-recent-warnings-pipe-format-1-2 | Factor a producer-coupled render seam out of textHandler.Handle | byte-identical to Handle output incl. baselines and trailing newline, no process-global handler mutation, testing.T-first test-only marker |
| status-recent-warnings-pipe-format-1-3 | Migrate status reader to ParseLogLine | WARN+ERROR counted / INFO+DEBUG excluded, one-hour window cutoff inside vs outside, missing portal.log → 0 no error, malformed/non-matching line skipped without aborting scan, last-wins positional in append order, LastWarning component-present vs empty-component rendering, empty-message no-trailing-space, logFieldSeparator/expectedLogFieldCount removed, no doc comment references pipe format, writeLogLine helper removed, portal.log.old ignored |
| status-recent-warnings-pipe-format-1-4 | Migrate cmd-layer status tests to real-writer fixtures | rendered (last: <LEVEL> <component>: <msg>) suffix, ErrStatusUnhealthy when RecentWarnings > 0, no independently-defined format string remains, assertions retained |
