---
status: complete
created: 2026-06-09
cycle: 1
phase: Plan Integrity Review
topic: Status Recent Warnings Pipe Format
---

# Review Tracking: Status Recent Warnings Pipe Format - Integrity

## Summary

The plan is structurally strong. One phase, four well-scoped tasks, each a single TDD cycle within one architectural boundary. Every task carries the full template (Problem/Solution/Outcome/Do/Acceptance/Tests/Edge Cases/Context/Spec Reference). All load-bearing references were verified against the live source and check out exactly: `logFieldSeparator` (status.go:19) and `expectedLogFieldCount` (status.go:23); `scanRecentWarnings`/`logEntryQualifies` (status.go:186-221); the `textHandler.Handle` builder sequence (handler.go:147-181) including the level-bypass gate (142-145); the `*testing.T`-first `SetTestHandler` pattern (testhandler.go) and `currentHandler`/`setHandler` (log.go:123-132); `PortalLog`/`PortalLogOld` (paths.go:113/116); and the two cmd-layer fixture cases (state_status_test.go:182-203, 243-259). The dependency graph is correct and acyclic: T3 ← {T1, T2}, T4 ← {T3, T2}, with T1 and T2 genuinely independent.

Findings below are refinements, not blockers — one Important (a test-matcher detail that could otherwise cause a false migration failure) and three Minor.

## Findings

### 1. Task 4 omits the `strings.Contains` matcher detail, risking a brittle/incorrect migrated assertion

**Severity**: Important
**Plan Reference**: Phase 1, Task `status-recent-warnings-pipe-format-1-4` (`TestStateStatusRecentWarningsLastLineSuffixWhenNonZero`)
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Change Type**: update-task

**Details**:
The existing test asserts the suffix via `strings.Contains(outBuf.String(), want)` (state_status_test.go:200), not an exact full-output equality. Task 4's "Do" instructs updating the asserted suffix to the literal `"  Recent warnings: 1 (last: WARN daemon: flush failed: disk full)\n"` but does not state that the matcher remains a substring (`strings.Contains`) check. An implementer reading only Task 4 could reasonably switch to an exact-output comparison, which would fail because `portal state status` prints a multi-line report (daemon/save/size lines precede the warnings line). The substring semantics are load-bearing and must be preserved. This belongs in the task because Task 4's own acceptance criteria assert the exact suffix string but say nothing about how it is matched.

**Current**:
```markdown
  - In `TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` (~182-203): remove the hand-authored `logLine := now.Format(time.RFC3339) + " | WARN | daemon | flush failed: disk full"`. Source the line from the real writer via the Task 2 seam (`log.RenderLineForTest(t, slog.LevelWarn, "daemon", "flush failed: disk full", …)` with a timestamp inside the one-hour window) and write it to `state.PortalLog(dir)`. Update the asserted suffix to `"  Recent warnings: 1 (last: WARN daemon: flush failed: disk full)\n"` (the trimmed `<LEVEL> <component>: <msg>` form — note the message contains a later colon which is preserved). Retain the `ErrStatusUnhealthy` assertion (warnings > 0 → unhealthy).
```

**Proposed**:
```markdown
  - In `TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` (~182-203): remove the hand-authored `logLine := now.Format(time.RFC3339) + " | WARN | daemon | flush failed: disk full"`. Source the line from the real writer via the Task 2 seam (`log.RenderLineForTest(t, slog.LevelWarn, "daemon", "flush failed: disk full", …)` with a timestamp inside the one-hour window) and write it to `state.PortalLog(dir)`. Update the asserted suffix to `"  Recent warnings: 1 (last: WARN daemon: flush failed: disk full)\n"` (the trimmed `<LEVEL> <component>: <msg>` form — note the message contains a later colon which is preserved). **Keep the existing `strings.Contains(outBuf.String(), want)` matcher** — `portal state status` prints a multi-line report, so the warnings line is asserted as a substring, not via full-output equality. Retain the `ErrStatusUnhealthy` assertion (warnings > 0 → unhealthy).
```

**Resolution**: Fixed
**Notes**:

---

### 2. `RenderLineForTest` timestamp format (RFC3339Nano) not surfaced where the cmd/state tests depend on it

**Severity**: Minor
**Plan Reference**: Phase 1, Tasks `status-recent-warnings-pipe-format-1-3` and `-1-4`
**Category**: Task Self-Containment
**Change Type**: add-to-task

**Details**:
The legacy cmd-layer fixtures stamp the timestamp with `time.RFC3339` (state_status_test.go:189, 250), while the new parser (`ParseLogLine`, Task 1) and the production writer use `time.RFC3339Nano`. The render seam (Task 2) builds the line through `textHandler.render`, which formats with `RFC3339Nano` — so the migrated tests get the correct format for free *via the seam*. This is correct as written, but the dependency is implicit: an implementer of Task 3/4 who hand-writes a timestamp (rather than letting the seam stamp it) could regress to `RFC3339` and silently parse `ok=false`. A one-line note in Task 2's "Do" that the seam stamps `RFC3339Nano` (matching the parser) removes the ambiguity. This is informational hardening, not a correctness gap in the current text.

**Current**:
```markdown
  - Allow the caller to supply the record time (add a time parameter, or document that the helper stamps a caller-chosen time) so Task 3/4 can place lines inside or outside the one-hour window. If a single fixed signature is cleaner, accept a `time.Time` as the first value parameter after `t`.
```

**Proposed**:
```markdown
  - Allow the caller to supply the record time (add a time parameter, or document that the helper stamps a caller-chosen time) so Task 3/4 can place lines inside or outside the one-hour window. If a single fixed signature is cleaner, accept a `time.Time` as the first value parameter after `t`. The caller-supplied time is rendered through the same `r.Time.Format(time.RFC3339Nano)` path `Handle` uses, so the fixture's timestamp is `RFC3339Nano` — exactly what `ParseLogLine` (Task 1) expects. Tests must always source the line from this seam rather than hand-formatting a timestamp, so the format cannot drift back to the legacy `time.RFC3339`.
```

**Resolution**: Fixed
**Notes**:

---

### 3. Task 4 does not list the new test-file imports it will require

**Severity**: Minor
**Plan Reference**: Phase 1, Task `status-recent-warnings-pipe-format-1-4`
**Category**: Implementation Readiness
**Change Type**: add-to-task

**Details**:
`cmd/state_status_test.go` currently imports only `bytes`, `os`, `path/filepath`, `strconv`, `strings`, `testing`, `time`, and `internal/state`. The migration requires `log/slog` (for `slog.LevelWarn`/`slog.LevelError`) and `github.com/leeovery/portal/internal/log` (for `RenderLineForTest`), and likely drops the now-unused `time.RFC3339` formatting (`time` is still needed for the window math). Naming the import delta keeps the task self-contained — minor, since a Go implementer would resolve imports trivially, but the plan elsewhere is exhaustive about file-level detail so this is a consistency gap.

**Current**:
```markdown
  - If sourcing fixtures from the seam is repetitive, add a local `*testing.T`-first helper in this test file that wraps `log.RenderLineForTest` + write to `state.PortalLog(dir)` (or reuse a shared `internal/state`-test helper if practical) — but it must delegate to the seam, never define a format string.
  - Verify no remaining hand-authored pipe-format string (`" | "`) exists anywhere in `cmd/state_status_test.go`.
```

**Proposed**:
```markdown
  - If sourcing fixtures from the seam is repetitive, add a local `*testing.T`-first helper in this test file that wraps `log.RenderLineForTest` + write to `state.PortalLog(dir)` (or reuse a shared `internal/state`-test helper if practical) — but it must delegate to the seam, never define a format string.
  - Add the `log/slog` and `github.com/leeovery/portal/internal/log` imports to `cmd/state_status_test.go` (the file currently imports neither). The `now.Format(time.RFC3339)` calls are removed with the hand-authored strings; `time` remains needed for the one-hour window math.
  - Verify no remaining hand-authored pipe-format string (`" | "`) exists anywhere in `cmd/state_status_test.go`.
```

**Resolution**: Fixed
**Notes**:

---

### 4. `StatusReport.LastWarning` doc comment edit names line numbers but not the exact current text being replaced

**Severity**: Minor
**Plan Reference**: Phase 1, Task `status-recent-warnings-pipe-format-1-3`
**Category**: Implementation Readiness
**Change Type**: update-task

**Details**:
Task 3 instructs updating the `StatusReport.LastWarning` doc comment "(lines ~65-66)" to new prose but does not quote the exact current comment (`// LastWarning is the full text of the most recent qualifying WARN/ERROR` / `// log line (last-wins). Empty when there are no qualifying entries.` — status.go:65-66). Quoting the current text makes the edit unambiguous and self-verifying (the same way the task quotes the constants to remove). The other doc-comment edit in this task (`scanRecentWarnings`, ~181-185) has the same characteristic. This is a polish/consistency item; line refs plus the new text are already enough for a careful implementer.

**Current**:
```markdown
  - Update the `StatusReport.LastWarning` doc comment (lines ~65-66) from "the full text of the most recent qualifying WARN/ERROR log line" to: "the most recent qualifying entry rendered as `<LEVEL> <component>: <msg>` — timestamp prefix and trailing attrs/baselines omitted. Empty when there are no qualifying entries."
```

**Proposed**:
```markdown
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
```

**Resolution**: Fixed
**Notes**:

---
