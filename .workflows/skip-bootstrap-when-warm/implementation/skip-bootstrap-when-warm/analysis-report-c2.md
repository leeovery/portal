---
topic: skip-bootstrap-when-warm
cycle: 2
total_findings: 6
deduplicated_findings: 6
proposed_tasks: 2
---
# Analysis Report: Skip Bootstrap When Warm (Cycle 2)

## Summary
Cycle 2 surfaced 6 findings across three agents (3 duplication, 1 standards, 2 architecture) with no overlap, so all 6 deduplicate to 6 distinct findings. There are no high-severity findings; the feature is fully implemented, task-by-task reviewed, and green. Two findings clearly earn a follow-up task: the medium-severity `runHookStaleCleanup` over-parameterisation (a production-dead code branch plus a contract doc that points at removed code and omits its live daemon caller) and the low-but-real observability gap where the abridged saver revive swallows its underlying error with no portal.log breadcrumb. The remaining four (two low test-scaffold duplications, one low production-epilogue duplication below the Rule-of-Three threshold, and one cosmetic seam-naming smell) are acceptable as-is on green, reviewed code and are recorded below as watch-items.

## Discarded Findings
- Verbatim-duplicated integration-test env scaffolding (`setupAbridgedEnv` ≡ `setupConcurrentColdBootEnv`) — low, duplication. Test-only scaffold at exactly two instances (below the strict Rule-of-Three threshold the file's own docstring cites as the reason for deferral); the two copies differ only in a socket-prefix string literal, and the load-bearing reap/isolation helpers are already shared. Documented watch-item — consolidate the moment a third caller appears; not worth a task on green code.
- Entry-path context-injection + warning-drain epilogue duplicated across `PersistentPreRunE` branches (cmd/root.go) — low, duplication. Production critical-path, but only two sites of a few lines each (below Rule of Three), differing only in the `serverStarted` value and a defensive nil guard. Raised in cycle 1 and consciously accepted; the divergence risk is bounded and reviewer-visible. Watch-item, not a task at this stage.
- Repeated inline `openTUIFunc`-capture closure across the latch-routing tests — low, duplication. Test-only scaffold; although it exceeds the Rule-of-Three count, it is a test-helper convenience whose only drift risk (an `openTUIFunc` signature change) is caught by the compiler. Low value on green code; discard.
- `RestoringChecker` seam name/doc still describe only the restoring marker (internal/state/markers.go) — low, architecture. A cosmetic seam-naming/doc-clarity smell with no correctness impact; the latch read composes correctly. Cosmetic naming observations are not worth a task at this stage; discard (a one-line doc extension is optional cleanup, not a tracked follow-up).
