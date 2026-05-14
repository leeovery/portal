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

  Enter target: session vs focused pane [pending]

  Transition mechanics: instantaneous vs two-beat dismiss-then-attach [pending]

  Mid-load / placeholder behaviour [pending]

  Edge cases [pending]
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

## Summary

### Key Insights

*(populated as discussion progresses)*

### Open Threads

*(populated as discussion progresses)*

### Current State

- Discussion just started — no subtopics decided yet
