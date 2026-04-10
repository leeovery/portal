# Discussion: Built-in Session Resurrection

## Context

Portal should own the full session lifecycle: server start → session restoration → resume hook execution. Currently the middle step depends on tmux-resurrect/continuum, which has a 100% failure rate — sessions never come back after reboot. The resume hook feature is effectively broken end-to-end despite the code being correct, because the session structure it depends on doesn't exist.

Research has confirmed full technical feasibility. tmux provides all the APIs needed for capture (`list-panes -a -F`) and restore (`new-session`, `split-window`, `select-layout`). The question is no longer *can we do this* but *how should we design it*.

Key design principles established in research:
- Portal's hook system is generic — no awareness of what consumers do with it
- Portal doesn't maintain a separate session registry — reads tmux directly
- Portal captures all sessions (Portal-created and native tmux), consistent with existing behavior
- Portal is always the entry point — bootstrap is the natural place for restoration

### References

- [Research: Built-in Session Resurrection](./../research/built-in-session-resurrection.md)

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Hook Lifecycle Redesign [decided]
  ├─ One-shot vs persistent hooks [decided]
  └─ Per-hook configurability [decided]

  Save-Side Architecture [pending]
  ├─ Trigger mechanism (events vs periodic vs hybrid) [pending]
  ├─ Debouncing / serialization strategy [pending]
  ├─ Save format and schema [pending]
  └─ In-process vs subprocess execution [pending]

  Restore-Side Architecture [pending]
  ├─ Bootstrap integration [pending]
  ├─ Shell readiness detection [pending]
  └─ Layout restoration approach [pending]

  Session & Project Store Interaction [pending]
  ├─ Restored session naming [pending]
  └─ projects.json timestamp handling [pending]

  Ephemeral Session Opt-Out [pending]

  CleanStale Guard Behavior [pending]

  Scope Boundaries [pending]
  ├─ Environment / shell state (explicit non-goal) [pending]
  └─ tmux version compatibility [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture. Not every subtopic needs its own section — minor items resolved in passing can be folded into their parent.*

---

## Hook Lifecycle Redesign

### Context

The resume hook system fires stored commands when a user enters a session. Research flagged "one-shot vs persistent" as a design decision — do hooks fire once and delete themselves, or persist across reboots until explicitly removed?

Current implementation is effectively persistent: entries live in `hooks.json` and survive reboots; volatile markers (`@portal-active-<pane>`) prevent duplicate runs within a tmux server lifetime. The research proposed making this configurable per-hook.

### Options Considered

**A: Add a `mode` field — configurable per-hook (`once` vs `always`)**
- Pros: Expressive; user declares intent; `once` auto-cleans after firing so broken hooks fail only once instead of every reboot.
- Cons: Adds API surface, CLI flag, storage field, docs, test matrix. Two more states for users to reason about.

**B: Single behavior — persistent only (status quo)**
- Pros: Minimal mechanism. Matches Portal's generic-hook principle. No new fields.
- Cons: Callers wanting one-shot semantics must implement it themselves at the command level.

### Journey

Initial framing was that `once` and `always` solve different use cases — `once` for dynamic commands like `claude --resume <uuid>` (where a wrapper re-registers on each start), `always` for static commands like `npm start`. Proposed adding a mode field, with the semantic anchor being "survives reboot yes/no."

First clarification: "session alive" meant the *logical* session (same named session across reboots), not the tmux session instance. Detach/reattach within a server lifetime was raised as an edge case but is a non-issue — existing volatile markers handle it correctly because processes are still running and nothing needs restarting. The only scenario where `once` and `always` would behave differently is reboot recovery.

Naming settled early: `once` / `always` — maps cleanly to `--mode=once` CLI flag, matches user mental model ("run this once" vs "always run this when I come back").

Then the user pushed back with a use case audit. For their Claude setup, *both* modes work — the Claude wrapper re-registers a `once` hook on resume, AND a separate exit hook removes the `always` version on explicit Claude exit. That prompted the pivotal question: if both modes work for the flagship use case, what is `once` actually for?

Audit of use cases:
- **Static dev commands** (`npm start`, `tail -f`, file watchers): `always` only; `once` makes no sense.
- **Claude resume** (dynamic UUID): both work.
- **Ephemeral one-time tasks**: `once` slightly cleaner, `always` + manual removal works.
- **Stale hook hygiene** (broken hook fails once vs every reboot): minor win for `once`.

No slam-dunk use case for `once`. The decisive argument came from re-reading the generic-hook design principle from research:

> Portal's hook system is generic. No awareness of what consumers do with it. Portal stores and fires a command string — it's the caller's responsibility to make that command correct.

One-shot vs persistent is *policy*. Portal provides the *mechanism*. If a caller wants one-shot behavior, they implement it at the command level:

```
portal hooks set --on-resume "my-cmd && portal hooks remove --pane $(tmux display -p '#{pane_id}')"
```

Self-removing command. Caller owns the policy, Portal owns the mechanism — consistent with the rest of the hook design.

### Decision

**Do not add a `mode` field.** Portal keeps its single behavior: hooks persist across reboots until explicitly removed. Callers wanting one-shot semantics implement it by having the command remove its own hook as the final step.

**Trade-off accepted**: callers shoulder one-shot policy instead of Portal. Small extension of existing caller responsibility — callers already own the entire command string.

**Confidence**: high. YAGNI-compliant; a mode field can always be added later if a concrete use case emerges.

**False path noted**: the research framing of "one-shot vs persistent as two viable models" overstated the design space. On inspection, `always` (current behavior) handles every real use case with minor caller-side wrapping.

---

## Summary

### Key Insights
*(To be completed during discussion)*

### Open Threads
*(To be completed during discussion)*

### Current State
- Hook Lifecycle Redesign: **decided** — no mode field; single persistent behavior; one-shot is a caller-level policy via self-removing commands
- Remaining: Save-Side Architecture, Restore-Side Architecture, Session & Project Store Interaction, Ephemeral Session Opt-Out, CleanStale Guard Behavior, Scope Boundaries
