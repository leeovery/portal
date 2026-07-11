---
status: in-progress
created: 2026-07-11
cycle: 5
phase: Input Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Input Review

## Findings

### 1. §Adapter Contract retains the superseded per-adapter PATH/env-injection framing

**Source**: discussion §6 "recipe execution contract (review-002 F1)" (lines 267–275) — which explicitly *refines/supersedes* §1's per-adapter env-injection (§1 impl flag "Spawned-window PATH (review-002 F3)", line 98). The source decision: "Portal makes `{command}` self-sufficient, uniformly. This *refines* #1's 'each adapter injects the picker's PATH': instead, the picker … threads it into `{command}` itself."
**Category**: Enhancement to existing topic
**Affects**: `Adapter Contract & Extensibility` → "Two implementations, same contract"

**Details**:
The source made a definite supersession decision: env delivery is a **single uniform mechanism in the composed command** — the picker builds an env-self-sufficient argv, so **no adapter needs a per-terminal env property**. The spec captures this supersession correctly in two places (§Spawn Architecture → "Spawned-window environment", line 87: "no adapter needs a per-terminal env property … This supersedes the earlier per-adapter 'each adapter injects the picker's PATH via its own env property' framing"; and §Config Schema → recipe execution contract, line 359).

But §Adapter Contract → "Two implementations, same contract" (line 310) still states that each adapter's concerns include **"injecting the picker's `PATH`/env into the spawned window"** — the abandoned §1 framing — and even cross-references "(see *Spawn Architecture*)", the very section that supersedes it. This distorts the source's #6 F1 decision in one location: an implementer reading the Adapter Contract as the golden description of adapter responsibilities would build the per-adapter env injection the source explicitly removed, contradicting the uniform-composed-command mechanism. The offending clause should be dropped so an adapter's owned concerns are just open-window + typed-result quarantine, with env delivery living uniformly in the composed command per §Spawn Architecture.

**Current**:
> Each adapter owns its own terminal-specific concerns, including injecting the picker's `PATH`/env into the spawned window (see *Spawn Architecture*) and quarantining all OS/terminal specifics behind a typed result (see *Permissions & Error Quarantine*).

**Proposed Addition**:
(Pending — leave blank until discussed. Direction: remove the "injecting the picker's `PATH`/env into the spawned window" clause; adapters own only open-window-with-command + typed-result quarantine, with env carried uniformly in the composed `{command}` per §Spawn Architecture.)

**Resolution**: Pending
**Notes**:

---
