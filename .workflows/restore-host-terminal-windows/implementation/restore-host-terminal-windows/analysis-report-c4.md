---
topic: restore-host-terminal-windows
cycle: 4
total_findings: 6
deduplicated_findings: 6
proposed_tasks: 3
---
# Analysis Report: restore-host-terminal-windows (Cycle 4)

## Summary
Cycle 4 confirms strong convergence: no high-severity findings, and the spawn/burst core is already single-sourced by the prior three cycles (classify/count chokepoint, message renderers, log-emission helpers, exec-boundary, footer fitter, left-bar glyph column, section-header splice, burst-outcome chokepoint). Six raw findings dedupe to six (no cross-agent overlap this cycle) and normalize to three proposed tasks — two medium (the architecture agent's re-resolve nil-panic, assessed as a genuine reachable defect on the config-script + mid-session-deletion path; and the duplication agent's seven-seam spawn-defaults copy-paste between the CLI and picker) and one low (centralizing the net-N split behind a shared `spawn.SplitNetN`). Three findings were discarded: two already-decided intentional designs re-litigated by the standards agent, and one low duplication whose own author recommends leaving it.

## Deduplication & Grouping
- No cross-agent duplicates this cycle — the three agents surfaced disjoint findings, so all six map one-to-one (three retained, three discarded).
- The architecture "model caches Resolution but not the Adapter" finding was code-verified end to end (model.go:2456 discards the adapter → burst_progress.go:464 re-resolves → configadapter.go:116 live `os.Stat` makes the second resolve non-deterministic → burst.go:162 `Adapter.OpenWindow` on a nil interface inside the un-recovered `go func()` at burst_progress.go:105). Confirmed REAL and reachable (config-script recipe + mid-session script deletion/chmod between page-entry detection and Enter), not merely theoretical — the assessment is recorded in Task 1's Problem section for the orchestrator to weigh.

## Discarded Findings
- **Spawn-failure/permission flash co-renders two rows vs the spec's "single slot, highest wins" (standards, low)** — Re-litigation of an already-decided design. The two-row co-render (the §11 ▌-band + the section-header row, with the spawn-failure/permission flash deliberately co-rendering with the `N selected` banner) was CONFIRMED INTENTIONAL in cycle 1 and is documented with a tripwire note at the precedence seam (notice_band.go / model.go). The finding itself acknowledges it is "documented as a deliberate decision" and needs "no code change if the recorded decision is ratified." Not a defect.
- **Picker permission path omits the per-window DEBUG the CLI emits (standards, low)** — Re-litigation of an already-decided design. The CLI-vs-picker per-window DEBUG asymmetry on the permission path is INTENTIONAL and was explicitly preserved across cycles 1 and 2 (the picker's permission arm emits ONLY the permission INFO by design; the omission is documented in-source). The spec does not mandate per-window DEBUG on the permission path. Not a defect.
- **Repeated multi-select suppression guard across four row-action dispatch arms (duplication, low)** — Low severity, does not cluster into a broader pattern, and the finding's OWN recommendation is to leave it as-is: the four `if m.multiSelectMode { return m, nil }` guards each carry a distinct per-key rationale comment and are deliberately kept present for `keymap_dispatch_guard_test.go`'s default-mode parity probe. Consolidating behind a single predicate would displace the per-key documentation for negative net value; the author advises acting only if a fifth suppressed row action is ever added. Discarded as a non-actionable cosmetic note.
