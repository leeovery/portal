---
scope: slow-open-empty-previews-and-zombie-sessions
cycle: 1
source: review
total_findings: 51
deduplicated_findings: 4
proposed_tasks: 4
---
# Review Report: Slow Open Empty Previews And Zombie Sessions (Cycle 1)

## Summary

Review verdict is Request Changes. One blocking issue: T9-7 left
`TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` asserting ERROR-level
log lines after production was changed to emit WARN per Component C spec —
test now fails. Two significant non-blocking bugs in the T6-4 scrollback
stability harness silently green-light the exact regressions the test was
designed to catch (empty baseline + missing scrollback dir). One spec/impl
mismatch on Component F acceptance bullet 3 needs a docs-side decision.

## Discarded Findings

Per the synthesizer's filter rules (low-severity quickfixes and ideas
discarded unless they cluster into a pattern). Each item below was assessed
individually; none cluster.

- Quickfixes #1-18 (excluding those promoted) — minor wording / doc-hygiene
  on identity primitive, fingerprint backstop docstring, planning artefact
  references to pre-rename helper, parseShowEnvironmentKeys logging, dead
  return at daemon_lock.go:173-175, positionString simplification, t.Run
  subtest splitting, memo path drift, sentinel constant for orphan count,
  helper consolidation, file-header observation-window comment drift,
  dirNames redundancy, var-anchor removal, fingerprint positive-control
  logging, preamble enumeration, doc.go softening. All are paper-cuts; no
  pattern of systemic doc rot, no user-visible defect.
- Ideas #19-48 — speculative enhancements (additional rationales, captured
  stderr, harmonised log formats, summary INFO lines, regression tests for
  happy paths, injected delays, breadcrumb DEBUG logs, broader AST walks,
  meta-tests, integration tests for real flock races, plan-history notes,
  pre-check hoisting, footnotes, version re-measurement notes, positive log
  markers, ppid checks, table-driven contract tests). No spec deviation, no
  blocking defect, no clustered pattern.
- Bug #51 (T7-5 stale comment in `TestStateDaemon_DoesNotWritePIDFileWhenLockHeld`)
  — test still passes; comment misleads but does not affect behaviour or
  coverage. Below promotion threshold as standalone item.
