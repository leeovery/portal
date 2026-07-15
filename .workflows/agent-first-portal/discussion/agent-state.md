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

### herdr prior art — detection layers, state model, agent-shaped API
*From: ad-hoc herdr research (outside workflow) · external · 2026-07-15*

herdr (herdr.dev, github.com/ogulcancelik/herdr) is a Rust agent-multiplexer — a
tmux *replacement* with its own PTY layer and a daemon/client split; ~15k GitHub
stars in ~105 days, solo developer, pre-1.0. Researched 2026-07-15 as prior art
for this epic: it is the closest existing implementation of the agent-state
spine this topic is designing, and several of its choices independently
validate or usefully challenge the discovery seed.

**State model.** herdr tracks four semantic states per pane: `working`,
`blocked`, `done`, `idle`. Near-identical to our planned Working / Waiting /
Idle / Unknown. The one divergence: herdr has `done` (task finished) where we
have `Unknown`. Worth testing whether `done` earns a place in our model — after
Waiting it is the state users most want notified — and whether `Unknown` is
better framed as a capture-quality signal (detection couldn't classify) than a
peer state.

**Two-layer detection** (feeds *Event capture & hook→state mapping* and the
"how do non-Claude agents signal state" open thread). Layer 1 is zero-config:
process-name matching plus terminal-output heuristics — launch any of 14+
supported agents in a pane and herdr picks it up with no setup. Layer 2 is
opt-in per-agent integrations (`herdr integration install claude`) that use the
agent's own hook system (e.g. Claude Code SessionStart / tool-approval hooks)
to report state precisely — granular "waiting for tool approval" vs the
heuristic tier's generic "working". The layering matters for us: our discovery
seed assumes hook-reporting agents via adapters; herdr shows a heuristic
fallback tier is viable and gives day-one coverage for agents with no adapter
yet (tmux gives us `pane_current_command` + `capture-pane` for the same trick).
Hooks remain the precision tier, exactly as we planned.

**IPC transport** (feeds *Hook-to-daemon IPC transport*). herdr's control
channel is newline-delimited JSON over a unix domain socket
(`~/.config/herdr/herdr.sock`, per-named-session sockets under
`sessions/<name>/`, `HERDR_SOCKET_PATH` override; named pipe on Windows).
Request/response with correlation ids
(`{"id":"req_1","method":"pane.current","params":{…}}`), typed error objects
(`code` + `message`), protocol versioning, tolerant of unknown response fields.
Directly reusable prior art for our hook→daemon unix-socket design.

**Correlation** (feeds *Pane / session correlation*). herdr owns its panes so
correlation is native, but its API still has agents self-identify via a
`caller_pane_id` param, and lets an agent register its own native session
reference (`pane.report_agent_session`). Our `$TMUX_PANE` plan is the
tmux-native equivalent — no contradiction, but note the self-report posture:
the agent tells the daemon who it is rather than the daemon inferring
everything.

**Agent-shaped consumer contract** (feeds *Consumer read contract* and *Agent
store & persistence*). The biggest genuinely-new angle: herdr's API treats
agents as *consumers and orchestrators* of state, not just subjects of it.
Verbs: `pane.report_agent` (self-report state), `pane.report_metadata`,
`pane.read`, `pane.send_text` / `pane.send_keys`, `pane.wait_for_output`
(block until output matches), `agent.wait_for_status`, `events.subscribe`
(push on agent-status change and lifecycle events). Agents spawn panes, read
each other's output, and wait on each other — multi-agent orchestration
through the multiplexer itself. Our seed frames consumers as picker /
status-line / notifications / mobile; herdr suggests a fifth consumer class:
the agents themselves. The read contract should decide deliberately whether it
is human-surface-only or agent-consumable, even if the agent-consumer surface
is deferred.

**Strategic footnote worth keeping.** herdr has no reboot resurrection (their
admitted gap — no tmux-resurrect equivalent) and re-implements terminal
emulation with reported rendering-perf issues at many panes. This validates
Portal's wrap-tmux position: we keep resurrection and tmux's battle-tested
core for free while adding the state spine herdr had to build a whole
multiplexer to get. Also explicit in their docs: no sandboxing — agents run
with user permissions. Same posture as us, but worth stating as a non-goal.

Sources: herdr.dev · herdr.dev/compare · herdr.dev/docs/socket-api ·
github.com/ogulcancelik/herdr · bitdoze.com/herdr-agent-multiplexer (review
with detection-mechanism detail).
