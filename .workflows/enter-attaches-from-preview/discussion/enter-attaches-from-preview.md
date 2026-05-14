# Discussion: Enter Attaches From Preview

## Context

The scrollback preview page (`Space` from Sessions) is a read-only viewport that lets the user inspect a session's recent output before committing to attach. Today the preview's `Update` handler (`internal/tui/pagepreview.go:257-317`) handles `Esc`, `Home`, `End`, `Tab`, `]`, `[` but has no `tea.KeyEnter` case — Enter falls through to the embedded viewport, which treats it as a no-op for scrolling. The user therefore has to press `Esc` to dismiss and then `Enter` on the highlighted session, when their mental model says one keystroke should do it.

The current behaviour matches the existing spec, so this is a **spec amendment**, not a bug fix. `session-scrollback-preview/specification.md:60-72` lists preview's owned keymap as `]`, `[`, `Tab`, `Esc` and explicitly says "Everything else either passes through to the embedded bubbles/viewport (scroll keys) or is unbound/no-op". The user's mental model is reinforced by spec line 17 — "Attach. `Enter` continues to attach as today (unchanged)." — which reads that way in isolation even though in context it was scoped to Sessions-page behaviour.

The goal of this discussion is to decide whether to add Enter-attaches-from-preview, and if so, what it attaches to and how it behaves in edge cases. A secondary goal is to define the keymap-expansion policy so we are not re-litigating this per key (`r`, `k`, etc.) later.

### References

- Inbox source: `.workflows/.inbox/.archived/ideas/2026-05-14--enter-attaches-from-preview.md`
- Current preview keymap implementation: `internal/tui/pagepreview.go:257-317`
- Existing spec: `.workflows/session-scrollback-preview/specification.md:17, 60-72`
- Related completed feature: `session-scrollback-preview` (last phase: review)

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Enter target: session vs focused pane [decided]

  Transition mechanics: instantaneous vs two-beat dismiss-then-attach [decided]

  Mid-load / placeholder behaviour [decided]

  Edge cases [exploring]
  ├─ Filter committed, zero matches [pending]
  ├─ Session killed externally while previewing [pending]
  └─ Preview opened on a row that is no longer current [pending]

  Keymap expansion policy [pending]
  └─ Where does the line sit for `r` rename, `k` kill, other Sessions keys [pending]

  Spec-amendment scope [pending]
  └─ Update spec line 17 and lines 60-72 to reflect the new owned key [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## Enter target — session vs focused pane

### Context

Preview already makes pane focus a real, observable concept. `]` and `[` step between windows, `Tab` cycles panes within the current window. By the time the user presses Enter, they have potentially navigated to a specific `(window, pane)` coordinate inside the previewed session. The question is whether that navigation state is *viewport chrome* (discarded on attach, tmux picks its own current pane) or *intent* (carried into the attach).

### Options Considered

**Option A — Attach to the session, ignore preview's pane focus.**

Enter triggers the existing Sessions-page attach path. tmux's last-current pane in the session wins. Preview's pane-focus state is treated as "which scrollback am I looking at right now", not a destination.

- Pros: simplest; preview stays a pure peek; pane focus is a viewport-rendering concept, not a session-state primitive.
- Cons: ignores the user's keystrokes. They tabbed to a specific pane *for a reason*. Dropping that on attach is friction.

**Option B — Attach to the session, applying preview's window AND pane focus first.**

Before the existing attach/switch path runs, issue `tmux select-window -t <session>:<window>` and `tmux select-pane -t <session>:<window>.<pane>` using the indices preview captured at open and walked with `]`/`[`/`Tab`. Then the existing connector takes over.

- Pros: preview's navigation has *meaning* — it's "navigate to where I want to land, then commit". Matches the user's mental model: "I previewed *this pane*, so Enter takes me to *this pane*". Implementation cost is two `tmux` calls before existing code path; no new architecture.
- Cons: overrides "where I was last working" in the session. If the user had pane 2 active, walked away, came back, previewed, Tab'd to pane 0 just to peek, then Enter — they land on 0, not 2. Real cost, but bounded — the user just navigated to it deliberately.

**Option C — Attach to the focused pane only.**

Collapses into Option B at the tmux layer because tmux attaches whole sessions; `select-pane` is the only available knob. Not a separate option.

### Journey

Started with the framing question "session or pane?" and immediately recognised that preview's `]`/`[`/`Tab` keys already commit pane focus to a real navigational state — they are not idle. The user confirmed the desired behaviour with a concrete example: a session with two windows × four panes, after Tab to "third pane of the second window", Enter should activate *that* window and *that* pane.

Pushed back on Option B with the "walked-away" scenario — does B override real session state with a casual peek? The cost is real but bounded: the user's last `Tab`/`]`/`[` press is deliberate. If they only wanted to peek, they would have used Esc.

Implementation falls out cleanly: two new tmux calls (`select-window`, `select-pane`) inserted *before* the existing connector path. The attach/switch mechanics, the hooks pipeline, the resume hydration — all unchanged. Inside-tmux uses `switch-client` with `-t <session>:<window>.<pane>` already supported, but the explicit pre-select keeps the two paths uniform.

### Decision

**Option B — Enter attaches to the session, applying preview's captured window AND pane focus before the existing attach/switch path runs.**

Deciding factor: preview's window/pane navigation state is *intent*, not chrome. The user paid keystrokes to focus a specific coordinate; ignoring that on attach turns the navigation into theatre.

Mechanics:

1. On `tea.KeyEnter` in preview, capture the current `(session, window_index, pane_index)` from preview's state.
2. Issue `tmux select-window -t <session>:<window_index>` and `tmux select-pane -t <session>:<window_index>.<pane_index>` (order matters — window first, then pane within it).
3. Dispatch the same connector message the Sessions-page Enter dispatches today (`AttachConnector` outside tmux, `SwitchConnector` inside tmux).
4. Hooks fire exactly as on any other attach — no special-cased path.

Trade-offs accepted: in the "walked-away peek" scenario, the user lands on the last-focused preview pane rather than tmux's prior current pane. Mitigation: the user's most recent `]`/`[`/`Tab` press *is* their stated intent.

Confidence: high.

---

## Transition mechanics

### Context

Once Enter attaches with preview's focus applied, *how* the handoff happens matters. The inbox raised "instantaneous vs two-beat dismiss-then-attach" — i.e. does the user see a perceptible frame of "back to Sessions list" between preview and the attached session?

### Options Considered

**Option A — Instantaneous: preview's `Update` returns the pre-select + connector commands directly.**

`tea.KeyEnter` in preview issues `tmux select-window` + `tmux select-pane` (per the Enter target decision) and the existing `AttachConnector` / `SwitchConnector` command as one logical unit. Bubble Tea processes the cmd, tmux takes over via `syscall.Exec` (outside) or `switch-client` (inside). No intermediate render.

**Option B — Two-beat: preview dismisses to Sessions page, then Sessions-page Enter fires.**

Preview returns a "dismiss" message; the page state machine transitions to `pageSessions`; a synthetic Enter is dispatched. Two render passes, perceptible frame.

### Journey

Option B looked superficially clean (reuse Sessions-page Enter as the single attach entry point) but fails the previous decision: Sessions-page Enter does not know about preview's `(window, pane)` focus. The pre-select+attach sequence must be authored *as one unit* in the preview Update, not split across a page transition.

User framing reinforced the call: "programmatically, it doesn't make any sense" to navigate back and re-attach. The connector primitives already work from any page-Update return path; preview can drive them directly.

### Decision

**Option A — Instantaneous. Preview's `Update` returns the select-window + select-pane + connector commands as one logical sequence; no intermediate render, no Sessions-page round-trip.**

Build-phase note: the three tmux calls (`select-window`, `select-pane`, `attach`/`switch-client`) should be sequenced so the selects complete before the connector hands off the terminal. Implementation detail (likely `tea.Sequence`, or wrapped into one connector function that performs all three) — not pinned by spec.

Confidence: high.

---

## Mid-load / placeholder behaviour

### Context

The inbox framed this as "what if the user presses Enter while content is still loading?" Re-reading the existing preview spec clarifies that the tail-N read is synchronous (`state.TailScrollback` returns immediately; viewport renders via straight passthrough). There is no async-load UI state. The only three observable shapes are:

1. Real bytes — viewport renders content.
2. `(nil, nil)` — viewport renders the "(no saved content)" placeholder (brand-new session, no `.bin` yet, or daemon hasn't captured).
3. OS-level read error — viewport renders an error string (EACCES, EIO).

The question collapses to: **does the placeholder or error state change Enter's behaviour?**

### Decision

**No — Enter attaches unconditionally regardless of scrollback state.**

Rationale:

- Whether scrollback was *saved* is independent of whether the session is *attachable*. The live tmux session exists either way — preview wouldn't have opened on a non-existent session.
- A "no saved content" placeholder most commonly means the daemon hasn't captured yet, or the session is fresh. Neither is a reason to block attach.
- An OS read error is a *file-system* problem, not a session problem. Blocking attach on it would make file trouble unnecessarily block session use.

No confirmation prompt, no guard. The user's keystroke is their commitment.

Confidence: high.

---

## Summary

### Key Insights

*(populated as discussion progresses)*

### Open Threads

*(populated as discussion progresses)*

### Current State

- Discussion just started — no subtopics decided yet
