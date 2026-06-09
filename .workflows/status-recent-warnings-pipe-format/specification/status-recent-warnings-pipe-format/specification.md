# Specification: Status Recent Warnings Pipe Format

## Specification

## Problem & Scope

### Bug

`portal state status` always reports **`Recent warnings: 0 (last: none)`** regardless of what WARN/ERROR lines `portal.log` actually contains. The failure is silent — no error, no crash. The "Recent warnings" section is permanently empty in production, masking real daemon/bootstrap/restore warnings.

### Root cause

The status log reader (`internal/state/status.go`) parses `portal.log` assuming a **legacy pipe-delimited** field layout (`timestamp | level | component | message`, separator `" | "`, 4 fields). The observability layer changed the *writer* (`internal/log`) to **slog text** format:

```
<RFC3339Nano> <LEVEL> <component>: <msg> <attrs k=v…> pid=… version=… process_role=…
```

There are no `" | "` separators in this format, so `logEntryQualifies`'s `strings.SplitN(line, " | ", 4)` yields a single-element slice (`len < 4`) for **every** line and returns `false` unconditionally. `RecentWarnings` is therefore always `0` and `LastWarning` always `""`.

The contract between the writer (`internal/log`) and reader (`internal/state/status.go`) was broken on the writer side with no corresponding reader update, and no test exercised the reader against real writer output.

### Blast radius

**Affected — the two warning-derived fields only:**
- `portal state status` "Recent warnings" line → always `0 (last: none)`.
- The health/exit-code policy: `isUnhealthy`'s `RecentWarnings > 0` branch (`cmd/state_status.go`) can never fire, so a daemon actively logging WARN/ERROR still reports healthy (exit 0).

**Not affected (confirmed):**
- All other status fields — daemon running/PID/version, last save, sessions/panes counts, state size — read `daemon.pid` / `daemon.version` / `sessions.json` / the state-dir tree, **not** `portal.log`.
- No other production code reads `portal.log` (`scanRecentWarnings` is the only non-test caller of `PortalLog(`).

### Out of scope

- **The writer/handler is correct** and is not changed. Only the reader drifted; only the reader is fixed.
- No change to the recent-warnings window (last hour), the level filter (WARN/ERROR), the last-wins semantics, the malformed-line swallow-and-skip contract, or `CollectStatus`'s best-effort no-error-propagation behaviour.

### Severity & release class

Medium — a diagnostic/observability regression (the command lies by omission), not data-loss or a crash. Suitable for a **regular release**, not a hotfix.

## Solution Design

### Overview

Migrate the status reader to parse the slog **text** format, and define that format in **one** place by exporting a parse helper from `internal/log` (the writer's package) and consuming it from `internal/state/status.go`. The import direction `state → log` already exists and `log` never imports `state`, so the coupling is legal and cycle-free. This directly removes the "writer and reader each define the format independently" defect that caused the drift.

### 1. Shared parse helper in `internal/log`

A new exported helper (e.g. `internal/log/parse.go`) is the single inverse of the writer's line format. It is defined alongside the writer so any future writer-format change forces this parser to change too.

**Contract:**

```go
// LogLine holds the fields parsed from one rendered portal.log text line.
type LogLine struct {
    Time      time.Time // parsed from the RFC3339Nano timestamp token
    Level     string    // "DEBUG" | "INFO" | "WARN" | "ERROR"
    Component string    // subsystem prefix (trailing ':' removed); "" if absent
    Message   string    // human message only — contextual attrs and the
                        // pid/version/process_role baselines excluded
}

// ParseLogLine parses one portal.log text line. ok=false for any line that
// does not match the writer's layout (wrong shape / unparseable timestamp).
func ParseLogLine(line string) (parsed LogLine, ok bool)
```

**Parsing rules** (derived from the writer layout `<RFC3339Nano> <LEVEL> <component>: <msg> <attrs…> pid=… version=… process_role=…`):

- **Time** = the first whitespace-delimited token, parsed with `time.RFC3339Nano`. (Whole-second and fractional-second inputs both parse; `time.RFC3339` is equivalent — the writer emits RFC3339Nano, so the helper uses the matching layout for producer-consumer symmetry.) Unparseable → `ok=false`.
- **Level** = the second whitespace-delimited token, verbatim.
- **Component** = the run after the level token up to the first `:` (component names carry no spaces or colons), trailing `:` removed. An empty component (writer emitted no component) yields `Component == ""` and still `ok=true`.
- **Message** = the text after `<component>: `, up to (but excluding) the first whitespace-delimited token of the form `key=value` (matching `^[A-Za-z_][A-Za-z0-9_.]*=`). This single boundary rule drops both contextual attrs and the trailing baselines in one pass.
  - **Documented assumption:** log messages do not contain a `key=value`-shaped token. The codebase's messages are short human phrases (closed catalogs per component), so this holds. If ever violated, the *only* effect is the displayed `LastWarning` summary truncating early — it never affects `RecentWarnings` count or the health signal.

### 2. Reader migration — `internal/state/status.go`

- **Remove** the `logFieldSeparator = " | "` and `expectedLogFieldCount = 4` constants.
- `scanRecentWarnings` parses each line **once** via `log.ParseLogLine`. A line is a qualifying entry when: `ok == true` **and** `Level` is `WARN` or `ERROR` **and** `!Time.Before(cutoff)`. Non-qualifying / unparseable lines are silently skipped (the existing swallow-and-skip contract is preserved — a 100%-mismatch now becomes a 0%-mismatch, but malformed individual lines still skip cleanly).
- On a qualifying line, increment `RecentWarnings` and set `LastWarning` to the composed summary **`<LEVEL> <component>: <msg>`** (e.g. `WARN daemon: tick complete`), last-wins. This replaces storing the raw line.
- Update the `StatusReport.LastWarning` doc comment: from "full text of the most recent qualifying WARN/ERROR log line" to "the most recent qualifying entry rendered as `<LEVEL> <component>: <msg>` — timestamp prefix and trailing attrs/baselines omitted."

### 3. Consumer — `cmd/state_status.go`

**No change.** `warningsLine` already renders `LastWarning` directly (`"%d (last: %s)"`); because `LastWarning` now arrives pre-trimmed, the displayed line reads e.g. `Recent warnings: 1 (last: WARN daemon: tick complete)`. The `isUnhealthy` `RecentWarnings > 0` branch now fires correctly once real warnings are counted.

### Behaviour unchanged

The recent-warnings window (last hour, `recentWarningWindow`), the WARN/ERROR level filter, last-wins selection, missing-`portal.log` → zero-counts, and `CollectStatus`'s best-effort no-error-propagation are all preserved. The writer/handler in `internal/log` is not modified.

## Acceptance Criteria & Testing

### Acceptance criteria (behaviour)

Given `portal.log` written by the real `internal/log` writer:

1. A `WARN` or `ERROR` line with a timestamp within the last hour → `CollectStatus` reports `RecentWarnings ≥ 1`.
2. `LastWarning` is the trimmed summary `<LEVEL> <component>: <msg>` of the most recent qualifying entry (last-wins), and `portal state status` renders `Recent warnings: N (last: <LEVEL> <component>: <msg>)`.
3. `isUnhealthy` returns true (process exits non-zero) when `RecentWarnings > 0`.
4. A qualifying-level line **older** than the one-hour window is **not** counted.
5. An `INFO`/`DEBUG` line is **not** counted (level filter intact).
6. Missing `portal.log` → `RecentWarnings = 0`, `LastWarning = ""`, no error.
7. A malformed/unparseable line is silently skipped and does not abort the scan of remaining lines.

### Testing requirements

**Anti-false-green (the core requirement).** The original bug survived because the existing status tests hand-authored pipe-format fixtures — validating the parser against input only the parser understood. The fix must close this:

- **Remove the independent format fixtures.** The hand-authored pipe-format helpers/lines must go: `writeLogLine` in `internal/state/status_test.go` (emits `… | … | … | …`) and the two pipe-format fixtures in `cmd/state_status_test.go`. No test may construct a log line from a format string defined independently of the production writer.
- **At least one producer-coupled end-to-end regression test.** A test must drive the **real `internal/log` writer** to emit a `WARN` line into the status directory's `portal.log`, then run `CollectStatus` and assert `RecentWarnings`, the trimmed `LastWarning`, and the `isUnhealthy`/non-zero-exit consequence. This is the guard that would have caught the original mismatch and will turn **red** on any future writer-format drift — fixtures derive from the single production format source, never a parallel definition.

**Retained coverage, re-expressed in the slog text format.** The existing cases must be preserved (translated to the new format, not deleted): window-cutoff (inside vs. outside the last hour), level-filter (`INFO`/`DEBUG` excluded; both `WARN` and `ERROR` included), last-wins selection, missing-`portal.log` → zero, and the malformed-line skip case (currently `status_test.go:368`) re-expressed as a line that does not match the new layout.

**Unit coverage for `ParseLogLine`.** Direct tests of the new helper: a well-formed line → correct `Time`/`Level`/`Component`/`Message`; the message-boundary rule strips contextual attrs and the `pid`/`version`/`process_role` baselines; a message with no attrs is preserved whole; an empty-component line parses with `Component == ""` and `ok == true`; an unparseable timestamp → `ok == false`.

### Definition of done

`portal state status` surfaces the actual recent-warning count with a readable last-warning summary, exits non-zero when warnings are present, and a producer-coupled test fails if the writer's line format changes without a matching parser update.

---

## Working Notes
