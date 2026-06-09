---
status: complete
created: 2026-06-09
cycle: 2
phase: Traceability Review
topic: Status Recent Warnings Pipe Format
---

# Review Tracking: Status Recent Warnings Pipe Format - Traceability

## Result

**CLEAN** — no findings. The plan remains a faithful, complete translation of the specification in both directions after the cycle-1 integrity refinements were applied.

This cycle re-traced the spec against the plan independently (not relying on cycle-1 memory) and confirmed the 4 integrity refinements landed cleanly without introducing untraceable scope.

## Cycle-1 integrity refinements — verified present and spec-grounded

| Refinement | Where it now lives | Traces to |
|---|---|---|
| `strings.Contains(outBuf.String(), want)` matcher preserved | Task 4 Do (phase-1-tasks.md:211; tick-8ff2fe) | §3 + Testing → cmd-layer tests (multi-line report → substring match); not new scope |
| Seam stamps `RFC3339Nano` (matches `ParseLogLine`) | Task 2 Do (phase-1-tasks.md:92; tick-1c4434) | §1 Time rule (RFC3339Nano) + Producer-coupled seam |
| `log/slog` + `internal/log` import delta named | Task 4 Do (phase-1-tasks.md:214; tick-8ff2fe) | File-level readiness for Testing → cmd-layer tests; no behaviour change |
| `StatusReport.LastWarning` doc comment quoted current→new | Task 3 Do (phase-1-tasks.md:134-144; tick-095fcb) | §2 doc-comment update (verbatim new prose) |

All four are precision/readiness improvements that quote or clarify existing spec-grounded content. None introduce behaviour the spec does not require.

## Direction 1: Specification → Plan (completeness)

Every specification element has plan coverage with implementer-sufficient depth:

| Spec element | Plan coverage |
|---|---|
| Bug + root cause (§Problem & Scope) | Task 3 Problem; phase Goal "Why this order" |
| Blast radius — two warning fields + `isUnhealthy` branch; "not affected" / "only portal.log read" | Task 3/4 Acceptance; phase Acceptance |
| Out of scope — writer unchanged; no window/filter/last-wins/swallow-and-skip change | Task 2 byte-identical extraction; Task 3 preserves; phase Goal |
| §1 `LogLine` struct (Time/Level/Component/Message) | Task 1 Do (struct verbatim) |
| §1 `ParseLogLine(line) (LogLine, ok)` signature | Task 1 Do + Acceptance |
| §1 Parsing rules — Time / Level / Component / Message boundary regex `^[A-Za-z_][A-Za-z0-9_.]*=` | Task 1 Do (verbatim, regex included) |
| §1 Message edge cases (later colons preserved, quoted multi-word attr, empty message, no-attrs whole, documented assumption) | Task 1 Do + Edge Cases + Tests + Context |
| §1 `ok=false` triggers (no colon, <2 tokens, unparseable ts, empty line) | Task 1 Acceptance + Edge Cases |
| §2 Remove `logFieldSeparator`/`expectedLogFieldCount`; parse-once; qualifying `ok && WARN/ERROR && !Time.Before(cutoff)` | Task 3 Do + Acceptance |
| §2 `LastWarning` composition (component-present / empty-component no stray space / empty-message no trailing space) | Task 3 Do + Acceptance |
| §2 Last-wins positional in append (chronological) order | Task 3 Do + Edge Cases + Context |
| §2 Doc-comment updates (`StatusReport.LastWarning`, `scanRecentWarnings`) + constant hygiene + `logEntryQualifies` fold/rewrite | Task 3 Do + Acceptance |
| §3 Consumer `cmd/state_status.go` — no change | Task 4 Do ("do not modify cmd/state_status.go") + Acceptance |
| §Behaviour unchanged (window, level filter, last-wins, missing-log→zero, best-effort no-error) | Task 3 Do/Acceptance/Edge Cases; phase Acceptance |
| Acceptance criteria 1–7 | Distributed across Task 1/3/4 Acceptance + phase Acceptance |
| Testing — anti-false-green / remove independent format definitions (`writeLogLine` + cmd pipe strings) | Task 3 (writeLogLine removal) + Task 4 (cmd pipe-string removal) |
| Testing — producer-coupled end-to-end regression test at `CollectStatus` layer | Task 3 Do + Tests |
| Testing — cmd-layer cases migrated not deleted, suffix updated to trimmed form | Task 4 (both named cases) |
| Testing — producer-coupled seam via same render path as `textHandler.Handle` | Task 2 |
| Testing — retained coverage re-expressed in slog text format (each named case) | Task 3 enumerates every retained `TestCollectStatus_*` by name |
| Unit coverage for `ParseLogLine` | Task 1 Tests |
| Definition of done | phase Acceptance + Task 3/4 producer-coupled assertions |

Source-file references in the plan were re-verified against the live codebase this cycle and check out: `logFieldSeparator`/`expectedLogFieldCount` at status.go:19/23 (used at 208-209); `writeLogLine` at status_test.go:16; malformed-line case `TestCollectStatus_ToleratesMalformedLogEntries` at 346 (the spec's `status_test.go:368` falls inside this function — consistent); both cmd cases at 182 / 243; `PortalLogOld` at paths.go:116. All retained `TestCollectStatus_*` names referenced in Task 3 exist (`_SkipsEntriesOlderThanCutoff`, `_UsesCallerSuppliedNowForWindow`, `_IgnoresInfoAndDebugEntries`, `_CountsWarnAndErrorEntriesInWindow`, `_LastWarningHoldsLastValidEntry`, `_RecentWarningsZeroWhenLogMissing`, `_DoesNotScanPortalLogOld`).

## Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every piece of plan content traces to a specific specification section. No invented scope, no hallucinated edge cases, no untraceable acceptance criteria:

- **Task 1** — entirely from §1 "Shared parse helper" and "Unit coverage for `ParseLogLine`." The boundary regex, three `ok=false` triggers, and every edge case appear verbatim in spec §1.
- **Task 2** — from "Testing requirements → Producer-coupled test seam," which explicitly sanctions factoring the line-building out of `Handle` into a render function / `*testing.T`-first helper. The implementer-discretion note (seam shape / time-parameter shape) is honestly flagged against the spec's own "leaves the seam's exact shape open" language — not invented scope. The RFC3339Nano note traces to §1's Time rule.
- **Task 3** — from §2, Acceptance criteria, Testing requirements, and §Behaviour unchanged. `portal.log.old` ignored is a *retained* existing behaviour (existing `TestCollectStatus_DoesNotScanPortalLogOld`; `PortalLogOld` at paths.go:116), supported by the spec's "Retained coverage" clause and the blast-radius "only portal.log is read" — not new scope.
- **Task 4** — from §3 and "Testing requirements → cmd-layer tests." The exact asserted suffix `WARN daemon: flush failed: disk full` (later colon preserved) traces to §1's later-colon rule and §2's composition; the two-space indent and `strings.Contains` matcher are sourced from the existing production-rendered assertion, not invented. The import delta is file-level readiness, not new behaviour.

## Findings

None.

## Notes

The plan continues to correctly scope to the reader-only fix: it never proposes changing the writer/handler (Task 2's extraction is explicitly behaviour-preserving and byte-identical), matching the spec's "the writer/handler is correct and is not changed" out-of-scope boundary. The cycle-1 integrity refinements did not perturb this fidelity — each refinement clarifies or quotes existing spec-grounded content rather than adding scope.
