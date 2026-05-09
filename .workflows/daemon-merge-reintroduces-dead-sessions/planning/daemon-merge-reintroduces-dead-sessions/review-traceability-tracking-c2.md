---
status: complete
created: 2026-05-09
cycle: 2
phase: Traceability Review
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: Daemon Merge Reintroduces Dead Sessions - Traceability

## Summary

Bidirectional traceability review (cycle 2 — follow-up after cycle 1 integrity finding was applied to task 2-1's Tests section). **Zero findings.** The cycle 1 polish preserved traceability in both directions; the rest of the plan is unchanged from cycle 1's clean baseline.

## Cycle 2 Focal Check — Task 2-1 Tests Polish

The cycle 1 integrity finding rewrote the first bullet of task 2-1's `Tests` section from a stream-of-consciousness draft (`"...given marker {x} and zero live panes... wait, this overlaps with task 2-3's zero-panes guard. Use markers..."`) into a clean declarative description (`"given markers {stale__0.0, live__0.0} and live panes {live:0.0} (non-empty live set so the zero-panes guard from task 2-3 does not short-circuit), assert exactly one unset call for @portal-skeleton-stale__0.0 and zero unset calls for @portal-skeleton-live__0.0"`).

**Spec → Plan trace of polished bullet:**

| Polished element | Spec anchor |
|---|---|
| Stale-marker unset assertion (`stale__0.0` ∉ live → one unset of `@portal-skeleton-stale__0.0`) | §Fix Component B → Behavior ("Compute the set difference: markers whose paneKey is not present in the live pane set. For each stale marker, unset it"); §Testing Requirements → Stale-marker cleanup unit ("Given a marker whose paneKey doesn't correspond to a live pane, the cleanup unsets it") |
| Live-marker preservation assertion (`live__0.0` ∈ live → zero unsets) | §Testing Requirements → Stale-marker cleanup unit ("Given a live marker [...] the cleanup leaves it alone") |
| Non-empty live set framing (avoiding short-circuit by zero-panes guard) | §Soft-Warning Posture → Mass-unset hazard guard (the guard is task 2-3's surface; this test deliberately exercises the unset path with a non-empty live set) |
| Mixed paneKey forms (`stale__0.0` canonical, `live:0.0` tmux format) | §Adapter Wiring → PaneKey conversion ("`session:window.pane` [...] convert each entry to canonical paneKey form via `state.SanitizePaneKey`") |
| Option name composition `@portal-skeleton-<paneKey>` | §Adapter Wiring → Marker unset ("the full option name `@portal-skeleton-<paneKey>` (i.e. the `SkeletonMarkerPrefix` constant followed by the canonical paneKey)") |

The polish dropped no spec-anchored content and added no spec-unanchored content. It only removed authoring meta-commentary ("...wait, this overlaps...") that the cycle 1 integrity review correctly flagged as draft prose. The fixture (`{stale__0.0, live__0.0}` markers, `{live:0.0}` live panes) and the assertion shape (one unset, zero unsets) are unchanged from the original draft's *intended* test — the polish made the intent declarative rather than narrated.

**Plan → Spec trace** of the surrounding task 2-1 content (Problem, Solution, Outcome, Do, Acceptance Criteria, remaining three Tests bullets, Edge Cases, Context) is identical to cycle 1; no further drift.

## Direction 1 — Specification → Plan (completeness)

Carry-forward from cycle 1 with no changes:

- **Fix Component A — Live-Set Filtering** — session/window/pane levels covered by tasks 1-1 / 1-2 / 1-3; helpers untouched (single point of enforcement) reflected in ACs across 1-1, 1-2, 1-3; Self-Healing two-tick sequence in task 1-4; Preserved Behavior (hydrate-in-progress) in task 1-5.
- **Fix Component B — Stale-Marker Cleanup** — Behavior in task 2-1; Soft-Warning Posture across 2-3 / 2-4; Mass-unset hazard guard in 2-3; Concurrency (no serialisation) in 2-5 ACs; Synergy with SweepOrphanFIFOs (insertion between step 6 and step 7) in 2-5; Adapter Wiring (marker enumeration / error-propagating live-pane call / marker unset) in 2-6 + 2-1; PaneKey conversion + Parse contract in 2-2 + 2-1; Why This Step Is Needed (scrollback-save resumption) in 2-7.
- **Testing Requirements** — `capture_test.go:570-617` replacement in task 1-1; structural-level tests in 1-2 / 1-3; empirical-scenario regression in 1-4; cleanup unit (stale unset / live preserved) in 2-1; PaneKey normalisation correctness fixture in 2-2; bootstrap integration (insertion point + soft-warning degradation) in 2-5; tests-to-preserve in 1-5.
- **Acceptance Criteria #1-#8** — full coverage map carried forward from cycle 1 (AC#1 → 1-4, AC#2 → 1-4, AC#3 → 1-4, AC#4 → Phase 2 + 2-5, AC#5 → 1-1/1-2/1-3/1-4, AC#6 → 1-5, AC#7 → Phase 1/2 acceptance, AC#8 → 2-7).
- **Files Touched** — all paths from §Scope → Files Touched are referenced across tasks 1-1 through 2-7.

## Direction 2 — Plan → Specification (fidelity)

Carry-forward from cycle 1 with no changes. The cycle 1 cross-walk verified every task's Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context against specific spec anchors (table reproduced in cycle 1's tracking file). The cycle 2 integrity polish modified prose in one Tests bullet only, did not introduce any new content, and the polished bullet still anchors to the same spec sections (§Fix Component B → Behavior, §Testing Requirements → Stale-marker cleanup unit, §Adapter Wiring → PaneKey conversion + Marker unset).

No hallucinated content found.

## Findings

(none)

## Conclusion

**Status: clean.** The cycle 1 integrity polish to task 2-1's Tests section did not introduce traceability drift. The plan remains a faithful translation of the specification in both directions; every spec element has plan coverage with adequate depth, and every plan element traces back to the specification with no hallucinated content.
