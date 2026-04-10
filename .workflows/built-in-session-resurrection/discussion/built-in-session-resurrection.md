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
  ├─ Execution model (daemon vs subprocess-per-event) [pending]
  ├─ Trigger mechanism (which tmux events to hook) [pending]
  ├─ Crash safety / periodic save cadence [pending]
  ├─ Debouncing / serialization strategy [pending]
  ├─ Save format and schema [pending]
  ├─ save-state CLI surface and contract [pending]
  └─ tmux hook registration lifecycle (install/uninstall/upgrade) [pending]

  Restore-Side Architecture [pending]
  ├─ Bootstrap integration [pending]
  ├─ Fate of WaitForSessions / bootstrapWait [pending]
  ├─ Shell readiness detection [pending]
  └─ Layout restoration approach [pending]

  Failure Modes & Recovery [pending]
  ├─ Corrupt / partial saved state [pending]
  ├─ Missing working directories on restore [pending]
  ├─ Layout fit failures (terminal size drift) [pending]
  └─ User feedback on partial restore [pending]

  Observability & Diagnostics [pending]
  ├─ Save-state introspection command [pending]
  ├─ Logging strategy [pending]
  └─ Health signals for silent failures [pending]

  CleanStale Guard Behavior [pending]
  ├─ Guard rationale change post-restoration [pending]
  └─ Stale-hook detection criteria (binary/dir/project missing) [pending]

  Session & Project Store Interaction [pending]
  ├─ Restored session naming [pending]
  └─ projects.json timestamp handling [pending]

  Ephemeral Session Opt-Out [pending]

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

One-shot vs persistent is *policy*. Portal provides the *mechanism*. If a caller wants one-shot behavior, they implement it at the command level — not inside Portal.

### False path: `&&` chaining

An initial framing proposed that one-shot callers could self-remove via shell chaining:

```
portal hooks set --on-resume "my-cmd && portal hooks rm --on-resume"
```

**This doesn't work for the flagship use case.** The canonical hook commands are long-running processes — `claude --resume <uuid>`, `npm start`, `tail -f`. These never exit, so the `&&` clause never fires, and the hook never removes itself. The proposed pattern was architecturally broken for the exact class of commands hooks exist to serve.

Verified against the codebase: the actual CLI is `portal hooks set --on-resume "..."` and `portal hooks rm --on-resume`, both inferring the current pane from `TMUX_PANE` and keying hooks by structural key (`session:window.pane`). The API shape is fine; shell chaining is not.

### The actual caller pattern: wrapper-script lifecycle management

The correct model — and the one the user already described from their Claude setup — is that long-running processes are invoked by a wrapper script which *owns* the hook lifecycle:

- Wrapper registers a Portal hook when the process starts (using current state, e.g., resume UUID)
- Wrapper re-registers on each resume if the hook command is dynamic
- Wrapper removes the hook on explicit process exit (via exit trap or explicit teardown)

Portal is never involved in deciding when to remove; it just exposes `set`/`rm` primitives that the wrapper calls at the appropriate lifecycle moments. This keeps Portal fully generic while giving callers precise control.

### Decision

**Do not add a `mode` field.** Portal keeps its single behavior: hooks persist in the store across reboots until explicitly removed via `portal hooks rm`. Callers that want one-shot or bounded-lifetime semantics manage it from a wrapper script around the target process — using set/rm as primitives at start/exit points.

**Trade-off accepted**: callers of long-running processes shoulder the responsibility of wiring up wrapper-script hook management. This is consistent with the rest of Portal's hook design — callers already own the command string entirely, and wrapping a long-running process is standard operational practice.

**Confidence**: high. YAGNI-compliant; a mode field can be added later if a concrete use case emerges where wrapper-script management is genuinely impractical.

**False paths documented**:
1. *"One-shot vs persistent as two viable models"* (original research framing) — overstated the design space. `always` (current behavior) handles every real use case with caller-side wrapping.
2. *"`&&` chaining for self-removal"* — architecturally broken for long-running processes, the exact class of commands hooks serve.

---

## Summary

### Key Insights
*(To be completed during discussion)*

### Open Threads
*(To be completed during discussion)*

### Current State
- Hook Lifecycle Redesign: **decided** — no mode field; single persistent behavior; one-shot is a caller-level policy via wrapper-script lifecycle management (not `&&` chaining)
- Map expanded post-review to include Failure Modes & Recovery, Observability & Diagnostics, and several previously-missing sub-concerns under Save-Side and Restore-Side
- Remaining: Save-Side Architecture, Restore-Side Architecture, Failure Modes & Recovery, Observability & Diagnostics, CleanStale Guard Behavior, Session & Project Store Interaction, Ephemeral Session Opt-Out, Scope Boundaries
