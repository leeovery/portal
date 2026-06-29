# Discussion: Agent State

## Context

Agent-state is the host-side spine of the agent-first-portal epic. It defines and
captures a per-session agent-state model (Working, Waiting, Idle, Unknown) on the
host, and is the single source of truth that every downstream surface reads from —
the picker glyph, the needs-attention view, the tmux status-line segment,
desktop/mobile notifications, and the mobile control tower.

The shape sketched in discovery:

- Per-agent hooks report events to the daemon over a unix IPC socket.
- Events are correlated to panes via `$TMUX_PANE`.
- Events are normalised through an **adapter interface** — Claude Code first, with
  Codex, OpenCode and others added later as additive adapters.
- A concurrency-safe **agent store** in the daemon is the single source of truth,
  with a persisted snapshot so state survives a daemon blip.

Open threads carried in from discovery:

- Verifying the Claude hook-event → state mapping against current Claude Code
  behaviour.
- How non-Claude agents signal state.

This is the foundational topic of the epic (suggested execution order 1):
agent-state-surfacing, transport, and mobile-client all consume what is decided
here.

### References

- Discovery map item: `agent-first-portal.discovery.agent-state`
- Sibling topics: agent-state-surfacing (consumer), mobile-client (consumer),
  resume-hook-internalisation (shares hook-install machinery)

## Discussion Map

A living index of subtopics tracked during the discussion.

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — Agent State (7 subtopics · 7 pending)

  ├─ ○ State model & semantics [pending]
  ├─ ○ Event capture & hook→state mapping [pending]
  ├─ ○ Hook-to-daemon IPC transport [pending]
  ├─ ○ Pane / session correlation [pending]
  ├─ ○ Adapter interface (multi-agent normalisation) [pending]
  ├─ ○ Agent store & persistence [pending]
  └─ ○ Consumer read contract [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough
exploration to capture.*

---

## Summary

### Key Insights

*(to be populated as the discussion progresses)*

### Open Threads

- Verifying the Claude hook-event → state mapping against current Claude Code
  behaviour.
- How non-Claude agents signal state.

### Current State

- Nothing decided yet — discussion just initialised from the discovery seed.

## Triage

(none)
