# Discussion: Session Scrollback Preview

## Context

Quick Look-style preview of a session's scrollback from the portal `open` panel,
so users can disambiguate similarly-named sessions — especially Claude / team-up
sessions in the same project, where session names are `{directory}-{nanoid}` and
the only distinguishing context lives in the running content — without paying
the attach/detach cost.

Research established feasibility for every shape under consideration: the
primitives (`tmux capture-pane`, `state.ListSkeletonMarkers`, per-pane `.bin`
files, the `pageFileBrowser` precedent, `bubbles/viewport`) are all in place,
and the side-effect-free preview path is sound (skeleton-marker branch or
always-disk both leave session state byte-identical). What remains is *what to
build*, not *can we build it*.

### Locked Feature Shape (from research, not for re-litigation)

- **Trigger.** Space on a highlighted session opens preview; Enter attaches as
  today; Esc returns to the list.
- **Interaction shape.** Sub-page peer of `pageFileBrowser` — full-screen, own
  keymap, progressive Esc.
- **In-preview stepping.** Step between candidate sessions without exiting back
  to the list (Claude Code resume-style).
- **Centrepiece.** Visual terminal state of the session's panes — same bytes a
  fully attached client would see. Not metadata labels.
- **Multi-pane / multi-window in scope.** Specific rendering shape is design
  phase territory.
- **Side-effect-free.** Space + Esc leaves session state byte-identical. No
  hydration, no hook fire, no marker mutation, no FIFO consumed.

### References

- `.workflows/session-scrollback-preview/research/session-scrollback-preview.md`
- CLAUDE.md § *Server bootstrap*, § *Resume hooks*
- `.workflows/tui-session-picker/specification/...` — page state machine,
  `bubbles/list` precedent

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Source of preview bytes (live-capture vs always-disk) [pending]

  Multi-pane rendering shape [pending]
  ├─ Sequential vs per-window vs literal-layout
  └─ Cost vs fidelity tradeoff against real-world pane-count distribution

  History depth [pending]
  ├─ Bounded snapshot for fast stepping (capture cost ceiling)
  └─ Reachable deeper history on demand?

  Refresh semantics [pending]
  ├─ Snapshot-frozen vs manual `r` vs live tail
  └─ Interaction with rapid stepping

  Stepping key inside preview [pending]

  List cursor sync vs no sync on Esc [pending]

  Filter behaviour during preview [pending]
  ├─ In-preview stepping iterates filtered set or all items
  └─ Space-while-filtering — load-bearing primary-use-case fork

  Brand-new-session edge case (no `.bin` yet) [pending]

  Privacy / threat model [pending]
  ├─ Glanceability vs deliberate-attach exposure shift
  └─ Opt-in toggle / redaction / docs

---

*Subtopics are documented below as they reach `decided` or accumulate enough
exploration to capture.*

---

## Summary

### Key Insights
*(populated as discussion progresses)*

### Open Threads
*(populated as discussion progresses)*

### Current State
- Discussion just initialised; all subtopics pending.
