---
status: in-progress
created: 2026-07-02
cycle: 1
phase: Traceability Review
topic: Skip Bootstrap When Warm
---

# Review Tracking: Skip Bootstrap When Warm - Traceability

## Summary

Both directions checked against the specification.

**Direction 1 (Spec → Plan, completeness):** Every specification element has plan
coverage with adequate implementer-level depth. The three phases partition the spec's
three pillars cleanly — the version-stamped latch + set-point + CleanStale removal
(Phase 1), the entry-path three-way branch + abridged liveness-only path (Phase 2), and
the daemon-owned hooks cleanup (Phase 3). Every decision, edge case, invalidation mode,
constraint, and test-strategy item maps to a task or is correctly handled as
non-actionable context. Notably, the spec's "accepted residues" (flapping daemon,
cold-boot cleanup leftovers, daemon-death vs cleanup home) correctly generated **no**
tasks — they are tolerated per spec, and the plan does not invent handling for them. The
"out of scope" directives (no `portal`-level unset command; the manual escape hatch is
raw `tmux set-option -u`) are correctly honoured — the plan adds no unset surface.

**Direction 2 (Plan → Spec, fidelity):** Every task's Problem, Solution, implementation
detail, acceptance criteria, tests, and edge cases trace to a specific spec section
(each task carries an explicit Spec Reference plus inline `Spec §` quotes). One single
minor piece of plan content lacks a spec anchor — an optional DEBUG log line introduced
in Task 2-1 — surfaced below as Finding 1. It introduces no new taxonomy and is framed
as best-effort, so it is low-severity, but per the anti-hallucination standard it is
flagged for explicit user disposition rather than silently justified.

## Findings

### 1. Optional DEBUG log line in the abridged saver has no spec anchor

**Type**: Hallucinated content
**Spec Reference**: N/A (spec §"Abridged EnsureSaver — Liveness-Only" describes the transient-error-as-absent fold with no logging prescription; the closed logging taxonomy lives in the codebase logging spec, not this feature spec)
**Plan Reference**: Phase 2, Task skip-bootstrap-when-warm-2-1 (`Do` section, third bullet)
**Change Type**: remove-from-task

**Details**:
The spec's §"Abridged EnsureSaver — Liveness-Only" folds a `SaverPanePIDOrAbsent`
transient probe error into "attempt revive" (bias toward reviving on an unreadable
probe). It prescribes **no logging** for this case — the only spec-anchored emission on
this path is the `SaverDownWarning` funnelled into `bootstrapWarnings` when a revive
fails. Task 2-1's `Do` section adds an instruction to emit a DEBUG line for the
transient-error-as-absent case:

> "Log the transient-error-as-absent case at DEBUG under the bootstrap component if a
> logger is readily available (best-effort; do not add a new component or attr)."

This is plan-introduced content with no corresponding spec section. It is low-severity —
it reuses the existing `bootstrap` component and explicitly forbids new component/attr
vocabulary, and it is framed as optional ("if a logger is readily available",
"best-effort") — so it neither expands scope nor violates the closed logging taxonomy.
But it is not traceable to the specification, so per the anti-hallucination standard it
is flagged for the user to either approve as an intentional diagnostic addition or
remove. The recommended default is removal (the abridged helper takes no logger
parameter in its pinned signature `ensureSaverLiveness(client *tmux.Client, stateDir
string)`, so "if a logger is readily available" is likely never satisfiable at the call
site anyway, making the instruction dead guidance).

**Current**:
```
- Body: call `_, present, err := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)`. Treat `present == true && err == nil` as alive → return immediately (no revive, no warning). Treat every other shape — `!present` (absent) OR a non-nil `err` (transient probe error) — as "needs revive" (per spec §Abridged EnsureSaver: a transient error folds into "attempt revive"; mirror Component D's "treat any error as absent" collapse for the liveness case). Log the transient-error-as-absent case at DEBUG under the bootstrap component if a logger is readily available (best-effort; do not add a new component or attr).
```

**Proposed**:
```
- Body: call `_, present, err := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)`. Treat `present == true && err == nil` as alive → return immediately (no revive, no warning). Treat every other shape — `!present` (absent) OR a non-nil `err` (transient probe error) — as "needs revive" (per spec §Abridged EnsureSaver: a transient error folds into "attempt revive"; mirror Component D's "treat any error as absent" collapse for the liveness case).
```

**Resolution**: Pending
**Notes**: Sole non-traceable content in the entire plan. If the user prefers to keep a diagnostic breadcrumb, it should be approved as an intentional addition (and the helper signature would then need a logger argument for the "if a logger is readily available" clause to be reachable — currently it is not). Otherwise remove per the Proposed text.

---
