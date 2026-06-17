# Discussion: ZX Spectrum-Inspired TUI Design

## Context

Portal's TUI is currently functional but personality-free. The seed proposed a
ZX Spectrum aesthetic (rainbow primaries on black, block logo, chunky borders,
spaced uppercase headers, cycling block cursor, Manic Miner-style status bar).

**Reframed in session (2026-06-17):** the user widened the goal. The real want
is *"make Portal's UI more colourful / exciting / nicer to use from a design
perspective,"* **without going against the user's terminal preferences.** ZX
Spectrum is now treated as *inspiration, not a literal spec.* Bailing on the
redesign entirely is explicitly on the table — the bar is "is this actually an
improvement worth shipping."

**Current state (baseline):**
- No visual identity. Pink cursor (`lipgloss.Color("212")`), grey detail text
  (`#777777`), green "attached" marker (`76`).
- Rounded borders used only on the modal (`modal.go`) and the scrollback
  preview chrome (`pagepreview.go`, adaptive blue `#3B5577`/`#7B95BD`).
- Loading page is a plain centered string `"Restoring sessions…"`
  (`viewLoading`), subject to `LoadingMinDuration = 1.2s`.
- Session-list title is plain text with mode suffixes (`Sessions` / `Sessions —
  by project` / `Sessions — by tag`) via `sessionListTitleForMode`.
- Footer is a manually-rendered three-column keymap (`renderKeymapFooter`); the
  bubbles/list built-in help renderer is disabled.
- Grouping renders real `HeaderItem` rows interleaved into the `bubbles/list`
  delegate — every row is exactly one delegate line (load-bearing for
  pagination; the grouped-viewport-overflow incident is documented in CLAUDE.md).

### References

- Seed: `seeds/2026-03-19-spectrum-tui-design.md` (inbox:idea)
- Discovery: `discovery/session-001.md`
- Prior art in-repo: `preview-visual-distinction` spec established
  `AdaptiveColor` usage + manual border-row composition in `pagepreview.go`.
- Stack: Bubble Tea (TUI) + Lipgloss (styling) — colours, block characters,
  borders, tick-based animation all supported.

## Discussion Map

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — ZX Spectrum TUI (12 subtopics — 1 decided · 1 exploring · 10 pending)

  ┌─ ✓ Terminal theming & canvas ownership [decided]
  ├─ ◐ Direction & ambition [exploring]
  ├─ ○ Colour palette (adaptive accents) [pending]
  ├─ ○ PORTAL logo [pending]
  ├─ ○ Borders & framing [pending]
  ├─ ○ Spaced uppercase headers [pending]
  ├─ ○ Cursor & selection treatment [pending]
  ├─ ○ Status bar [pending]
  ├─ ○ Loading interstitial [pending]
  ├─ ○ Modal accent [pending]
  ├─ ○ Animation infra & performance [pending]
  └─ ○ Scope boundary (v1 vs deferred) [pending]

*Subtopics 4–11 are contingent on the Direction & ambition decision — their
shape depends on how far the redesign pushes.*

---

## Terminal theming & canvas ownership

### Context
The ZX Spectrum identity is defined by bright saturated colour on a *black
canvas*. A TUI doesn't own its background by default — Lipgloss paints
foreground colour onto the terminal's existing background. So the literal
aesthetic forces a choice: does Portal paint its own black canvas, or adapt to
the terminal?

### Options Considered
**A — Own the canvas:** paint a black backdrop across the full alt-screen;
aesthetic identical everywhere.
- Pros: literal Spectrum look guaranteed on any terminal.
- Cons: overrides light-terminal users' deliberate themes; full-bleed background
  painting must fill *every* cell or the terminal bg leaks at the seams.

**B — Adapt to the canvas:** `AdaptiveColor` (light/dark variants), respect the
terminal background.
- Pros: plays nice with the user's theme; reuses the in-repo
  `previewBorderColor` pattern.
- Cons: cannot be literal "rainbow on pure black" on a light terminal.

### Journey
Opened arguing for **A** — the identity *is* the black canvas, and adapting felt
like a false comfort that trades the feature's whole reason for existing in
order to stay polite. The user rejected the premise: they will not override
user preferences, and — more importantly — reframed the goal away from literal
Spectrum toward general "more colourful / nicer" with Spectrum as inspiration.
That collapses the A-vs-B tension entirely: if literal rainbow-on-black isn't a
hard requirement, there is no reason to fight the terminal. Key realisation that
unlocked it: the Spectrum proposal's *elements* (logo, borders, spaced headers,
status bar, cursor, loading screen) are structure/typography and are
theme-agnostic — only the literal pure-black colour scheme needs canvas
ownership.

### Decision
**Respect the terminal background. Do NOT force a black canvas.** Use adaptive
accent colours (light/dark variants) so the redesign works on any terminal
theme. Drops literal "bright rainbow on pure black" as a goal; keeps everything
else viable. Confidence: **high** (explicit user steer). Consequence: "Spectrum"
is now inspiration/flavour, not a literal spec — carried into Direction &
ambition.

---

## Direction & ambition

### Context
With the canvas decision made, the open question is *how far* the redesign
pushes — from a bold distinctive retro identity down to a light tasteful accent
pass (or bail). This is a taste/ambition fork that shapes every contingent
subtopic below it.

### Journey
*(in progress — presenting three concrete directions for the user to react to)*

---

## Summary

### Key Insights
1. The Spectrum proposal separates cleanly into a **colour scheme** (the only
   preference-fighting part — dropped) and **structure/typography** (logo,
   borders, headers, status bar, cursor, loading — theme-agnostic, kept). You
   get most of the "exciting" without owning the canvas.

### Open Threads
- Bail is explicitly acceptable if the redesign doesn't earn its place.
- Animated cycling-colour border noted in seed as possible-but-likely-overkill.

### Current State
- **Decided:** respect terminal theme, no forced canvas, adaptive colours.
- **Exploring:** overall direction & ambition level.

## Triage

(none)
