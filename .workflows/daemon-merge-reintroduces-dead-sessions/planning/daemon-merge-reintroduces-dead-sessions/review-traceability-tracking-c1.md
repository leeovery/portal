---
status: complete
created: 2026-05-09
cycle: 1
phase: Traceability Review
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: Daemon Merge Reintroduces Dead Sessions - Traceability

## Summary

Bidirectional traceability review (cycle 1) found **zero findings**. The plan is a faithful, complete translation of the specification.

### Direction 1 — Specification → Plan (completeness)

Every spec element has plan coverage at adequate depth:

- **Fix Component A — Live-Set Filtering**:
  - Session-level filter → task 1-1
  - Window-level filter → task 1-2
  - Pane-level filter → task 1-3
  - Helpers untouched (single point of enforcement) → ACs across 1-1, 1-2, 1-3
  - Self-Healing Behavior (two-tick sequence) → task 1-4
  - Preserved Behavior (hydrate-in-progress) → task 1-5
  - Rejected Alternatives → contextual; no task needed (spec-rejected paths)
- **Fix Component B — Stale-Marker Cleanup**:
  - Behavior (enumerate / diff / unset) → task 2-1
  - Soft-Warning Posture → tasks 2-3 (zero-panes / enum-error guard) and 2-4 (per-marker unset / malformed-line skip)
  - Mass-unset hazard guard → task 2-3
  - Concurrency with the daemon (no serialisation) → task 2-5 ACs
  - Synergy with SweepOrphanFIFOs (insertion between step 6 and step 7) → task 2-5
  - Adapter Wiring (marker enumeration, error-propagating live-pane call, marker unset) → task 2-6 + parts of 2-1
  - PaneKey conversion + Parse contract → tasks 2-2 and 2-1
  - Why This Step Is Needed (scrollback-save resumption) → task 2-7
- **Testing Requirements**:
  - Existing test to replace at `capture_test.go:570-617` → task 1-1
  - Window-level / pane-level filter tests → tasks 1-2 / 1-3
  - Empirical-scenario regression → task 1-4
  - Cleanup unit (stale unset / live preserved) → task 2-1
  - PaneKey normalisation correctness fixture → task 2-2
  - Bootstrap integration (right insertion point + soft-warning degradation) → task 2-5
  - Tests to preserve in `internal/restore/session_markers_test.go` → task 1-5
- **Acceptance Criteria #1-#8**:
  - AC#1 (synthetic repro + prev-population precondition) → task 1-4 + Phase 1 acceptance bullet
  - AC#2 (empirical scenario) → task 1-4
  - AC#3 (self-heal on next tick) → task 1-4 tick 2 + Phase 1 acceptance
  - AC#4 (no stale marker post-bootstrap absent soft warning) → Phase 2 acceptance + task 2-5
  - AC#5 (filter prevents resurrection while marker exists pre-cleanup) → tasks 1-1, 1-2, 1-3, 1-4
  - AC#6 (hydrate-in-progress flow) → task 1-5
  - AC#7 (tests pass) → Phase 1 / Phase 2 acceptance
  - AC#8 (scrollback-save resumption) → task 2-7
- **Files Touched** (`internal/state/capture.go`, `capture_test.go`, `cmd/bootstrap/`, `internal/bootstrapadapter/`, `cmd/bootstrap/bootstrap_test.go`, `internal/bootstrapadapter/adapters_test.go`) — all referenced across tasks 1-1 through 2-7.

### Direction 2 — Plan → Specification (fidelity)

Every task element traces back to specification content:

| Task | Primary spec anchors |
|------|----------------------|
| 1-1 | Fix Component A → Filtering Levels (session); Data Flow / Signature Approach; Testing Requirements → Existing Tests to Replace |
| 1-2 | Fix Component A → Filtering Levels (window); Data Flow / Signature Approach |
| 1-3 | Fix Component A → Filtering Levels (pane); Data Flow / Signature Approach |
| 1-4 | Empirical Confirmation; Self-Healing Behavior; Acceptance Criteria #1, #2, #3 |
| 1-5 | Fix Component A → Preserved Behavior; Tests to Preserve; Acceptance Criteria #6 |
| 2-1 | Fix Component B → Behavior; Adapter Wiring |
| 2-2 | Fix Component B → Adapter Wiring → PaneKey conversion / Parse contract; Testing Requirements → PaneKey normalisation correctness |
| 2-3 | Fix Component B → Soft-Warning Posture → Mass-unset hazard guard; Adapter Wiring (error-propagating call rationale) |
| 2-4 | Fix Component B → Soft-Warning Posture; Adapter Wiring → Parse contract |
| 2-5 | Fix Component B → Location; Concurrency with the Daemon; Synergy with SweepOrphanFIFOs |
| 2-6 | Fix Component B → Adapter Wiring (Marker enumeration / Live pane enumeration / Marker unset) |
| 2-7 | Why This Step Is Needed; Acceptance Criteria #8; Out of Scope (marker production path unchanged) |

No content was found that lacks a specification anchor. Notable items examined for possible hallucination and confirmed traceable:

- Task 2-4 references to `*FatalError` and `errors.As` — derived from the spec's "never escalates to a fatal abort" requirement combined with "consistent with existing bootstrap conventions" deference; not invented.
- Task 2-6 live-tmux smoke tests — extend (do not replace) the spec-required co-located unit tests, mirroring the existing `FIFOSweeper` adapter pattern that the spec explicitly invokes ("mirroring the existing `FIFOSweeper` adapter test shape").
- Task 2-7 negative-control variant — derives from the spec's load-bearing-precondition pattern (Acceptance Criteria #1's risk-if-skipped framing) and the explicit "verifies the secondary harm closed by Fix Component B" requirement in AC#8.
- Task 2-5 ten-step sequence enumeration — verbatim from the spec's Location section.
- Task 2-3's "true no-op startup is a no-op with no warning needed" branch — derived from the spec's "warning is the user-visible signal" framing in AC#4 (warnings are only useful when there is something at risk).

## Findings

(none)

## Conclusion

**Status: clean.** No findings. The plan is a faithful translation of the specification in both directions; every spec element has plan coverage with adequate depth, and every plan element traces back to the specification with no hallucinated content.
