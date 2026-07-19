---
topic: cli-verb-surface-redesign
cycle: 2
total_findings: 7
deduplicated_findings: 5
proposed_tasks: 3
---
# Analysis Report: CLI Verb Surface Redesign (Cycle 2)

## Summary
Cycle-2 analysis surfaced 7 findings across the duplication, standards, and architecture passes; after dedup/grouping (5 distinct groups) three actionable, low-risk tasks remain: narrow `portal doctor`'s daemon-liveness probe off the over-scoped `CollectStatus` while sharing one pane counter, remove the production-dead divergent session-glob branch from the resolver, and refresh two stale post-redesign doc/comment artefacts. The stale-classification "mirror," the domain-enum promotion, and the bare-`portal` reclassification are discarded as forced / spec-governed / closed-taxonomy churn in a converging loop.

## Dedup / Grouping Notes
- **A1 (CollectStatus over-scope for the daemon probe) + D2 (doctorPaneCount byte-identical copy)** merged — both are "doctor over-reads state / duplicates a counter the state package owns" and share one fix (narrow the probe, export/share the counter, single index read) → Task 1.
- **D1 (doctor re-authors stale-classification)** is distinct from A1 (different fix) → evaluated on its own → discarded.
- **A2 (dead divergent glob branch)** stands alone → Task 2.
- **A3 (bare-string domain vocabulary)** stands alone → discarded.
- **S1 (bare-`portal` process-role stale comment) + S2 (CLAUDE.md incident-note staleness)** cluster into a "stale post-redesign documentation" pattern → Task 3.

## Discarded Findings
- **Doctor's read-only stale-classification "mirrors" (D1, medium)** — The diagnose path is read-only and structurally cannot invoke the mutating `CleanStale` / `runHookStaleCleanup`; the parallel classification is a deliberately-documented "READ-ONLY mirror," a forced read-only-vs-mutating split rather than an accidental duplicate. Extracting a shared read-only primitive is churn against a forced design boundary in a converging cycle, so it is not proposed.
- **Open-target domain bare-string vocabulary → typed enum (A3, low)** — The domain strings ("session"/"path"/"alias"/"zoxide") match the spec's `resolve` log-taxonomy `domain` attr values; promoting them to a typed enum is churn against a spec-governed vocabulary, and the map↔cobra pairing is already drift-guarded (`TestOpenTargetPinsCoverValueTakingFlags`). The associated `default`-arm exhaustiveness guard and the duplicated command-on-attach guard string are low-severity standalone polish that do not cluster and do not warrant cycle-2 churn.
- **Reclassify bare `portal` from `roleTUI` to `roleBootstrap` (part of S1, low)** — Reclassifying is churn against a closed, forensically-inert process-role taxonomy (bare `portal` returns cobra's `ErrHelp` before `PersistentPreRunE` and emits only `process:` start/exit markers). The classification is deliberately retained; only the stale comment is actioned (Task 3).
