# Discussion: Agent State Surfacing

## Context

How agent state surfaces on the desktop and in the terminal, distinct from
host capture (the [agent-state](agent-state.md) topic). From the discovery
seed: an inline agent-state glyph on every session row in every picker view,
distinct from the existing attached dot; a by-status / needs-attention view
mode alongside flat, by-project and by-tag, sorted so Waiting floats to the
top; a Portal-owned tmux status-line segment showing the aggregate (e.g.
"2 waiting") with a keybind to jump to waiting sessions; notifications —
macOS at the desk, OSC 9 banner through Blink on mobile until the client
exists. No separate dashboard page; the dashboard is a lens, not a page.

*Stub created by a triage landing on 2026-07-15 — working sections fill in
when this topic is picked up.*

### References

- [Discovery seed](../manifest.json) — `discovery.items.agent-state-surfacing`
- [Agent state discussion](agent-state.md) — the host-side spine this topic reads from

## Discussion Map

A living index of subtopics tracked during the discussion.

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — Agent State Surfacing (0 subtopics)

  (seeded when the discussion session first runs)

---

*Subtopics are documented below as they reach `decided` or accumulate enough
exploration to capture.*

---

## Summary

### Key Insights

*(to be populated as the discussion progresses)*

### Open Threads

*(none yet)*

### Current State

- Nothing decided yet — stub created by a triage landing; discussion not yet run.

## Triage

### herdr prior art — sidebar, needs-attention, blocked notifications
*From: ad-hoc herdr research (outside workflow) · external · 2026-07-15*

herdr (herdr.dev, github.com/ogulcancelik/herdr) is a Rust agent-multiplexer —
a tmux replacement with agent-state awareness as its headline feature; ~15k
GitHub stars in ~105 days, pre-1.0. Researched 2026-07-15 as prior art. Its
host-side capture design is landed in [agent-state](agent-state.md)'s triage;
this entry carries the *surfacing* choices relevant to this topic.

**Persistent state sidebar.** herdr's core surface is a sidebar showing
semantic state (blocked / working / done / idle) across every pane — the
whole product pitch is at-a-glance herd state. Validation for this topic's
seed: the picker glyph + needs-attention view is Portal's equivalent, and
herdr's UX confirms blocked/Waiting-first ordering is the right default.
Notably, herdr had to build an entire multiplexer to own that chrome; Portal
gets the same lens through the picker and status line without owning the
screen.

**Blocked-agent notifications.** herdr detects when an agent waits for input
and notifies. Reviews treat this as the single highest-value signal — it is
what turns a multiplexer into a herd manager. Matches this topic's
macOS + OSC 9 plan; prioritise Waiting notifications over any other state
transition.

**Real panes, state as chrome.** herdr renders true terminal views with state
displayed *around* them ("real terminal views, not a wrapped
interpretation"). Portal parallel: state lives in picker rows, the status-line
segment, and notifications — never re-interpreting or wrapping pane content.

**Aggregate summary at narrow widths.** herdr's TUI is responsive down to
phone-over-SSH widths, switching to a touch-optimised menu; the compact
aggregate ("N waiting") is what survives narrowing. Supports the seed's
status-line segment shape ("2 waiting") and is adjacent prior art for the
mobile-client topic's control-tower home screen (routed there separately if
needed — not this topic's scope).

Sources: herdr.dev · herdr.dev/compare · bitdoze.com/herdr-agent-multiplexer.
