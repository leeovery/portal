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

  Visual treatment approach [decided] → border-only
  ├─ Dim-only [decided] → rejected
  ├─ Border-only [decided] → chosen
  └─ Combination [decided] → rejected

  Border composition [exploring]
  ├─ Chrome line: inside header vs above frame [pending]
  ├─ Border style (rounded / normal / thick) [pending]
  └─ Border color [pending]

  Session name visibility [pending]
  └─ Whether to surface session name on preview (currently shows window name only) [pending]

  Behaviour under terminal constraints [pending]
  └─ Narrow / short terminals — does the border cost too much? [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## Visual treatment approach

### Context

Preview's chrome line is a single row at the top. Underneath, the embedded `bubbles/viewport` renders raw scrollback bytes (ANSI passthrough). The body has no styling of our own — whatever colors and SGR sequences the session emitted are rendered verbatim. The question is what signal we add on top of that to make the page unambiguously read as "preview, not attached."

### Options Considered

**Dim-only — render the scrollback at reduced contrast.**
- Pros: zero screen-real-estate cost beyond the existing chrome line; minimal change to the layout; subtle.
- Cons: ANSI scrollback is already colored (vim, bat, git diffs, prompts). Reliably dimming a colored payload is harder than dimming plain text — naïve wrapper styles (e.g. lipgloss `Faint(true)` applied around the viewport) interact unpredictably with the embedded SGR sequences the viewport prints verbatim. Failure mode shows up months later on a specific user colorscheme. The fade is content-dependent rather than chrome-defined.

**Border-only — wrap the viewport in a visible frame.**
- Pros: the visual cue is *enclosure*, painted by Portal rather than by the session's own bytes, so it is reliable regardless of scrollback content. The existing chrome line tucks naturally into the frame's header region. Costs ~2 rows + 2 cols (≈4–8% of vertical space on typical 50/24-row terminals — negligible).
- Cons: takes screen real estate; the body of the preview still *renders* identically to attached — distinction comes purely from the surround.

**Combination (border + subtle dim).**
- Pros: maximally unambiguous.
- Cons: pays both costs (real estate + ANSI-interaction risk) for a signal one of them already provides.

### Decision

**Border-only.** Wrap the viewport in a visible frame; do not touch the body's rendering.

Decisive factor: the dim approach's failure mode is *content-dependent* — it works on a plain prompt and breaks on a tmux session full of `bat`, `vim`, or a colorful prompt — which is precisely the scrollback content preview is most useful for. The border approach is content-independent: it is Portal's paint over Portal's layout, and its appearance does not vary with what the session was doing. Real estate cost is modest and predictable; ANSI-interaction risk for dim is unbounded and only surfaces in the wild.

Confidence: high.

---

## Summary

### Key Insights

*(populated as discussion progresses)*

### Open Threads

*(populated as discussion progresses)*

### Current State

- Nothing decided yet — discussion just initialized.
