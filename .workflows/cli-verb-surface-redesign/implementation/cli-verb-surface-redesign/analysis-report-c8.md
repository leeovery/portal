---
topic: cli-verb-surface-redesign
cycle: 8
total_findings: 7
deduplicated_findings: 7
proposed_tasks: 4
---
# Analysis Report: CLI Verb Surface Redesign (Cycle 8)

## Summary
Three analysis agents (duplication, standards, architecture) report a strongly-conformant, well-factored implementation: the two-callers-of-one-service spawn core, single-sourced message/log renderers, the `open` precedence chain + burst, `doctor`/`--fix` catalog, `hook` rename, hidden `state`, and the spec-governed `resolve` log component are all in place, with the retired attach/spawn/clean/state-status surfaces cleanly removed. No high-severity findings surfaced. The residual issues are one medium architectural drift (target-domain routing carried by three hand-aligned string vocabularies) plus a handful of lows; several lows are carried forward from cycle 6 and were previously accepted.

Seven findings were reported (2 duplication, 1 standards, 4 architecture). None duplicate across agents — each concerns a distinct site/concern — so deduplication leaves seven. Four are proposed as tasks; three are discarded on the merits below.

## Proposed Tasks
1. **Typed Domain across the cmd↔resolver routing boundary** (medium, architecture) — collapse the two overlapping runtime domain vocabularies into one typed constant set so the routing switches become exhaustive-checkable instead of guard-test-enforced.
2. **Shared exact-session-match helper + unified error handling** (low, architecture) — the exact-session match is authored three times with inconsistent lister-error handling; extract one helper.
3. **Route doctor's Detector/Resolve through the shared spawn-seam bundle** (low, architecture) — doctor hand-rebuilds the seams the anti-drift bundle exists to centralize, becoming a third by-hand-synced copy.
4. **Explicit sentinel at iota 0 for the doctor `checkStatus` enum** (low, standards) — a health-check zero value currently reads as "pass"; add a `checkUnknown` sentinel per the golang-naming convention.

## Discarded Findings
- **doctor's read-only stale diagnosis parallels the prune classification (duplication, low)** — Carried forward from cycle 6 and evidently accepted then; the finding agent itself offers "leave as-is," each block is ~10-15 lines of simple/stable logic, and the fix would churn two pre-existing store files (`hooks.Store.CleanStale` / `project.Store.CleanStale`) for a small divergence-risk payoff. Real but sub-threshold; discarded rather than re-litigated at cycle 8. (Guidance nonetheless applies: do not add a *further* independent copy of either classifier.)
- **`completeSessionNames` / `completeAliasKeys` identical prefix-filter completers (duplication, low)** — A 2-instance repetition sitting explicitly below code-quality.md's Rule-of-Three threshold; the finding agent flagged it as a "clean, low-cost consolidation candidate rather than a must-fix." Consolidating now is premature abstraction by the project's own standard. Discarded; revisit only if a third Portal-owned completion namespace lands (which would make it a genuine Rule-of-Three case).
- **attach-or-mint modeled by two isomorphic sum types with bidirectional converters (architecture, low)** — The `resolver.QueryResult`↔`spawn.Surface` mapping spans a legitimate package boundary (the finding agent grants "a cross-package boundary justifies some mapping"), the round trip occurs only in the single-surviving-surface degenerate case, and there is no correctness inconsistency — it is a design-taste concern. The remedy is a cross-package refactor with low payoff; discarded.
