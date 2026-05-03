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

  Source of preview bytes (live-capture vs always-disk) [decided]

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

## Source of Preview Bytes

### Context

Preview must show the visual terminal state of each pane in the previewed
session. Two architectural shapes were viable per research:

- **Always-disk.** Read each pane's `.bin` file written by the save daemon.
- **Marker-branched.** Branch on `@portal-skeleton-<paneKey>`: live
  `tmux capture-pane -e -p -S -<n>` for hydrated panes, disk for skeletons.

Both are feasible and side-effect-free (research F3). This is a what-to-build
choice — staleness vs liveness vs code-path complexity — not a feasibility
question.

### Options Considered

**Option A — Always-disk**

- Pros:
  - Single read path. No marker check. No fork in the rendering pipeline.
  - No new tmux wrapper. The existing `CapturePane` hardcodes `-S -` and is
    shared with save-daemon semantics; a bounded variant
    (`CapturePaneTail(target, n)`) would be net-new.
  - No per-preview tmux IPC. File reads are microseconds, so stepping is
    essentially instant.
  - F13 rapid-stepping race largely vanishes — the window where a slow
    capture overwrites a newer view is too small to matter when the read is
    file I/O.
  - Skeleton panes already require this path. Extending it to hydrated panes
    adds zero new code.
- Cons:
  - Up to ~1s stale, longer if the daemon's per-tick worst case exceeds 1s
    under heavy load (research F2: "small but not strictly bounded by 1s").
  - Brand-new sessions created within the last save tick may have no `.bin`
    yet — placeholder needed (separate subtopic).
  - Surface labelled "what attaching now would show" is actually a snapshot
    of the previous tick.

**Option B — Marker-branched (live for hydrated, disk for skeletons)**

- Pros:
  - Zero staleness for hydrated panes.
  - Sets up infrastructure if a future live-tail mode is wanted.
- Cons:
  - Two code paths; rendering forks on marker.
  - Requires a new `CapturePaneTail` wrapper.
  - F13: rapid stepping → in-flight captures for session N landing after
    user has stepped to N+1; needs generation/sequence tokens to ignore
    stale replies.
  - Per-preview tmux IPC adds latency (sub-30ms typical, real cost on busy
    multi-pane workspaces).

### Journey

Opened the decision against the actual use case rather than the abstract
property: disambiguation is a **recognition** task, not a **live monitoring**
task. The user is glancing to identify "which `pigeon-AbCdEf` is which",
not watching a session change in real time.

User probed the worst-case staleness directly:

> If a log is tailing 100 lines/sec and the daemon is busy, we could be
> hundreds of lines out of date — still doesn't matter. I'm not looking at
> line content; I'm recognising "this is my tailing log session". It catches
> up once I attach.

Two ends of the bandwidth spectrum both hold:
- Slow output (Claude TUI): 1-2s stale is invisible because Claude moves
  slowly in the first place.
- Fast output (busy logs): the user isn't reading individual lines, they're
  identifying the session by overall shape.

User also independently confirmed: not a single-user-on-multiple-machines
environment. No concurrent attach contention worth designing for — they
won't be working on the previewed session from elsewhere while previewing it.

Live preview was explicitly rejected as a separate concern: previews are
**snapshots at the moment they're taken**, not real-time feeds. This pre-empts
part of the Refresh Semantics subtopic — though "do we re-read on each step?"
and "is `r` worth offering?" remain open there.

### Decision

**Always-disk.** Single read path: `state.ScrollbackFile(stateDir, paneKey)`
for every pane in the previewed session, regardless of skeleton-marker state.

Deciding factors:

- Disambiguation use case is recognition, not live monitoring — staleness is
  invisible at the user-perception level for both ends of the bandwidth
  spectrum.
- Single-user-per-machine environment means no concurrent-attach contention
  worth designing complexity around.
- Architectural simplicity: no marker check, no rendering fork, no F13
  mitigation, no new tmux wrapper.
- Reversibility is high — if liveness ever matters later, marker-branched
  can be added as a per-pane source override without changing the rendering
  pipeline.

Trade-offs accepted:

- Staleness ceiling is "small but not strictly bounded by 1s". Accepted
  because it doesn't bite the use case.
- Brand-new session edge case (no `.bin` yet) is owned by its own subtopic.
- The "live state" surface label is a small honesty cost — preview is a
  snapshot, full stop.

Confidence: high. Grounded in actual workflow, with genuine reversibility.

---

## Summary

### Key Insights
*(populated as discussion progresses)*

### Open Threads
*(populated as discussion progresses)*

### Current State
- 1 of 9 subtopics decided (Source of Preview Bytes — always-disk).
- Refresh Semantics has an early signal: snapshot, not live tail.
- 8 subtopics still pending.
