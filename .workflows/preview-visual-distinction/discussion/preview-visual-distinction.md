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
  ├─ Chrome line: inside header vs above frame [decided] → top header
  ├─ Width cascade / truncation [decided] → cascading degradation
  ├─ Border style [decided] → RoundedBorder (matches modal)
  └─ Border color [pending]

  Session name visibility [pending]
  └─ Whether to surface session name on preview (currently shows window name only) [pending]

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

## Chrome line composition and width cascade

### Context

With the border-only direction locked, two layout questions follow immediately: (a) where the existing chrome line sits relative to the new frame, and (b) how the chrome behaves when terminal width can't accommodate it. The chrome today (`internal/tui/pagepreview.go:165-175` → `chromeLine()`) is `Window M of M · Pane X of X · win: {name}    ] next win · [ prev win · tab next pane · enter attach · esc back` — roughly 110 chars of fixed overhead plus a variable-length window name. There is no width-awareness today; long window names or narrow terminals already wrap to a second visual row in option A's structure, just silently.

### Options Considered

**Layout — chrome above the frame (A) vs chrome as the frame's top header (B).**

- **A — chrome above frame**: structurally simpler in lipgloss. Chrome `Render()`s independently; frame surrounds only the viewport. Overhead: chrome row + top border row = 2 rows. Overflow failure mode: chrome wraps to a second visual row, pushing the viewport down.
- **B — chrome in top header**: the metadata strip becomes part of the frame edge (e.g. `┌─ window 1 of 3 · pane 1 of 1 · win:nvim · ] next win · … esc back ─┐`). Overhead: 1 row. Reinforces "this is one contained preview surface." Overflow failure mode: the corner character clips or wraps, breaking the entire border integrity — strictly worse than A's wrap, *unless* width-handling exists.
- Lipgloss has no first-class label-in-border primitive; B requires assembling the top edge manually (corner + chrome chars + corner). One-time, bounded work.

**Width handling — none vs cascading truncation.**

- **No width handling**: existing behaviour. A wraps silently; B breaks.
- **Cascading truncation**: a pure function `composeChromeLine(width int, …) string` that applies degradation in order until the line fits.

### Journey

Initial lean was B for the visual gestalt — single bounded preview surface, the metadata strip reads as a *label of* the thing rather than a *line above* the thing. Concern raised: B's overflow failure is worse than A's, so it's only viable if width can be respected.

This pivoted the conversation: width handling isn't a B-specific safety net — A also benefits today (long window names wrap and push the viewport down silently). So the truncation cascade is a real robustness improvement either way, and adopting B just makes it load-bearing rather than nice-to-have. Implementation is bounded: pure function, no I/O, exhaustively unit-testable at width thresholds. The previewModel already receives terminal width via `tea.WindowSizeMsg` (needed to size the viewport), so the data is available; no new model surface.

False path: briefly considered "drop chrome above some narrow-terminal threshold and let viewport fill the frame." Rejected as a primary strategy — the chrome is the only navigational discoverability inside preview; dropping it should be the absolute last-resort fallback, not the first response to narrowing.

### Decision

**Layout: B — chrome line as the frame's top header.** Implemented by composing the top edge manually (`┌─ … ─┐`) rather than reaching for a lipgloss primitive that doesn't exist. Frame surrounds the viewport; bottom edge is the standard lipgloss border.

**Width handling: cascading degradation**, applied in order until the assembled line fits the available width (measured with `lipgloss.Width`):

1. **Truncate window name with `…` suffix** when the budget for the name segment is positive but smaller than the name.
2. **Drop the `· win: {name}` segment** entirely if budget for it is below a sensible minimum (target: ~8 chars; below that the truncation reads as garbage).
3. **Swap full keymap for compact form** — `] [ tab enter esc` instead of the verbose `] next win · [ prev win · tab next pane · enter attach · esc back`. Saves ~50 chars. Labels are not lost from the product — the bottom help bar still carries the verbose form on the Sessions page; preview's chrome is just a hint surface here.
4. **Drop chrome entirely** — render the frame with no header label. Strictly a degenerate-terminal fallback; almost no real user terminal hits this.

`composeChromeLine` is a pure function in `internal/tui/pagepreview.go`. Tested at each cascade threshold with table-driven cases.

Side benefit: defends against pathological window names regardless of terminal width — e.g. a long file path as a vim session's window name no longer breaks rendering today.

Confidence: high.

---

## Border style

### Context

`lipgloss` ships several border presets (`NormalBorder`, `RoundedBorder`, `ThickBorder`, `DoubleBorder`, `BlockBorder`, `HiddenBorder`). Portal currently uses borders in exactly one place: `internal/tui/modal.go:24` uses `RoundedBorder()` for kill/rename/edit modal overlays. No other styles are in use.

### Decision

**`lipgloss.RoundedBorder()`** — matches the existing modal precedent.

**Rationale**:

1. Introducing a second border style would silently establish a new design rule in Portal ("each contextual surface has its own border style"). The current implicit rule is simpler: rounded border = contextual surface, no border = main page. Preview is a contextual surface, so it fits.
2. Geometry already differentiates preview from modals — modals are small centered overlays, preview is a full-width framed page. They will never be visually confused even with identical border characters.
3. Rounded corners read more cleanly at small column widths than `ThickBorder`'s heavier glyphs.

A coupling worth naming for the build phase: the manually-composed top edge (chrome-in-header) must use the same character set as `RoundedBorder`'s left/right/bottom edges so the corners align. Implementation must source corner/edge characters from the chosen lipgloss border value rather than hardcoding them — that way a future style switch would be a one-line change.

Confidence: high.

---

## Summary

### Key Insights

*(populated as discussion progresses)*

### Open Threads

*(populated as discussion progresses)*

### Current State

- Nothing decided yet — discussion just initialized.
