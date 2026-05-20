TASK: Collapse shouldKillSaverOnVersionDecision + portalSaverVersionMismatch Into A Single Predicate (3-1)

ACCEPTANCE CRITERIA:
- portalSaverVersionMismatch deleted from internal/tmux/portal_saver.go
- PortalSaverVersionMismatch re-export removed from internal/tmux/export_test.go
- shouldKillSaverOnVersionDecision becomes the sole predicate encoding dev/empty/read-error rules
- Reframed predicate-matrix test covers all 8 rows: dev-stored, dev-current, empty-stored, empty-current, equal non-dev, mismatched non-dev, readErr non-absent, readErr ErrVersionFileAbsent → false
- "byte-equivalent" semantic-equivalence comment removed
- go test ./internal/tmux/... passes

STATUS: Complete

SPEC CONTEXT: Cycle-1 analysis flagged two parallel predicates encoding the same dev/empty/read-error rules, kept in sync by hand with an in-source "byte-equivalent in semantics" admission. Post Change 1/2 work, portalSaverVersionMismatch had zero production callers and survived only via a test-only re-export. The two predicates diverged only on ErrVersionFileAbsent (true vs false). Task collapses to a single source of truth with the absent-file row pinned to false.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/portal_saver.go:325 — call site uses shouldKillSaverOnVersionDecision exclusively
  - internal/tmux/portal_saver.go:342-382 — single predicate, evaluation order documented (dev short-circuits → ErrVersionFileAbsent → non-absent read error → equality)
  - internal/tmux/export_test.go:17-23 — ShouldKillSaverOnVersionDecision re-export (the actively-used one; the dead PortalSaverVersionMismatch re-export from cycle-1 is removed)
  - Grep confirms zero references to PortalSaverVersionMismatch or portalSaverVersionMismatch anywhere in internal/tmux
  - Grep confirms "byte-equivalent" comment is gone
- Notes: Decision matrix doc table at portal_saver.go:286-294 is consistent with the surviving predicate's truth table.

TESTS:
- Status: Adequate
- Coverage: internal/tmux/portal_saver_test.go:1893-2008 — TestShouldKillSaverOnVersionDecision_PredicateMatrix exercises all 8 acceptance rows.
- Notes: Test framing comment (lines 1893-1909) correctly documents the predicate is alive-branch-only and the caller consults BootstrapAliveCheck FIRST. The "absent → false" case carries an explicit comment noting the prior parallel predicate returned true.

CODE QUALITY:
- Project conventions: Followed. Package-level seam pattern preserved.
- SOLID principles: Good. Single Responsibility restored.
- Complexity: Low. Flat conditional dispatch.
- Modern idioms: Yes. errors.Is for sentinel comparison.
- Readability: Good. Doc comment at portal_saver.go:342-357 enumerates exact evaluation order.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] The call site at internal/tmux/portal_saver.go:325 reads `alive && shouldKillSaverOnVersionDecision(...)`. The predicate's own doc comment says "Callers must have already established that the daemon is alive". Renaming to e.g. shouldKillSaverOnVersionDecisionAliveDaemon or moving the alive-gate inside the predicate would make accidental misuse harder. Low priority.
