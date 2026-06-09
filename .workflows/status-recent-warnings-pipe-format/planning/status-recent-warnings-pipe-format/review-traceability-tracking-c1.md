---
status: complete
created: 2026-06-09
cycle: 1
phase: Traceability Review
topic: Status Recent Warnings Pipe Format
---

# Review Tracking: Status Recent Warnings Pipe Format - Traceability

## Result

**CLEAN** — no findings. The plan is a faithful, complete translation of the specification in both directions.

## Direction 1: Specification → Plan (completeness)

Every specification element has plan coverage with implementer-sufficient depth:

| Spec element | Plan coverage |
|---|---|
| Bug / root cause (§Problem & Scope) | Task 3 Problem statement; phase Goal "Why this order" |
| §1 Shared parse helper — `LogLine` struct + `ParseLogLine` contract | Task 1 (struct fields verbatim, return shape) |
| §1 Parsing rules (Time / Level / Component / Message boundary regex `^[A-Za-z_][A-Za-z0-9_.]*=`) | Task 1 Do section, verbatim |
| §1 Message edge cases (later colons preserved, quoted multi-word attr, empty message, no-attrs message, documented assumption) | Task 1 Edge Cases + Tests + Context |
| §1 `ok=false` triggers (no colon, <2 tokens, unparseable timestamp, empty line) | Task 1 Acceptance + Edge Cases |
| §2 Reader migration — remove `logFieldSeparator`/`expectedLogFieldCount`, parse-once, qualifying condition `ok && WARN/ERROR && !Time.Before(cutoff)` | Task 3 Do + Acceptance |
| §2 `LastWarning` composition (component-present / empty-component no stray space / empty-message no trailing space) | Task 3 Do + Acceptance |
| §2 Last-wins positional in append (chronological) order | Task 3 Do + Edge Cases + Context |
| §2 Doc-comment updates (`StatusReport.LastWarning`, `scanRecentWarnings`) + constant hygiene + `logEntryQualifies` fold/rewrite | Task 3 Do + Acceptance |
| §3 Consumer `cmd/state_status.go` — no change | Task 4 Do ("do not modify cmd/state_status.go") + Acceptance |
| §Behaviour unchanged (window, level filter, last-wins, missing-log→zero, best-effort no-error) | Task 3 Do/Acceptance/Edge Cases; phase Acceptance |
| Acceptance criteria 1–7 | Distributed across Task 1/3/4 Acceptance and phase Acceptance |
| Testing — anti-false-green / remove independent format definitions (`writeLogLine` + cmd pipe strings) | Task 3 (writeLogLine removal) + Task 4 (cmd pipe-string removal) |
| Testing — producer-coupled end-to-end regression test at `CollectStatus` layer | Task 3 Do + Tests ("producer-coupled regression") |
| Testing — cmd-layer cases migrated not deleted, suffix updated to trimmed form | Task 4 (both named cases) |
| Testing — producer-coupled seam via same render path as `textHandler.Handle` | Task 2 |
| Testing — retained coverage re-expressed in slog text format (each named case) | Task 3 enumerates every retained test by name |
| Unit coverage for `ParseLogLine` | Task 1 Tests |
| Definition of done | phase Acceptance + Task 3/4 producer-coupled assertions |

Source-file references in the plan were spot-verified against the codebase and are accurate: `status.go` constants at lines 19/23, `StatusReport.LastWarning` doc at 65–66, `scanRecentWarnings` at 181–202, `logEntryQualifies` at 204–221, `handler.go` render block at 132/147–181 with bypass gate at 142–145, `writeLogLine` helper at status_test.go:13–29, malformed-line case at 346–384, both cmd cases at 182–203 / 243–259, `PortalLog` at paths.go:113 and `PortalLogOld` at :116. All eight retained `TestCollectStatus_*` names referenced in Task 3 exist.

## Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every piece of plan content traces to a specific specification section. No invented scope, no hallucinated edge cases, no untraceable acceptance criteria:

- **Task 1** — entirely from §1 "Shared parse helper" and "Unit coverage for `ParseLogLine`."
- **Task 2** — from "Testing requirements → Producer-coupled test seam," which explicitly sanctions "factor the line-building out of `Handle` into an exported render function, or a test-only render helper — `*testing.T`-first." The implementer-discretion note (seam shape / time-parameter shape) is honestly flagged against the spec's own "leaves the seam's exact shape open" language — not invented scope.
- **Task 3** — from §2, Acceptance criteria, Testing requirements, and §Behaviour unchanged. `portal.log.old` ignored is a retained behaviour (existing `TestCollectStatus_DoesNotScanPortalLogOld`), not new scope.
- **Task 4** — from §3 and "Testing requirements → cmd-layer tests." The exact asserted suffix `WARN daemon: flush failed: disk full` (later colon preserved) traces to §1's later-colon rule; the two-space indent is sourced from the existing production-rendered assertion, not invented.

## Findings

None.

## Notes

The plan correctly scopes to the reader-only fix: it never proposes changing the writer/handler (Task 2's extraction is explicitly behaviour-preserving and byte-identical), matching the spec's "the writer/handler is correct and is not changed" out-of-scope boundary.
