# Discussion: Preview Visual Distinction

## Context

When the quick preview opens (Space on a session in the TUI), it visually looks indistinguishable from a fully-attached session. The scrollback fills the screen and reads identically to the attached state — there is no signal that this is a read-only, transient preview. Users need to be able to tell instantly that they are in preview mode and not actually inside the session.

Two directions were sketched in the inbox seed:

1. **Dim the preview content** — render the scrollback text at reduced contrast / lower opacity so it reads as inactive. Cheap, minimal layout change, no screen real estate cost.
2. **Bordered chrome around the preview** — wrap the content in a visible frame with header/footer regions for the session name and keybinding hints. More explicit and discoverable; takes screen space.

A combination is also possible (subtle border + slightly dimmed text). The goal is "obviously a preview" — not maximally decorated.

Relevant area: `internal/tui` preview page (`pagePreview` arm), rendered via `previewModel` injected with `TmuxEnumerator` + `ScrollbackReader` seams. Dimming would live in the lipgloss styling layer; chrome would mean wrapping the content area in an outer layout with header/footer regions.

Worth thinking about alongside the broader discoverability question for the preview feature — if we go the chrome route, the footer naturally doubles as the keybinding hint surface. There is a separate completed quick-fix (`preview-keymap-discoverability`) that may already address part of the hint problem; need to check what shape it took before deciding here.

### References

- Inbox seed: `.workflows/.inbox/.archived/ideas/2026-05-15--preview-visual-distinction.md`
- Related completed quick-fix: `preview-keymap-discoverability`
- Related completed feature: `session-scrollback-preview` (the feature this builds on)
- Related completed feature: `enter-attaches-from-preview`

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Visual treatment approach [pending]
  ├─ Dimming intensity / mechanism [pending]
  └─ Chrome composition (border + header/footer) [pending]

  Keybinding discoverability surface [pending]
  └─ Relationship to existing `preview-keymap-discoverability` work [pending]

  Header content [pending]
  └─ Session name, project, last-active hints [pending]

  Footer content [pending]
  └─ Available controls (Enter, Esc, Space) [pending]

  Behavior under terminal constraints [pending]
  └─ Narrow terminals, color-limited terminals, accessibility [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## Summary

### Key Insights

*(populated as discussion progresses)*

### Open Threads

*(populated as discussion progresses)*

### Current State

- Nothing decided yet — discussion just initialized.
