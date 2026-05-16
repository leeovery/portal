# Discussion: Preview Visual Distinction

## Context

When the quick preview opens (Space on a session in the TUI), it visually looks indistinguishable from a fully-attached session. The scrollback body fills the screen and reads identically to the attached state — there is no signal that this is a read-only, transient preview. Users need to be able to tell instantly that they are in preview mode and not actually inside the session.

### What already exists

Preview already has a **single-line chrome strip** at the top of the page (`internal/tui/pagepreview.go` → `chromeLine()`), rendered above the embedded `bubbles/viewport`. Today it reads:

`window M of N · pane X of Y · win:{name} · ] next win · [ prev win · tab next pane · enter attach · esc back`

This was iterated up by two completed pieces of work:

- `preview-keymap-discoverability` (quick-fix, 2026-05-14) — annotated bare key tokens with short action labels and added the `win:` prefix on the window name so it is not mistaken for a stray number.
- `enter-attaches-from-preview` (feature, 2026-05-15) — added the `enter attach` token to the chrome and the `Enter` binding behind it.

So discoverability of the *keymap* is already handled. The remaining gap — what this discussion is about — is the **body** of the preview: the scrollback content underneath the chrome line still looks identical to an attached session.

### The seed proposals

Two directions were sketched in the inbox:

1. **Dim the preview body** — render the scrollback text at reduced contrast / lower opacity so it reads as inactive. Cheap, minimal layout change, no screen real estate cost beyond the existing chrome line.
2. **Bordered chrome around the preview body** — wrap the viewport content in a visible frame, with the existing chrome line living inside the frame's header. More explicit; takes screen real estate.

A combination is also possible (subtle border + slightly dimmed body). The goal is "obviously a preview" — not maximally decorated.

### Relevant code surface

- `internal/tui/pagepreview.go` — `pagePreview` arm of the page state machine, peer of `pageFileBrowser`. Owns a `bubbles/viewport` and the chrome line.
- `internal/tui/previewmodel` (constructor-injected with `TmuxEnumerator` + `ScrollbackReader` seams).
- Dimming would live in the lipgloss styling layer applied to the viewport content (or via the viewport's `Style` field).
- Chrome wrapping would mean introducing an outer layout wrapper around the viewport (likely via `lipgloss.NewStyle().Border(...).Render(...)` around the composed top-chrome + viewport block).

### Related work not in scope

- `general-tui-flash-infrastructure` (inbox idea, 2026-05-14) — a project-wide flash/toast primitive deferred from `enter-attaches-from-preview`. Orthogonal — not about visual identity of the preview surface.
- `tui-redesign` (cancelled feature) — earlier broader visual reskin of the TUI; orthogonal, intentionally not revived.

### References

- Inbox seed: `.workflows/.inbox/.archived/ideas/2026-05-15--preview-visual-distinction.md`
- Completed quick-fix: `.workflows/preview-keymap-discoverability/`
- Completed feature: `.workflows/session-scrollback-preview/` (the feature this builds on)
- Completed feature: `.workflows/enter-attaches-from-preview/`

## Discussion Map

### States

- **pending** — identified but not yet explored
- **exploring** — actively being discussed
- **converging** — narrowing toward a decision
- **decided** — decision reached with rationale documented

### Map

  Visual treatment approach [pending]
  ├─ Dim-only [pending]
  ├─ Border-only [pending]
  └─ Combination (dim + border) [pending]

  Dimming mechanism and intensity [pending]
  ├─ How to dim ANSI scrollback (overlay vs reset vs faint SGR) [pending]
  └─ Intensity target on dark/light themes [pending]

  Border composition (if used) [pending]
  ├─ Border style and color [pending]
  └─ Relationship to existing chrome line (above border vs inside header) [pending]

  Session name visibility [pending]
  └─ Whether to surface session name on preview (currently shows window name only) [pending]

  Behaviour under terminal constraints [pending]
  ├─ Narrow / short terminals — does the border cost too much? [pending]
  ├─ Low-color terminals — does dim degrade gracefully? [pending]
  └─ Accessibility — text remains legible at chosen dim level [pending]

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
