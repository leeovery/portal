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

  Edge cases [decided]
  ├─ Pre-select failure / stale window or pane index [decided]
  ├─ Session killed externally while previewing [decided]
  ├─ Filter committed, zero matches [decided]
  └─ Preview opened on a row that is no longer current [decided]

  Keymap expansion policy [decided]
  └─ Where does the line sit for `r` rename, `k` kill, other Sessions keys [decided]

  Spec-amendment scope [exploring]
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

## Edge cases — pre-select failure / stale indices

### Context

The Enter-target decision inserts two new tmux calls (`select-window`, `select-pane`) before the existing connector path. Between preview open and Enter, the underlying session can be mutated by *another tmux client on the same machine* (the user's own second attach, hooks that close windows, background processes splitting/killing panes). In that case the captured window/pane index may no longer be valid and the pre-select fails.

### Decision

**Best-effort pre-select with graceful degradation.**

- Issue `select-window -t <session>:<window>` and `select-pane -t <session>:<window>.<pane>` as before.
- If either call returns a non-zero exit (e.g. window or pane no longer exists), **log and swallow**; do not block, do not warn the user, do not abort.
- Proceed with the existing connector path (`AttachConnector` outside tmux, `SwitchConnector` inside tmux). tmux's last-current pane in the session wins as a natural fallback — equivalent to the pre-existing Sessions-page Enter behaviour.
- Do NOT proactively re-enumerate the session on Enter (no extra `list-panes -F` call). Re-enumeration would cost a round-trip on every Enter for an edge case that is bounded and self-correcting.

### Journey

User initially dismissed the staleness concern on the grounds that portal is a personal single-machine tool and multi-client mutation is rare. They then added that "it doesn't hurt to have some type of fallback if the window or pane is missing". The best-effort shape satisfies both framings: zero design surface for the common case (selects succeed → user lands where they navigated), and a free graceful path for the rare case (selects fail → tmux's last-current wins, which is what Enter did before this feature anyway).

### Trade-offs

- No user-visible feedback when pre-select fails. The user expected to land on pane 3 of window 2; instead they land on whatever tmux had as current. Considered acceptable because (a) the precondition (mutation by another client mid-preview) is rare and (b) the fallback is the pre-existing Enter semantics, not a regression.
- The "session itself was killed externally" case is a different shape — the *connector* fails, not the pre-select. Handled in a separate sub-decision below.

Confidence: high.

---

## Edge cases — session killed externally between preview open and Enter

### Context

Distinct from the pre-select failure case (window/pane disappeared within a live session). Here the entire session is gone — killed by `tmux kill-session`, `portal clean`, the daemon, or another tmux client — between preview open and Enter. Pre-selects fail silently (per the previous decision), then the *connector itself* fails: `tmux attach-session -A -t <session>` or `switch-client -t <session>` returns non-zero against a non-existent session.

Default behaviour without intervention:

- **Outside tmux (`AttachConnector`)**: `syscall.Exec` replaces the process; tmux's error lands in the user's shell. Portal is gone — no way to recover except re-run.
- **Inside tmux (`SwitchConnector`)**: tmux returns an error; the TUI has already exited (the connector path tears down before invoking tmux); error message location is unclear.

Both outcomes are worse than the pre-existing Sessions-page Enter UX — the user thought they were attaching, but instead they're staring at a shell error or a confusing state.

### Decision

**Proactive existence check on Enter + minimal inline flash on the Sessions list.**

1. On `tea.KeyEnter` in preview, before issuing the pre-select calls, run `tmux has-session -t <session>`.
2. If `has-session` returns zero (session exists): proceed with the pre-select + connector sequence as previously decided.
3. If `has-session` returns non-zero (session gone): dispatch a refresh-and-bail message that:
   - Transitions `pagePreview → pageSessions` (same path Esc takes today).
   - Triggers the existing sessions-list refresh on that transition (already part of `session-scrollback-preview`'s dismiss contract).
   - Emits a flash message — one ephemeral line pinned above the Sessions list, e.g. `session "{name}" no longer exists`, auto-cleared on the next keystroke or after a short tick.

The flash is **feature-local infrastructure** scoped to this edge case: a tiny piece of model state on the Sessions page (active flash text + timestamp), rendered as a single chrome line, cleared by the next `tea.KeyMsg` or a tick `tea.Cmd`. No general-purpose toast layer in this feature.

### Journey

Considered three shapes:

- **(α) Silent refresh.** Drop the killed session from the list, no message. Cheapest. Risk: user thinks Enter "just didn't work". Confusing.
- **(β) Minimal inline flash.** This decision. Bare minimum to close the UX loop.
- **(γ) Full toast/flash infrastructure.** General-purpose notification surface usable from every page, with stacking, severity styling, etc. Sibling-feature scope.

User chose (β) for this feature and asked to log (γ) as an inbox idea. The (γ) idea is filed at `.workflows/.inbox/ideas/2026-05-14--general-tui-flash-infrastructure.md` for separate scoping; this feature does not commit to building it.

### Trade-offs

- The flash mechanism added here is bespoke to the Sessions page. If (γ) lands later, the bespoke chrome line is replaced or absorbed. Accepting bespoke now keeps this feature small and shippable.
- The `has-session` call adds one tmux round-trip per Enter. Negligible — sub-millisecond locally, well within UI responsiveness.
- The pre-select calls remain best-effort (they could *still* fail intra-session for window/pane mutations); `has-session` only catches the whole-session case. The two checks compose: `has-session` first, pre-select swallowed-on-failure, connector last.

Confidence: high.

---

## Edge cases — filter-committed, zero matches, and stale row

### Context

Three remaining edge cases from the inbox and the review: (a) filter is committed and the previewed session no longer matches the filter; (b) filter would yield zero matches; (c) preview was opened on a row whose underlying session has shifted in the list (reorder, add, remove of other sessions).

### Decision

**All three reduce to the previously decided shape — no new behaviour is required.**

- **Stale row.** Enter attaches by *captured session name*, not by list-row position. Reordering of the Sessions list (whether from external mutation or filter dynamics) is invisible to the attach path.
- **Filter committed, previewed session no longer matches.** Same answer — Enter does not traverse the filtered list. Attach by name; after attach the TUI exits or `switch-client`s, and filter state is irrelevant.
- **Filter committed, zero matches.** Structurally impossible to reach from preview. To open preview the user highlighted a row, which means the filter had ≥1 match at preview-open. If matches subsequently went to zero (the previewed session was killed), that collapses into the **session-killed-externally** decision and is handled by `has-session` + flash.
- **In-flight filter input.** Cannot coexist with preview being open — preview owns the keymap once entered, so `KeyEnter` is dispatched to preview's `Update`, never to the filter input. Non-issue by construction.

Confidence: high.

---

## Keymap expansion policy

### Context

A stated secondary goal of this discussion. Lifting Enter from preview's "everything else is unbound/no-op" rule creates a slippery-slope question: where does the line sit for other Sessions-page keys with obvious analogues (`r` rename, `k` kill, etc.)? Defining the rule once is cheaper than re-litigating per key.

### Decision

**Strict view-only with Enter as the single exception. Preview is a verification surface, not a command surface.**

Owned preview keys, full list:

- `]` next window
- `[` previous window
- `Tab` next pane
- viewport-native scroll keys (passed through to `bubbles/viewport`)
- `Esc` dismiss back to Sessions list
- `Enter` (new) — commit attach with captured `(window, pane)` focus

Everything else is unbound or no-op. **`r`, `k`, and any future Sessions-page action keys are NOT inherited.** The user dismisses preview with `Esc` and acts from the Sessions page.

The rule, stated once for future referers:

> *Preview owns viewport-navigation keys and exactly one commit key (`Enter`). Every other action is "dismiss-then-act" via `Esc` + the Sessions-page binding. Proposals for new preview keys must argue the key is a verification primitive, not a convenience shortcut.*

### Journey

Considered B (per-session passthrough policy — inherit any Sessions-page key whose action is identity-bounded to the one previewed session). Rejected on two grounds:

1. **Destructive symmetry.** Under B, `k` (kill) becomes "see content → kill" in one keystroke from a viewer. The two-step `Esc` + `k` preserves a deliberate pause where the user's intent can survive. The deletion friction is a small but real safety net.
2. **Principle scales without re-litigation.** A's rule ("preview is verification + one commit key") is a clean test for any future key proposal. B's rule ("identity-bounded to the session") is fuzzier and invites the same per-key debate it tries to prevent.

User reinforced the framing: *"We're previewing here. It's okay to have the navigation surface area reduced. In fact, it's not only expected, but preferred."* The reduced surface area is the design intent, not a constraint.

### Trade-offs

- Tiny ergonomic cost for the "decide-while-looking" workflow that wants `k`/`r` immediately. Mitigated by Esc → key being two keystrokes vs one, with Esc explicitly marking the cognitive transition from "viewing" to "acting".
- Adding new preview keys later remains additive — the policy doesn't forbid expansion, only requires that proposals argue the verification-primitive test.

Confidence: high.

---

## Summary

### Key Insights

*(populated as discussion progresses)*

### Open Threads

*(populated as discussion progresses)*

### Current State

- Discussion just started — no subtopics decided yet
