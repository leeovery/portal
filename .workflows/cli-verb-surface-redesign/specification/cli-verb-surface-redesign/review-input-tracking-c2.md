---
status: complete
created: 2026-07-17
cycle: 2
phase: Input Review
topic: cli-verb-surface-redesign
---

# Review Tracking: cli-verb-surface-redesign - Input Review

## Findings

### 1. `--ack` write is best-effort — spec doesn't state the exec proceeds if the write fails

**Source**: Discussion — "Attach Disposition › Journey" (line 282): "`attach`'s actual body is tiny (has-session check → **best-effort ack write** → connect), and every piece has an `open` equivalent."
**Category**: Enhancement to existing topic
**Affects**: `portal open` — Flags & Command Passthrough (Hidden `--ack` flag section); secondarily Multi-Target Burst Mechanics (Atomic pre-flight & partial failure)

**Details**:
The discussion records that the ack write is **best-effort** — the spawned process still connects (execs into tmux) even if writing the `@portal-spawn-<batch>-<token>` server option fails. The spec describes the write as "its last act before exec'ing into tmux" and "a delivery receipt the parent burst polls for," but never states the write is best-effort / non-blocking on the exec. As written, a reader could reasonably infer the exec is contingent on the write succeeding.

This is a real boundary condition of the ack mechanism the redesign owns (`--spawn-ack` → hidden `--ack`): if the write fails, the window **still attaches** (the user gets their session), but the parent's ~8s poll sees no receipt and classifies that window "failed" (a false negative) — leave-what-opened applies, no orphan is created. The spec's partial-failure contract defines "un-acked / failed" purely via the poll timeout and does not cover the connected-but-write-failed case, so the failure semantics are incomplete without the best-effort qualifier. It is preserved existing behavior surfaced in the discussion, not a new decision, but the spec already documents the ack mechanism to this depth, so the best-effort nature belongs alongside it.

**Current**:
"Its behavior: the spawned Portal process, as its last act before exec'ing into tmux, writes `@portal-spawn-<batch>-<token>` as a tmux server option — a delivery receipt the parent burst polls for. Full burst mechanics are in the multi-target topic."

**Proposed Addition**:
Extended the hidden `--ack` behavior: "The write is best-effort: the process still execs into tmux even if the write fails, so the window attaches regardless. A failed write therefore produces a false negative — the window is up but the parent's poll sees no receipt within its timeout and classifies it failed (leave-what-opened applies; no orphan is created)."

**Resolution**: Approved
**Notes**: Auto-approved (sourced from discussion "Attach Disposition" best-effort ack; preserved behavior, completeness fix). Logged to spec.

---
