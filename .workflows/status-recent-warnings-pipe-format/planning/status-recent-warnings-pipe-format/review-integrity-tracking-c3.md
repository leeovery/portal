---
status: complete
created: 2026-06-09
cycle: 3
phase: Plan Integrity Review
topic: Status Recent Warnings Pipe Format
---

# Review Tracking: Status Recent Warnings Pipe Format - Integrity

## Findings

None. Cycle 3 is a convergence check and the plan is implementation-ready.

## Convergence Verification

This cycle re-reviewed the corrected plan after the 5 refinements applied in prior
cycles. All refinements are present and correct, all load-bearing source claims were
verified against the live tree, and no new structural gaps surfaced.

### Prior-cycle refinements confirmed present

1. **`strings.Contains` matcher retained** — Task 4 (1-4) explicitly keeps the
   substring matcher (`strings.Contains(outBuf.String(), want)`); both the Do step
   and acceptance criterion state it. Verified against `cmd/state_status_test.go:200`.
2. **RFC3339Nano stamping note** — Task 2 (1-2) documents that the caller-supplied
   time renders through the same `r.Time.Format(time.RFC3339Nano)` path `Handle`
   uses, so fixtures cannot drift back to the legacy `time.RFC3339`.
3. **Task 4 import delta** — Task 4 documents adding `log/slog` +
   `github.com/leeovery/portal/internal/log`, that the file currently imports
   neither, and that `time` stays for window math. Verified: the file imports
   neither today (`cmd/state_status_test.go:3-13`).
4. **Exact LastWarning doc-comment text** — Task 3 (1-3) carries the verbatim
   current and replacement doc-comment blocks. Verified the current text matches
   `internal/state/status.go:65-66` exactly.
5. **Cycle-2 newline note** — "seam output already ends with the trailing `'\n'`
   (Task 2 contract), so … do not double the newline" appears in both Task 3 (the
   fixture helper) and Task 4 (both cmd cases). Confirmed in both.

### Source-claim verification (every cited location confirmed accurate)

- `logFieldSeparator` at `status.go:19`, `expectedLogFieldCount` at `status.go:23` — exact.
- `scanRecentWarnings` at `status.go:186-202`, `logEntryQualifies` at `status.go:207` — exact.
- `StatusReport.LastWarning` doc comment at `status.go:65-66` — current text matches verbatim.
- `PortalLog(dir)` at `paths.go:113`, `PortalLogOld(dir)` at `paths.go:116` — exact.
- `writeLogLine` helper at `status_test.go:13-29` — present, constructs the pipe format, must be removed (Task 3 correctly targets it).
- Two cmd-layer warning fixtures at `cmd/state_status_test.go:189` (WARN) and `:250` (ERROR) — exact; these are the only two hand-authored pipe-format strings in the file (grep confirms no third).
- `warningsLine` renders `"%d (last: %s)"` at `cmd/state_status.go:112` — confirms Task 4's "consumer unchanged" claim.
- `textHandler.Handle` line-builder at `handler.go:147-181` ending with `b.WriteByte('\n')` at `:179`; level-filter/bypass gate at `:142-145`; `bestEffortWrite` call at `:181` — matches Task 2's extraction targets exactly.
- No existing `internal/log/parse.go`, `internal/log/rendertest.go`, `render` method, or `RenderLineForTest` symbol — no collision for the new files/symbols Tasks 1 and 2 introduce.

### Structural-quality checks (all pass)

- **Import cycle**: confirmed safe. `internal/state` already imports `internal/log`
  in 5+ production files; `internal/log` never imports `internal/state` (guarded in
  init.go/names.go/sink.go). Task 3 adds only a new symbol from an already-imported
  package — Task 3's cycle-free claim is accurate.
- **Dependency graph**: acyclic and complete. T1, T2 are parallel foundation
  (no blockers); T3 ← {T1, T2} convergence; T4 ← {T3, T2}. Both convergence points
  carry explicit edges, and the direct T2→T4 edge (seam used directly by T4, not
  only transitively through T3) is present and correct.
- **Priorities**: uniform (all priority 2). With explicit `blocked_by` edges driving
  order and creation-date preserving authoring order, `tick ready` yields the correct
  sequence (T1/T2 → T3 → T4). Per the reading-reference natural-order convention,
  uniform priority is acceptable here — no priority-vs-graph mismatch.
- **Task template compliance**: all four tasks carry Problem, Solution, Outcome, Do,
  Acceptance Criteria, Tests, Edge Cases, Context, and Spec Reference. Criteria are
  concrete pass/fail; Tests cover edge cases (empty component, empty message, quoted
  multi-word attr, later-colon preservation, malformed-line skip, last-wins).
- **Vertical slicing / scope**: each task is a single TDD cycle with an independently
  writable test. T1 (parser + unit tests), T2 (render seam + byte-identity test),
  T3 (reader migration + producer-coupled regression), T4 (cmd-layer fixture
  migration). No horizontal layering; no task is mechanical boilerplate.
- **Self-containment**: each task pulls forward the spec decisions it needs (boundary
  regex, qualifying condition, LastWarning composition rules, the documented
  no-`key=value`-in-messages assumption) without requiring the implementer to read a
  sibling task to know what to build.

## Findings Summary

- Critical: 0
- Important: 0
- Minor: 0

Plan is implementation-ready. Returning clean.
