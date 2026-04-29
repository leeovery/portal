---
agent: standards
cycle: 4
findings_count: 5
---
# Standards Analysis (Cycle 4)

## Summary

Cycle 3 promoted FIFO sweep to a standalone bootstrap step (FIFOSweeper at step 7), making the implementation nine-step, but the spec and CLAUDE.md were never updated to match — they still describe an eight-step sequence with CleanStale at step 7.

---

## Findings

### FINDING: Specification still describes an eight-step bootstrap; implementation is nine-step
- **Severity**: high
- **Files**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md:1046, 1048, 1052, 1161, 1165, 772`, `cmd/bootstrap/bootstrap.go:1, 125`
- **Description**: Cycle 3's architecture recommendation explicitly noted that promoting FIFO sweep to its own step "grows the eight-step contract to nine." `bootstrap.go:1` and `bootstrap.go:125` both say "nine-step PersistentPreRunE sequence" with steps Clear@6, SweepOrphanFIFOs@7, CleanStale@8, Return@9. But the spec was NOT updated:
  - Spec § "Bootstrap Flow (Integrated) → PersistentPreRunE Sequence" (lines 1007–1048) lists exactly **eight** steps with CleanStale at step 7 and Return at step 8 — no FIFOSweeper.
  - Spec line 772 still says: *"This defensive pattern eliminates the need for a separate stale-FIFO sweep step"* — directly contradicting the implemented FIFOSweeper step.
  - Spec § "CleanStale Behavior" lines 1161 and 1165 still call CleanStale "step 7 of `PersistentPreRunE`."
  - Spec line 1048 still says "returns immediately after step 7."
  T9-9 spec realignment commit only touched mkfifo arm-timing and CreateFIFO semantics — it did not propagate the cycle-3 step-count change.
- **Recommendation**: Update the spec to nine-step — insert "**7. SweepOrphanFIFOs**" into the Bootstrap Flow list, renumber CleanStale → 8 / Return → 9, fix lines 1048/1161/1165, and rewrite line 772's "eliminates the need for a separate stale-FIFO sweep step" to acknowledge the bootstrap-step-7 sweep as a defensive complement to per-pane CreateFIFO.

### FINDING: CLAUDE.md documents the bootstrap as eight-step with stale numbering
- **Severity**: high
- **Files**: `CLAUDE.md:66, 67-75`
- **Description**: CLAUDE.md was rewritten in T8-1 (cycle 2) and predates cycle 3's FIFO-sweep promotion. Line 66 still says: *"`PersistentPreRunE` runs an **eight-step** `bootstrap.Orchestrator`."* The numbered list (lines 67-75) ends with `7. **CleanStale**` (line 74) and `8. **Return**` (line 75), with no FIFOSweeper / SweepOrphanFIFOs entry. New contributors are told eight-step while the code says nine-step.
- **Recommendation**: Insert "7. **SweepOrphanFIFOs** — best-effort cleanup of orphan `hydrate-*.fifo` files whose paneKey is no longer represented by a live `@portal-skeleton-*` marker" between the current items 6 and 7; renumber CleanStale → 8 and Return → 9; change "eight-step" on line 66 to "nine-step."

### FINDING: cmd/root.go doc-comment still says "canonical eight-step sequence"
- **Severity**: medium
- **Files**: `cmd/root.go:92`
- **Description**: The `buildBootstrapDeps` doc-comment says: *"In production, builds a fully-wired *bootstrap.Orchestrator that runs the canonical **eight-step** sequence."* The orchestrator's own package doc and type doc both say "nine-step." Two adjacent files in the same import graph contradict each other on the load-bearing step count.
- **Recommendation**: Change "canonical eight-step sequence" → "canonical nine-step sequence" on `cmd/root.go:92`.

### FINDING: phase5_integration_test.go file-level comment says "eight-step bootstrap.Orchestrator"
- **Severity**: medium
- **Files**: `cmd/bootstrap/phase5_integration_test.go:3`
- **Description**: The file's leading comment says *"Phase 5 integration tests exercise the **eight-step** bootstrap.Orchestrator."* Inside the same file, line 248 says "**the nine-step sequence**" and line 346 introduces the FIFOSweeper test as "step 7." The file's own opening claim contradicts what the tests downstream verify.
- **Recommendation**: Update line 3 to "exercise the nine-step bootstrap.Orchestrator."

### FINDING: phase5_integration_test.go marker-probe comments misnumber CleanStale
- **Severity**: low
- **Files**: `cmd/bootstrap/phase5_integration_test.go:39, 74`
- **Description**: The `markerProbeStub` comment at line 39 says: *"absent during step 7 (CleanStale)."* Under the now-canonical nine-step ordering, CleanStale is step 8 (FIFOSweeper is step 7). Same misnumbering at line 74. Tests still pass because they assert on observed marker state (not step indices), but the doc-comments are stale.
- **Recommendation**: Update both occurrences of "step 7 (CleanStale)" to "step 8 (CleanStale)".
