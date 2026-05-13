---
status: complete
created: 2026-05-13
cycle: 2
phase: Plan Integrity Review
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: Distinguish Transport Errors in GetServerOption - Integrity

## Findings

### 1. Task 1-4's `TestTick_SkipsOnTransportError` lacks the subtest split the parallel flush test uses

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1-4 — `phase-1-tasks.md` Tests section (lines 267-271)
**Category**: Acceptance Criteria Quality / Task Template Compliance
**Change Type**: update-task

**Details**:
Task 1-4's Tests section lists three named subtests for the flush case (`returns_nil`, `zero_commits`, `warn_log_fires`) but only a single bare name for the tick case (`TestTick_SkipsOnTransportError`). The Acceptance Criteria (lines 262-263) demand two assertions for the tick test ("no capture / no commit calls") plus the same warn-log option as the flush test, but the Tests section does not name them. The asymmetry is small but invites the implementer to under-specify the tick test's subtests relative to the explicit flush-test pattern in the same task, and a reader scanning the Tests section sees only one test name where the AC requires multiple assertions.

Aligning the two test-name lists removes the asymmetry without adding scope — the asserted behaviours are already listed in the AC and the Do section; this just names them in the Tests block for parity.

**Current**:
```
**Tests** (this task's tests are the deliverable):
- `"TestDefaultShutdownFlush_SkipsOnTransportError/returns_nil"` — under injected `*CommandError{Stderr: "lost server"}`, flush returns `nil`.
- `"TestDefaultShutdownFlush_SkipsOnTransportError/zero_commits"` — same scenario, capture/commit seam shows zero commit calls.
- `"TestDefaultShutdownFlush_SkipsOnTransportError/warn_log_fires"` (optional, if existing harness has a log-capture seam) — warn log is emitted via the structured logger.
- `"TestTick_SkipsOnTransportError"` — under injected `*CommandError`, `tick` performs no capture/commit and returns without escalating.
```

**Proposed**:
```
**Tests** (this task's tests are the deliverable):
- `"TestDefaultShutdownFlush_SkipsOnTransportError/returns_nil"` — under injected `*CommandError{Stderr: "lost server"}`, flush returns `nil`.
- `"TestDefaultShutdownFlush_SkipsOnTransportError/zero_commits"` — same scenario, capture/commit seam shows zero commit calls.
- `"TestDefaultShutdownFlush_SkipsOnTransportError/warn_log_fires"` (optional, if existing harness has a log-capture seam) — warn log is emitted via the structured logger.
- `"TestTick_SkipsOnTransportError/no_capture"` — under injected `*CommandError{Stderr: "lost server"}`, `tick` performs zero capture calls via the existing capture/commit mock-tracking seam.
- `"TestTick_SkipsOnTransportError/no_commit"` — same scenario, zero commit calls.
- `"TestTick_SkipsOnTransportError/warn_log_fires"` (optional, if existing harness has a log-capture seam) — warn log is emitted via the structured logger.
```

**Resolution**: Fixed
**Notes**: Lowest-priority finding. The current single-name bullet is not incorrect — it just lacks the parallel structure of the flush case. Reject if the implementer should be free to choose subtest granularity.

---

### 2. Task 1-2 Edge Cases lists the non-`ExitError` case twice with overlapping content

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1-2 — `phase-1-tasks.md:107-112`
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:
Task 1-2's Edge Cases section opens (line 108) with "Non-`ExitError` underlying type (e.g., `*exec.Error` from `exec.LookPath` when `tmux` binary is missing) — wrap with empty `Stderr`; do not attempt to extract stderr from it." and closes (line 112) with "Non-`ExitError` underlying type assertion — must use `var exitErr *exec.ExitError; !errors.As(cmdErr.Err, &exitErr)` to confirm the wrap correctly identified the non-exit case. Asserting against `*exec.Error` directly is brittle if Go's exec internals change."

The two bullets cover the same underlying concern from two angles (production wrap behaviour vs test assertion form). They are not strict duplicates, but the section reads as if the author forgot the first bullet existed when writing the fifth. Merging them into a single bullet covering both halves keeps the content but removes the visual redundancy.

**Current**:
```
**Edge Cases**:
- Non-`ExitError` underlying type (e.g., `*exec.Error` from `exec.LookPath` when `tmux` binary is missing) — wrap with empty `Stderr`; do not attempt to extract stderr from it.
- `cmd.Stderr` assignment invariant — if a future change ever assigns `cmd.Stderr` (to tee, capture, etc.), `(*exec.ExitError).Stderr` becomes empty silently. The inline comment must call this out; spec calls this a "load-bearing invariant of the current RealCommander implementation."
- Exit error with empty stderr (process exited non-zero but emitted nothing on stderr) — `Stderr == ""` is acceptable; downstream discriminator treats empty stderr as non-match = non-absence = propagate.
- Platform applicability — `sh` not on `PATH` → `t.Skip`. Darwin + Linux always have it; the skip is defensive.
- Non-`ExitError` underlying type assertion — must use `var exitErr *exec.ExitError; !errors.As(cmdErr.Err, &exitErr)` to confirm the wrap correctly identified the non-exit case. Asserting against `*exec.Error` directly is brittle if Go's exec internals change.
```

**Proposed**:
```
**Edge Cases**:
- Non-`ExitError` underlying type (e.g., `*exec.Error` from `exec.LookPath` when `tmux` binary is missing) — production wrap sets `Stderr: ""` and does not attempt to extract stderr. Test assertion must use `var exitErr *exec.ExitError; !errors.As(cmdErr.Err, &exitErr)` to confirm the wrap correctly identified the non-exit case; asserting against `*exec.Error` directly is brittle if Go's exec internals change.
- `cmd.Stderr` assignment invariant — if a future change ever assigns `cmd.Stderr` (to tee, capture, etc.), `(*exec.ExitError).Stderr` becomes empty silently. The inline comment must call this out; spec calls this a "load-bearing invariant of the current RealCommander implementation."
- Exit error with empty stderr (process exited non-zero but emitted nothing on stderr) — `Stderr == ""` is acceptable; downstream discriminator treats empty stderr as non-match = non-absence = propagate.
- Platform applicability — `sh` not on `PATH` → `t.Skip`. Darwin + Linux always have it; the skip is defensive.
```

**Resolution**: Fixed
**Notes**: Pure tidy-up. Content is preserved verbatim; the merge only collapses two related bullets into one.

---
