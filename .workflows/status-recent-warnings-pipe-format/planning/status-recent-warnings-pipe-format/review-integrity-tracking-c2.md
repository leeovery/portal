---
status: in-progress
created: 2026-06-09
cycle: 2
phase: Plan Integrity Review
topic: Status Recent Warnings Pipe Format
---

# Review Tracking: Status Recent Warnings Pipe Format - Integrity

## Summary

Cycle-1's four refinements are all present and verified in the current task text:

1. **Task 4 `strings.Contains` matcher** — present in Task 4 "Do" ("**Keep the existing `strings.Contains(outBuf.String(), want)` matcher**…") and reinforced in its first acceptance criterion ("via `strings.Contains` substring match").
2. **Task 2 `RenderLineForTest` RFC3339Nano stamping** — present in Task 2 "Do" ("rendered through the same `r.Time.Format(time.RFC3339Nano)` path `Handle` uses…").
3. **Task 4 new test imports named** — present in Task 4 "Do" ("Add the `log/slog` and `github.com/leeovery/portal/internal/log` imports…").
4. **Task 3 `LastWarning` doc-comment exact Current/Proposed text** — present in Task 3 "Do" with both the verbatim current comment (status.go:65-66) and the replacement, fenced as Go.

All load-bearing source references were re-verified against the live tree and check out exactly: `logFieldSeparator` (status.go:19), `expectedLogFieldCount` (status.go:23), the `StatusReport.LastWarning` doc comment text (status.go:65-66, matches Task 3's Current block byte-for-byte), `scanRecentWarnings` doc + body (status.go:181-202), `logEntryQualifies` (status.go:204-221), the `textHandler.Handle` builder sequence and level-bypass gate (handler.go), `PortalLog`/`PortalLogOld` (paths.go), the two cmd-layer fixture cases and their hand-authored pipe strings (state_status_test.go:188-189, 249-250), and the current import set of `cmd/state_status_test.go` (neither `log/slog` nor `internal/log` imported, `time` still used). The dependency graph in tick is correct and acyclic — T3 ← {T1, T2}, T4 ← {T3, T2}, T1 ⊥ T2 — and every edge reflects a genuine capability requirement.

The plan remains structurally strong: one phase, four well-scoped single-TDD-cycle tasks inside one architectural boundary, full template on every task. One Minor finding remains — a trailing-newline detail in how the seam's output is written to the fixture file. No Critical or Important findings.

## Findings

### 1. Fixture-write steps in Tasks 3 and 4 don't state that the seam output already includes the trailing newline

**Severity**: Minor
**Plan Reference**: Phase 1, Tasks `status-recent-warnings-pipe-format-1-3` and `status-recent-warnings-pipe-format-1-4`
**Category**: Implementation Readiness / Task Self-Containment
**Change Type**: add-to-task

**Details**:
Task 2's seam contract specifies that `RenderLineForTest` returns the canonical `portal.log` line **including the trailing `'\n'`** (Task 2 "Do": "ending with the trailing `'\n'`"; Task 2 acceptance criterion: "and a trailing `'\n'`"). Tasks 3 and 4 then instruct writing/appending that rendered output to `state.PortalLog(dir)` but do not note that the newline is already present. The legacy code these tasks replace appended its own newline (`os.WriteFile(state.PortalLog(dir), []byte(logLine+"\n"), …)` at state_status_test.go:189-190 and 250-251; the removed `writeLogLine` helper did the same). An implementer migrating by analogy could append a second `"\n"` to already-newline-terminated seam output, producing a blank line in the fixture file.

This is genuinely minor — it does not break any assertion. Task 4's assertions match against the *rendered status output* via `strings.Contains` (not the log file), and Task 3's `bufio.Scanner` line scan treats a trailing empty line as a non-matching line that is silently skipped (empty line → `ParseLogLine` `ok=false`). But the plan is exhaustive about byte-level fixture sourcing elsewhere (it explicitly closes the format-drift hole), so naming the trailing-newline ownership keeps the fixture-write steps unambiguous and prevents an accidental double newline. The note belongs in both tasks because each independently writes seam output to `state.PortalLog(dir)`.

**Current** (Task 3, `internal/state/status_test.go` bullet — the seam fixture helper):
```markdown
  - Add a `*testing.T`-first fixture helper that writes a real-writer line to `portal.log`, sourcing the rendered bytes from the Task 2 seam (`log.RenderLineForTest`). The helper appends the rendered line(s) to `state.PortalLog(dir)` (a plain file matching `PortalLog(dir)`; no rotation/symlink needed) in append (chronological) order so "last in file" == "most-recent timestamp".
```

**Proposed** (Task 3):
```markdown
  - Add a `*testing.T`-first fixture helper that writes a real-writer line to `portal.log`, sourcing the rendered bytes from the Task 2 seam (`log.RenderLineForTest`). The helper appends the rendered line(s) to `state.PortalLog(dir)` (a plain file matching `PortalLog(dir)`; no rotation/symlink needed) in append (chronological) order so "last in file" == "most-recent timestamp". The seam output already ends with the trailing `'\n'` (Task 2 contract), so the helper appends it verbatim — do **not** add a second newline.
```

**Current** (Task 4, `cmd/state_status_test.go` — first migrated case):
```markdown
  - In `TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` (~182-203): remove the hand-authored `logLine := now.Format(time.RFC3339) + " | WARN | daemon | flush failed: disk full"`. Source the line from the real writer via the Task 2 seam (`log.RenderLineForTest(t, slog.LevelWarn, "daemon", "flush failed: disk full", …)` with a timestamp inside the one-hour window) and write it to `state.PortalLog(dir)`. Update the asserted suffix to `"  Recent warnings: 1 (last: WARN daemon: flush failed: disk full)\n"` (the trimmed `<LEVEL> <component>: <msg>` form — note the message contains a later colon which is preserved). **Keep the existing `strings.Contains(outBuf.String(), want)` matcher** — `portal state status` prints a multi-line report, so the warnings line is asserted as a substring, not via full-output equality. Retain the `ErrStatusUnhealthy` assertion (warnings > 0 → unhealthy).
```

**Proposed** (Task 4):
```markdown
  - In `TestStateStatusRecentWarningsLastLineSuffixWhenNonZero` (~182-203): remove the hand-authored `logLine := now.Format(time.RFC3339) + " | WARN | daemon | flush failed: disk full"`. Source the line from the real writer via the Task 2 seam (`log.RenderLineForTest(t, slog.LevelWarn, "daemon", "flush failed: disk full", …)` with a timestamp inside the one-hour window) and write it to `state.PortalLog(dir)`. The seam output already ends with a trailing `'\n'` (Task 2 contract), so write it verbatim — drop the `+"\n"` the legacy fixture appended; do not double the newline. Update the asserted suffix to `"  Recent warnings: 1 (last: WARN daemon: flush failed: disk full)\n"` (the trimmed `<LEVEL> <component>: <msg>` form — note the message contains a later colon which is preserved). **Keep the existing `strings.Contains(outBuf.String(), want)` matcher** — `portal state status` prints a multi-line report, so the warnings line is asserted as a substring, not via full-output equality. Retain the `ErrStatusUnhealthy` assertion (warnings > 0 → unhealthy).
```

**Resolution**: Pending
**Notes**: Applies to both tasks. Task 4's second migrated case (`TestStateStatusExitNonZeroWhenRecentWarningsPresent`) writes its ERROR line the same way; if the first case's note is adopted, the implementer carries the same verbatim-write convention to the second case (no separate edit strictly required, but the helper-extraction bullet already covers it).

---
