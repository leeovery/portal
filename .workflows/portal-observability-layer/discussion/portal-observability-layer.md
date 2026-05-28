# Discussion: Portal Observability Layer

## Context

Portal's logging today is incidental — lines added when someone needed them — not a deliberate observability layer. A real incident on 2026-05-28 (a `hooks.json` wipe at 08:18 BST followed by a saver-disappearance event, with `portal.log` then truncated to 0 bytes at 08:38 BST) destroyed all diagnostic evidence before the symptom could be investigated. The same shape of gap shows up across several unrelated subsystems: silent error paths, missing tick-level summaries, discarded diagnostic context at boundaries, and inconsistent log prefixes that defeat grep-based audit trails.

The seed for this work was a reboot where some Claude `--resume` hooks fired and others didn't, and `portal.log` couldn't tell us which path each helper actually took (`project_reboot_hooks_followup` in MEMORY.md). The investigation surfaced parallel gaps during the cycle-1 review of `slow-open-empty-previews-and-zombie-sessions`. Together these point to a coherent set of patterns to apply consistently across the codebase — not a one-off patch.

The feature also has to cover **log rotation**: the current "rotate to 0 bytes whenever it feels like it" behaviour is the wrong default and was the proximate cause of the 2026-05-28 evidence loss.

### References

- Seed: `.workflows/.inbox/.archived/ideas/2026-05-25--portal-observability-layer.md`
- Memory: `project_reboot_hooks_followup`
- Related: `slow-open-empty-previews-and-zombie-sessions` (cycle-1 review surfaced parallel gaps)

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Subsystem prefix taxonomy [pending]
  Decision-point INFO line shape [pending]
  DEBUG breadcrumb pattern on swallowed errors [pending]
  Diagnostic context preservation at boundaries [pending]
  Cycle-level summary cadence and shape [pending]
  Log-level discipline (DEBUG/INFO/WARN/ERROR contract) [pending]
  Log-level propagation verification [pending]
  Log rotation mechanism [pending]
  Retention policy and audit [pending]
  Saver and daemon lifecycle event taxonomy [pending]
  Hook-firing observability limit (syscall.Exec) [pending]
  Rollout sequencing and scope bundling [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## Summary

### Key Insights

*(populated as discussion progresses)*

### Open Threads

*(populated as discussion progresses)*

### Current State

- Discussion started; map seeded from the inbox idea.
