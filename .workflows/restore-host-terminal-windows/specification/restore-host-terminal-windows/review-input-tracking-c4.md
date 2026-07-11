---
status: in-progress
created: 2026-07-11
cycle: 4
phase: Input Review
topic: restore-host-terminal-windows
---

# Review Tracking: Restore Host Terminal Windows - Input Review

## Findings

### 1. Ack-channel choice: daemon-readable server-option markers (future-workspace extensibility rationale) dropped

**Source**: Discussion §3 "Burst & Partial-Failure Contract" → confirmation-mechanism decision, bullet: *"Pivot to the daemon (channel c) is additive, not a rewrite: the daemon already ticks every second reading tmux state, so the future remember-and-restore-workspace feature just teaches it to read the same markers and record outcomes — no change to how the picker collects."*
**Category**: Enhancement to existing topic
**Affects**: Burst & Partial-Failure Contract → *Ack channel* subsection (secondarily relevant to Observability & State Footprint / Deferred Scope)

**Details**:
The discussion justifies the `@portal-spawn-*` tmux server-option ack channel on *four* grounds; the spec carries three of them (invisible to `ListSkeletonMarkers`, namespacing isolates sweeps in both directions, server options die with the server) but drops the fourth: the marker channel is deliberately **daemon-readable**, so the deferred remember-and-restore-workspace follow-on can teach the 1s-tick daemon to read the same markers and record outcomes *additively* (no rewrite of the picker's collection path).

This is a load-bearing forward-compat rationale for a design choice, not decision-process residue. It matters because (a) it explains why a server-option marker was chosen over an ephemeral in-process channel — it survives for a *different reader* later — and (b) the spec already documents analogous forward-compat rationale for its other core decisions (the `OpenWindow(command)` contract "future-proofs the adapter"; the config `commands.open` nesting is "additive sub-keys, not a breaking schema change"). Its omission here is an inconsistency: the ack channel is the one core mechanism whose future-extensibility story got trimmed. Materiality is low (it concerns a deferred feature), but it traces cleanly to source and belongs with the other extensibility notes.

**Current**:
> **Ack channel.** A namespaced **`@portal-spawn-<batch>-<session>` tmux server option**, behind a small ack seam (write-token / collect-tokens interface). Code-verified safe: the only all-server-options enumerator, `ListSkeletonMarkers`, skips any name not prefixed `@portal-skeleton-` (`internal/state/markers.go`), so a distinct `@portal-spawn-` prefix is invisible to it; namespacing isolates sweeps in both directions; server options die with the server.

**Proposed Addition**:
Append to the *Ack channel* paragraph (or add as a trailing sentence): "The server-option channel is also deliberately **daemon-readable**: the deferred remember-and-restore-workspace follow-on can teach the 1s-tick daemon to read the same `@portal-spawn-*` markers and record outcomes as an *additive* change — no rewrite of how the picker collects acks. (Forward-compat only; not built here.)"

**Resolution**: Pending
**Notes**:

---
