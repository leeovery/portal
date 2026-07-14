---
topic: restore-host-terminal-windows
cycle: 3
total_findings: 11
deduplicated_findings: 10
proposed_tasks: 8
---
# Analysis Report: restore-host-terminal-windows (Cycle 3)

## Summary
Cycle 3 shows clear convergence: the Phase-7/8 consolidations landed (spawn log-emission, message renderers, count-semantics, exec-boundary, nanoid alphabet are all single-sourced), so no high-severity findings remain and the residue is smaller and more local than prior cycles. The eleven raw findings dedupe to eight proposed tasks — two medium (a fresh TUI-presentation copy-paste cluster: the eight-fold section-header line-0 splice; and four dead burst-outcome fields on the god-object Model) and six low (left-bar glyph-column and footer narrow-degrade duplication, the Outcome zero-value sentinel, the burstAllConfirmed chokepoint derivation, and two observability/help-text nits). Three findings were discarded as already-decided or already-addressed.

## Deduplication & Grouping
- The section-header line-0 splice duplication was found by the **duplication** agent (eight verbatim copies of the `IndexByte`+guard+splice) and independently by the **architecture** agent while reviewing the sessions-page banner stack ("each repeating the identical … splice"). Merged into one task (Task 1), sourced from both.
- The architecture agent's "orchestration sequence reimplemented" (medium) and "burstAllConfirmed parallel loop" (low) findings share one concrete, not-already-shipped residual — the picker's success gate should rest on the shared `PartitionResults` chokepoint, verified by a cross-path parity test. That residual is captured in Task 6; the broader orchestration-sequence refactor is discarded (see below).

## Discarded Findings
- **Permission-path per-window DEBUG asymmetry (standards, low)** — The CLI emits per-window DEBUGs + the permission INFO while the picker's permission arm emits ONLY the permission INFO. This asymmetry is INTENTIONAL and was explicitly preserved in cycle 1 task 1 and cycle 2 task 1; the divergence is documented in-source and the spec does not mandate per-window DEBUG on the permission path. Not a defect — a decided design.
- **Sessions-page banner precedence scattered across two functions / replace-vs-co-render table (architecture, medium)** — The finding's core recommendation is to centralize the notice-slot precedence and encode the "spawn-failure/permission flash CO-RENDERS with the N-selected banner" decision as data. That two-row co-render was CONFIRMED INTENTIONAL in cycle 1 task 6 and is documented at the precedence seam with an explicit "do not collapse to single row" note. Only the finding's incidental observation of repeated line-0 splice surgery is actionable, and that is folded into Task 1.
- **Spawn-burst orchestration SEQUENCE reimplemented per caller (architecture, medium)** — The finding's evidence of prior drift (§8-3 adding preflight to decideBurst) is already resolved by cycle 2 task 3 (preflight-before-unsupported unified on both paths), and its classification-hoist recommendation is realized by cycle 1 task 1 (`PartitionResults`/`FirstPermission`/`Confirmed` both callers now derive from). The **duplication** agent independently reviewed the same parallel CLI/TUI orchestration and left it as-is — "genuine sync-vs-async control-flow, not copy-pasted logic." The one concrete, not-yet-shipped residual (derive the picker's success gate from the chokepoint + a cross-path parity test) is captured in Task 6; forcing a single shared orchestrator across the sync/async split is impractical and was not proposed.
