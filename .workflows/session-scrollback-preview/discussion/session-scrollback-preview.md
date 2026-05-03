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

  Multi-pane rendering shape [decided]
  ├─ Sequential, one pane at a time
  ├─ Window-grouped cycling
  └─ Header chrome with keystroke hints (Portal convention)

  History depth [decided]
  ├─ Bounded snapshot, scrollable within bounds
  └─ Generous N (e.g. ~500-1000 lines), pin in spec

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

## Multi-pane Rendering Shape

### Context

Sessions can contain multiple windows, each with multiple panes. Preview must
represent that structure (Stated Feature Shape: "Multi-pane / multi-window in
scope"), but *how* the structure is rendered has a real cost gradient. The
question is whether we render the literal `window_layout` (best fidelity,
custom parser ~50–100 LOC) or use a sequential / window-grouped flat
presentation that doesn't honour the actual layout shape.

Real-world distribution sample is N=1 — 14 of 16 sessions on the user's
machine are 1-pane (research F6) — so the dominant case collapses regardless
of the choice. Decision matters only for the 2+ pane minority.

### Options Considered

**Option A — Sequential / tabbed (flat cycle)**

One pane shown at a time. Single key cycles through every pane in the
session, flat ordering: w1.p1 → w1.p2 → … → w2.p1 → wraps. Header shows
position.

- Pros: cheapest. Reuses single-pane rendering verbatim. Header line tells
  the whole story.
- Cons: collapses the window/pane hierarchy. For sessions with multiple
  windows of distinct purpose, the flat list is harder to navigate
  intentionally.

**Option B — Sequential, window-grouped**

One pane shown at a time, but cycling is hierarchical: one key cycles panes
*within the current window*, another key jumps windows. Header shows both
window position and pane position.

- Pros: matches tmux's own mental model (windows contain panes; you switch
  between windows, you cycle panes within). Maps cleanly to the natural
  phrasing — "this session has two windows; window 1 has four panes,
  window 2 has one". Still cheap — same renderer.
- Cons: two keys instead of one. Marginal added concept.

**Option C — Literal `window_layout`**

Parse the opaque `window_layout` string, divide the preview viewport
proportionally, render each pane in its slot.

- Pros: best visual fidelity. Layout shape itself becomes a strong
  disambiguation signal — "the session with the four-pane grid" reads
  instantly without cycling.
- Cons: ~50–100 LOC parser nobody else in portal needs. Per-pane content
  fits a smaller box (typical 120×30 → ~30×12 per pane in a 4-pane grid).
  Header/footer chrome eats more vertical room. Higher implementation cost
  for a benefit that mostly accrues to a minority of sessions.

### Journey

Opened framing as a cost-fidelity gradient with three stops. User cut
straight through:

> If I open that up, as long as it says somewhere that this window has two
> windows, window one has four panes, and window two has one pane, I'm fine
> with that. As long as I can see that and tab between the panes and the
> windows. 95% of the time it's single window, single pane per session.

That argument lands twice:

1. **Distribution kills the case for fidelity.** The sessions where literal
   layout would shine are a minority of a minority. Most sessions are 1×1
   and all three options render identically.
2. **Recognition needs structure, not shape.** A header that says "window 1
   of 2, pane 2 of 4" *is* the structural disambiguation signal. The user
   doesn't need to *see* the layout to recognise the session; knowing the
   structural shape (counts and current position) is enough.

Picked the cycle semantics next: flat-vs-grouped. User went grouped — it
matches tmux's natural mental model and the way the user phrased the
overview in conversation ("window one has four panes, window two has one").

User added: keystroke hints visible in the chrome, matching portal's
existing UI convention elsewhere.

Literal-layout was explicitly deferred, not rejected:

> If later I decide I'd like to actually recreate the window layout
> structure, then we can add that in later. But for now, I just don't think
> we need it.

### Decision

**Sequential, window-grouped.** One pane shown at a time. Two keys for
in-preview navigation:

- **Window-cycle key** — moves across windows (forward; reverse via
  shift-modifier or sibling key).
- **Pane-cycle key** — moves across panes within the current window
  (forward; reverse via shift-modifier or sibling key).

Header (or footer) chrome shows structural overview and current position
explicitly — sufficient detail that the user can identify "which window am
I in" and "how many siblings does this pane have" at a glance — plus
visible keystroke hints in portal's existing UI convention.

The actual keybindings (which key to use for which axis) are owned by the
**Stepping key inside preview** subtopic, which now has two distinct
concerns to resolve:

1. *Between-session* stepping (cycle through the candidate sessions in the
   picker without exiting preview).
2. *Within-session* stepping (the window/pane cycles decided here).

Deciding factors:

- Real-world distribution makes literal-layout a low-leverage investment.
- Header chrome carries the structural disambiguation signal that fidelity
  would otherwise carry.
- Window-grouped matches tmux's mental model — natural for users who already
  think in tmux terms.
- Reversibility is high — literal-layout can be added later without
  invalidating the sequential renderer (additive flag/mode, not a rewrite).

Trade-offs accepted:

- Multi-pane sessions don't preview *as themselves* shape-wise. Mitigated by
  the chrome.
- Two cycle keys instead of one. Marginal.

Confidence: high. Decision is explicitly staged for upgrade if v1 feels
weak in practice.

### Open Sub-decision Carried Forward

Header chrome content is sketched but not pinned: counts + current position
+ keystroke hints. Exact wording / placement (header vs footer, single-line
vs two-line) is a UI detail to settle alongside the broader preview chrome
during specification or build. Not a discussion-phase blocker.

---

## History Depth

### Context

Each pane's `.bin` file on disk holds the full saved scrollback — for busy
sessions this can be 50k+ lines, ~3.7MB (research F1). The viewport renders
maybe 30 lines at a time. Question: how much of the file do we feed into the
viewport per preview, and is deeper history reachable from inside preview?

### Options Considered

**Option A — Bounded snapshot, fixed depth, no scroll**

Read last N lines (e.g. 200 = ~10× viewport). Viewport shows what fits. No
scroll. No way to reach deeper history.

- Pros: minimal memory footprint, simplest mental model.
- Cons: even within the bounded slice, the user can't peek above the visible
  viewport. Wastes the cheap part of `bubbles/viewport`.

**Option B — Bounded snapshot, scrollable within bounds**

Read last N lines. Viewport renders the tail by default. User scrolls up
within the viewport to see content within those N lines. Scroll boundary at
the top is the bounded read. Deeper history (beyond N) is not reachable in
v1.

- Pros: scroll within preview is free with `bubbles/viewport`. Generous N
  costs nothing extra at read time (we're disk-reading the full `.bin`
  regardless and tail-clipping in memory).
- Cons: scroll boundary is invisible to the user — pressing up at the top
  silently no-ops. Mitigated by chrome or by simply choosing N large enough
  that nobody notices.

**Option C — Bounded with lazy extend**

As B, but pressing `r` (or scrolling past top edge) extends the read to the
full file. Deferred load.

- Pros: deeper history reachable without paying for it on every preview.
- Cons: meaningful additional state per pane, second code path, edge
  cases on the trigger UI. Adds scope without clearly serving a current
  use case.

**Option D — Always full file**

Read everything every time. Simplest read pipeline.

- Pros: zero ceiling on visible history.
- Cons: feeds 50k-line content into `viewport.SetContent` for every step
  through every busy pane. Memory churn during fast stepping is real even
  if the disk read itself is cheap.

### Journey

The use case argument from earlier subtopics carries: disambiguation is
recognition, not forensics. "What command did I run earlier" is genuinely a
different feature — answered by full attach, not preview.

User's instinct landed on A first, then refined to B once it was clear that
scroll-within-bounds is essentially free in `bubbles/viewport`:

> Bounded snapshot, fixed depth, but just make it big enough that it shows
> enough information and can scroll up to and including whatever's
> captured. If later I want more, we can add it.

The "scroll within bounds is free" detail mattered: it removes the false
trade-off between "no scroll at all" and "full deferred extend".
Reversibility was confirmed in three concrete shapes (bigger N, lazy
extend, live tail) — none of which invalidate B as the v1 choice.

### Decision

**Option B — Bounded snapshot, scrollable within bounds.** Read the full
`.bin` from disk, tail-clip to the last N lines in memory, feed N to
`viewport.SetContent`. User can scroll up within those N lines via the
viewport's native scroll keymap.

The exact value of N is a spec detail. Research F1 used 200 lines as the
sub-10ms ceiling for `tmux capture-pane -S -200`, but since we're
disk-reading the full file and tail-clipping in memory the read cost is
constant regardless of N. We can be generous — ~500-1000 lines gives
comfortable scroll headroom without paying for it. Pin in spec.

Deciding factors:

- The disambiguation use case doesn't need deep history; the recent screen
  is the load-bearing content.
- Scroll within bounds is free with `bubbles/viewport` — no architectural
  cost.
- Reversibility into "more history" is easy in three independent shapes:
  bump N, add lazy extend, add live tail. None of these require revisiting
  this decision.

Trade-offs accepted:

- Hard top-edge boundary at N is invisible to the user. Acceptable if N is
  generous enough that the boundary rarely surfaces.
- Memory: holds N lines per previewed pane during preview. Negligible at
  N≤1000.

Confidence: high. Reversibility is genuine and the use case is well
constrained.

---

## Summary

### Key Insights
*(populated as discussion progresses)*

### Open Threads
*(populated as discussion progresses)*

### Current State
- 3 of 9 subtopics decided.
- Refresh Semantics has an early signal: snapshot, not live tail.
- Stepping Key subtopic now owns two distinct concerns (between-session
  stepping + within-session window/pane cycle keys).
- N (history depth ceiling) carried forward as a spec-time detail.
- 6 subtopics still pending.
