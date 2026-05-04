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
  *(Overridden during discussion in favour of an Esc → arrow → Space loop.
  See the Stepping Key Inside Preview subtopic for the rationale.)*
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

  Refresh semantics [decided] — re-read on every step
  ├─ File is source of truth; no content cached in model
  └─ Dwell refresh: step away + back (no `r` key needed)

  Stepping key inside preview [decided] — no between-session stepping;
  Esc → arrow → Space loop replaces it

  List cursor sync vs no sync on Esc [decided] — N/A (preview can't move
  the cursor; Esc returns to original position)

  Filter behaviour during preview [pending]
  └─ Space-while-filtering — load-bearing primary-use-case fork

  Brand-new-session edge case (no `.bin` yet) [pending]
  └─ Also: per-pane .bin gap in multi-pane sessions
     (split-window mid-session, daemon hasn't ticked the new pane yet)

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

**Sequential, window-grouped.** One pane shown at a time. Within-preview
navigation keys:

- **`]`** — next window. **`[`** — previous window. Bidirectional because
  windows are typically purposeful (editor / logs / repl) and overshoot is
  costly.
- **`Tab`** — next pane within current window, forward-only with
  wraparound. Pane counts are small enough that wraparound isn't painful;
  bidirectional bindings would be over-spec for the dominant case.

Degenerate cases (the dominant 95%+ single-window single-pane case): all
three keys silently no-op. No flicker, no error feedback, just nothing.

Header (or footer) chrome shows structural overview and current position
explicitly — sufficient detail that the user can identify "which window am
I in" and "how many siblings does this pane have" at a glance — plus
visible keystroke hints in portal's existing UI convention.

Within-session keys are pinned here. The **Stepping key inside preview**
subtopic now owns only *between-session* stepping (cycling through
candidate sessions in the picker without exiting preview), and must avoid
colliding with `]` / `[` / `Tab`.

**Position on session re-entry: reset.** Stepping out to session B and back
to session A re-opens A at window 1 / pane 1, not at the last viewed
position within A. Reasoning: the use case is disambiguation, fresh-view
matches "step ↔ recognise" better than memory; per-session position state
adds complexity for an interaction shape that doesn't need it.

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

### Chrome Floor

Because the rationale leans on chrome to discharge the promise the
literal-layout option would otherwise carry, the minimum chrome content is
a discussion-level commitment, not a spec detail:

**Floor (must show, v1):**

- **Window M of N** — without this, users have no signal that the session
  has multiple windows.
- **Pane X of Y** — same logic within a window.
- **Window name** — tmux's `#W` / window name. Adds disambiguation signal
  for users who name their windows.
- **Keystroke hints** — Portal's existing UI convention. Without them users
  don't know the cycle keys exist.

**Above the floor (rejected for v1):**

- Per-pane current-command (e.g. `nvim`, `claude`). Costs a `list-panes -F`
  call per preview; nice-to-have not load-bearing.
- Pane position hint (e.g. `(top-right)`). Faint layout nod without parsing
  layout — declined; the cycle nav covers the same gap.

Exact wording / placement (header vs footer, single-line vs two-line) is
still UI work for spec or build — but the *content* is now pinned.

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

## Refresh Semantics

### Context

Live-tail was foreclosed by the Source of Preview Bytes decision (snapshots,
not streams). The remaining question: between preview-open and dismiss,
when (if ever) is the disk re-read?

### Options Considered

**Option A — Snapshot at preview-open, frozen for whole preview**

Read each pane once when first seen. Never re-read until preview closes.

- Pros: cheapest, most predictable.
- Cons: dwelling on one session for 30s shows a 30-second-stale view. Esc
  + Space again to refresh (heavy-handed for a tiny problem).

**Option B — Re-read on every step**

Every focus event (between-session step, `]`, `[`, `Tab`) triggers a fresh
read of the focused pane's `.bin`. No timer, no polling — only reads when
the user acts.

- Pros: file is the source of truth, no content held in model state.
  Stepping cost is one disk read (microseconds for ~20KB tail-clipped from
  the typical `.bin`). Naturally handles dwell — step away + back is
  refresh.
- Cons: same content re-read on every visit (negligible cost).

**Option C — Manual `r` to refresh**

Snapshot frozen until user presses `r`.

- Pros: explicit user control over staleness.
- Cons: adds a key for a case already covered by stepping.

### Journey

User cut through quickly with the architectural framing:

> Re-reading on each step avoids having to store the content too — file is
> source of truth.

That observation is load-bearing for more than just refresh: it implies
the model doesn't cache pane content at all. Per-pane state stays at
"position cursor" and "currently focused pane key" — content is computed
on demand from disk every render-changing event. Memory footprint stays
bounded regardless of how long preview is open or how many panes have
been visited.

Dwell case (open preview, sit for 30s) is handled implicitly: any focus
change re-reads, and re-acquiring focus after stepping out and back is the
natural refresh idiom.

### Decision

**Option B — Re-read on every step.** No content cache, no `r` key, no
timer. Disk is the source of truth; preview is essentially stateless with
respect to byte content.

Read trigger events:

- Initial preview-open (Space).
- `]` / `[` window cycle.
- `Tab` pane cycle.

(Between-session step is *not* a trigger because in-preview between-session
stepping was removed — see Stepping Key. To preview a different session,
the user Esc's back to the list, moves the cursor, and presses Space —
which fires the initial-open trigger fresh.)

Deciding factors:

- File-as-source-of-truth eliminates content state from the preview model.
- Disk reads are microseconds — re-reading on every event is essentially
  free.
- Dwell refresh falls out naturally from stepping.
- One less key surface to design (`r`).

Trade-offs accepted:

- Sit-and-stare-at-one-pane case sees stale content. Low-friction recovery
  (step away + back). Acceptable.

Implementation detail flagged for spec/build (not a discussion-phase
decision): synchronous read in Update vs `tea.Cmd` async with generation
token. Disk reads of ~20KB are sub-millisecond and synchronous-in-Update is
likely fine, but if a `.bin` file is multi-MB the read becomes worth
deferring. Generation tokens (research F13) handle out-of-order returns if
async is chosen.

Confidence: high. The architectural framing makes this almost a
consequence of the always-disk decision rather than a separate choice.

---

## Stepping Key Inside Preview (and List Cursor Sync — N/A)

### Context

Research's Stated Feature Shape called for "in-preview stepping" between
candidate sessions, Claude Code resume-style: Space to open preview on the
highlighted session, then arrows (or some key) move you to the next
candidate session without exiting preview. The motivation was scanning
through several lookalikes without the Esc-Space-Esc-Space cost.

In-preview stepping created a cluster of design surface: which keys move
between sessions, whether the underlying list cursor follows along (List
Cursor Sync), whether stepping iterates the filter-narrowed set or all
sessions, and how those keys avoid colliding with within-session
navigation (`]`/`[`/`Tab`).

### Options Considered

**Option A — In-preview between-session stepping**

Arrow up/down (or similar) inside preview moves to next/previous candidate
session without leaving preview. Cursor in the underlying picker list
follows along (or doesn't — separate sub-decision).

- Pros: matches Claude Code's `--resume` picker mental model. One
  keypress per session-step when scanning lookalikes.
- Cons: surface area — keymap, cursor sync, filter set boundary, key
  collision with within-session keys. Nontrivial to implement and reason
  about. Two pending subtopics ride on it (List Cursor Sync, part of
  Filter Behaviour).

**Option B — Esc → arrow → Space loop**

Preview is bound to one session at a time. To preview another, Esc back
to the list, move the cursor, Space again.

- Pros: dramatic simplification. List Cursor Sync becomes N/A (preview
  can't move the cursor; Esc returns to where you were). Filter
  Behaviour loses its "stepping iterates filtered or all" sub-question.
  Refresh Semantics' trigger list narrows. No between-session keymap to
  design.
- Cons: ~2 extra keypresses per session-step when scanning lookalikes
  (Esc + Space added on top of arrow). For a 5-way scan that's ~10
  extra keystrokes versus Option A.

### Journey

User overrode the research-locked "in-preview stepping" constraint
directly:

> I don't think we should do this — Esc to go back, then arrow up and
> down is fine.

That trades the keystroke savings for a much smaller, simpler surface.
The tradeoff is genuine — Option A is faster for the multi-candidate
scan use case — but Option B's simplification is real and the keystroke
cost is bounded (2 extra keypresses per step is small absolutely).

The research's framing of in-preview stepping as solving "scanning
through several lookalikes" was acknowledged but not load-bearing
enough to justify the design surface.

### Decision

**Option B — No in-preview between-session stepping. Esc → arrow → Space
loop replaces it.**

Cascade:

- **Stepping Key inside preview**: no between-session keys to bind.
  Within-session keys (`]`/`[`/`Tab`) remain pinned by Multi-pane
  decision.
- **List Cursor Sync vs no sync on Esc**: N/A. Preview can't move the
  cursor, so the cursor stays where it was when Space was pressed; Esc
  returns to that same cursor position. No sync question.
- **Refresh Semantics**: the "between-session step" trigger no longer
  applies. Trigger list narrows to initial-open, `]`, `[`, `Tab`. (See
  the updated trigger list in the Refresh Semantics section.)
- **Filter Behaviour**: the "in-preview stepping iterates filtered set
  or all items" sub-question is moot. Space-while-filtering fork
  remains relevant.

Deciding factors:

- Simplification of three pending subtopics (Stepping Key + List Cursor
  Sync + part of Filter Behaviour) into one decision point.
- Esc-Arrow-Space is a familiar idiom across CLI tools; users already
  know it.
- Reversibility is easy — adding in-preview stepping later is additive
  (new keymap entry + cursor-sync-or-not), not a rewrite.

Trade-offs accepted:

- ~2 extra keypresses per session-step when scanning multiple lookalikes.
  Bounded cost, real but small.
- Overrides a research-locked Stated Feature Shape constraint.
  Documented here so the override is traceable.

Confidence: high. The simplicity payoff is concrete (subtopics collapse,
keymap design surface shrinks); the cost is bounded and behavioural, not
architectural.

---

## Summary

### Key Insights
*(populated as discussion progresses)*

### Open Threads
*(populated as discussion progresses)*

### Current State
- 6 of 9 subtopics decided (Stepping Key + List Cursor Sync collapsed
  into the Esc → arrow → Space loop decision).
- N (history depth ceiling) carried forward as a spec-time detail.
- File-as-source-of-truth: preview model holds no byte cache.
- Research's "in-preview stepping" Stated Feature Shape constraint was
  overridden during discussion. Documented in Context and Stepping Key.
- 3 subtopics still pending: Filter Behaviour (Space-while-filtering
  fork), Brand-new-session edge case, Privacy / Threat Model.
