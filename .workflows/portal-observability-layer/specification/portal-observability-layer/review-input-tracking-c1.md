---
status: in-progress
created: 2026-05-31
cycle: 1
phase: Input Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Input Review

## Findings

### 1. Lost `natural_churn` / `entries_failed` count definitions in cycle-summary

**Source**: discussion `## Cycle-level summary cadence and shape` → Mechanical rule, line 935 ("Additional counts ... Examples: `natural_churn` (sessions that ended cleanly mid-capture), `entries_failed` (per-item failures), `warnings`.")
**Category**: Enhancement to existing topic
**Affects**: *Cycle-level summary cadence and shape* (the "Where:" bullet list, spec lines 796-800) — and, secondarily, the *Closed attr-key value space* cycle-summary list (spec line 181)

**Details**:
The discussion gives a one-line definition for `natural_churn` ("sessions that ended cleanly mid-capture") and `entries_failed` ("per-item failures") at the exact place that enumerates the cycle-summary sub-category counts. The spec's cycle-summary "Where:" bullets collapse this to a generic "Additional sub-categorisation counts ride as attrs on the same summary line" and the closed-attr list defines only `step` / `windows` / `skipped` parenthetically — `natural_churn` is left undefined everywhere in the spec even though it is the headline attr on `capture: tick complete`, the single most-emitted production INFO line. An implementer reading the spec has to guess what `natural_churn` measures vs `anomalous`; the discussion answered that ("ended cleanly mid-capture" = a session that disappeared by normal user action during the tick, distinct from an anomalous capture failure). That distinction is exactly the forensic signal the capture summary exists to provide, so the definition is load-bearing, not cosmetic.

**Current**:
Spec lines 796-800:
```
Where:
- `<verb>` is the cycle's purpose phrase: `tick`, `sweep`, `step`, `phase`, `orchestration`, `replay`, etc.
- `<unit>` is the item being iterated: `sessions`, `panes`, `entries`, `orphans`, `steps`, `files`, etc.
- Additional sub-categorisation counts ride as attrs on the same summary line.
- `took` is always present.
```
Spec line 181 (closed-attr cycle-summary group) defines `step` / `windows` / `skipped` parenthetically but not `natural_churn` or `entries_failed`.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:

---

### 2. Concrete inbox gap-closure instrumentation sites not present in spec body

**Source**: discussion `## Diagnostic context preservation at boundaries` (motivating example) + the inbox-seed gap closures enumerated in the rollout subtopic (line 1208: "`defaultIdentifyPS` stderr capture, `escalateKillToSIGKILL` DEBUG breadcrumb, `ShowGlobalHooks` failure-log asymmetry, defensive-branch 'why this branch exists' comments")
**Category**: Gap/Ambiguity
**Affects**: *Diagnostic context preservation at boundaries* and/or *Log-level discipline* (these are the mechanical rules that govern the named sites)

**Details**:
Four concrete, pre-identified instrumentation defects were called out in the source as specific closures this feature must land: (1) `defaultIdentifyPS` discarding stderr, (2) `escalateKillToSIGKILL` missing a DEBUG breadcrumb, (3) the `ShowGlobalHooks` failure-log asymmetry, and (4) defensive branches lacking "why this branch exists" context. Only #1 survives into the spec body (named in the boundary-preservation Decision). #2/#3/#4 appear ONLY inside the deliberately-excluded rollout-sequencing subtopic's PR-2 task surface — so under the agreed omission they have no home in the spec at all.

These are not pure delivery-sequencing concerns; they are named, decided, existing-code gaps that the inbox seed specifically pointed at. The spec instruments by mechanical rule rather than by enumerated site, so the argument that "the boundary-class-1 rule and the swallowed-error level table will catch these anyway" is plausible — but `escalateKillToSIGKILL` (a DEBUG breadcrumb on an escalation path) and `ShowGlobalHooks` (a log-asymmetry fix, where one branch logs and the sibling doesn't) are precisely the kind of site a purely-mechanical pass can skip because nothing about the code shape forces a new log call. Surfacing them here so the spec can decide whether to (a) name them explicitly as worked examples under the relevant mechanical rule, or (b) confirm the mechanical rules subsume them and that the named-site list is intentionally a planning-only artifact.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**: #1 (`defaultIdentifyPS`) is already captured in the boundary-preservation section and is NOT part of this finding — only #2/#3/#4 are at issue.

---
