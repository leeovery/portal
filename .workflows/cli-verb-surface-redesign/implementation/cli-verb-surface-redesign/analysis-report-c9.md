---
topic: cli-verb-surface-redesign
cycle: 9
total_findings: 5
deduplicated_findings: 4
proposed_tasks: 1
---
# Analysis Report: CLI Verb-Surface Redesign (Cycle 9)

## Summary
Standards found the surface fully conformant (clean). The one substantive, cross-agent finding is that `doctor`'s strictly read-only stale-hook/stale-project diagnosis re-derives the same ∉-set / `os.Stat` staleness classification the stores own on their prune path, so diagnose and repair agree only by hand-mirroring — a data-loss-safety-adjacent drift risk raised independently by both the duplication and architecture agents. Three remaining low-severity nits (a duplicated completion loop, a single-use `tuiConfig` 1:1 mirror with an inaccurate harness-parity doc claim, and an open-burst detector that isn't sourced from the shared spawn-seams bundle) are minor, unrelated, and non-clustering; they are discarded this cycle.

## Recurring finding triage (doctor stale-diagnosis duplication)
This finding was raised in c6, discarded by synthesis in c8, and is now raised again in c9 by two independent agents. Weighing recurrence against the c8 discard rationale honestly:
- **What changed vs c8.** The c8 discard rested partly on the finding agent itself offering "leave as-is" and a judgment that the payoff was small. This cycle NEITHER agent offered leave-as-is: the duplication agent calls it "the one substantive residual duplication" and the architecture agent "the one structural concern worth fixing." Two independent agents corroborating, with no leave-as-is escape hatch, is a genuine signal — not merely a re-report.
- **Merits, not recurrence.** The diagnosis code (`checkStaleHooks`/`countStaleHookKeys`/`checkStaleProjects`) is freshly authored *for this very topic* (doctor subsumes `state status` and replaces `clean` via `--fix`), so it is squarely in scope. The hazard guard's whole reason for existing is data-loss safety (never wipe non-reconstructable user on-resume hooks on a down server); a silent drift where `doctor` reports green while `--fix` prunes has real stakes. There is a clean, low-churn extraction that makes diagnose and repair provably consistent by having the store own the staleness predicate — this reduces the drift risk rather than just churning two files.
- **Scoped to genuine drift reduction.** The proposed task extracts ONLY the ∉ / `os.Stat` staleness classification into a store-owned predicate that both `CleanStale` and doctor's diagnosis derive from. It deliberately leaves the mass-deletion hazard guard where it lives (`checkStaleHooks` + `runHookStaleCleanup`) — that is a cmd-layer repair-safety policy, not a store predicate, and consolidating it would add coupling/churn without touching the core drift risk. This keeps the fix aligned with the triage bar: reduce diagnose-vs-repair drift, do not churn.

Net: proposed on merits (medium severity, two-agent corroboration, in-scope fresh code, real safety stakes, clean store-owned fix), not auto-proposed for recurrence.

## Discarded Findings
- Byte-identical prefix-match completion loops (`completeSessionNames`/`completeAliasKeys`, low) — an ~8-line copy-paste with a trivial helper extraction; both slots were introduced together this feature (low live drift risk). Low severity, does not cluster with the other lows into a single actionable pattern-task. Discarded for a mature (cycle 9) converging topic.
- Single-use `tuiConfig` mirrors `tui.Deps` 1:1 with an inaccurate shared-harness doc claim and no parity guard (low) — genuine (the doc comment claims a capture-harness parity contract that does not exist, and a `tui.Deps` field can silently zero-value through the hand-copy), but architecture-assessed low, in a different subsystem from the other lows, non-clustering. Worth a maintainer's correction opportunistically (fix the doc to name `tui.Build` as the true chokepoint, or add a field-parity guard) but not actionable-enough to propose this cycle.
- Open-burst detector construction bypasses the single-source spawn-seams bundle (low) — behaviour is identical today (both paths wrap the same client); the defect is that the bundle's anti-drift doc claim overstates single-sourcing for the detector. Low severity, distinct subsystem, non-clustering. Discarded; a future touch of `buildOpenBurstDeps`/`buildProductionSpawnSeams` can either default `Detector` from `sharedSeams()` or narrow the bundle doc.
