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

  Discussion Map — ZX Spectrum TUI (18 subtopics — 6 decided · 4 converging · 1 exploring · 7 pending)

  ┌─ ✓ Terminal theming & canvas ownership [decided]
  ├─ ✓ Direction & ambition (evolved → restrained-modern) [decided]
  ├─ → Colour palette — Modern Vivid front-runner [converging]
  │  └─ ✓ Semantic colour roles [decided]
  ├─ ◐ Terminal-environment robustness [exploring]
  │  ├─ ✓ Contrast floor [decided]
  │  ├─ ✓ Colour-capability ladder (truecolor/256/16) [decided]
  │  ├─ ○ Narrow / short terminal behaviour [pending]
  │  └─ ○ NO_COLOR / monochrome degradation [pending]
  ├─ → PORTAL logo & header (wordmark + caret + separator) [converging]
  ├─ → Spaced uppercase header treatment [converging]
  ├─ ✓ Cursor & selection (thick violet left bar) [decided]
  ├─ → Status / footer & keybindings (? help modal) [converging]
  ├─ ○ Borders & framing [pending]
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

### Options Considered
Three ambition levels presented (all theme-adaptive):
**1 — Retro-arcade:** keep the Spectrum soul but adaptive — block-letter logo,
chunky framed border, spaced uppercase headers, playful "HI-SCORE" status bar,
vibrant multi-colour accents. Bold/distinctive/nostalgic. Risk: retro can read
gimmicky, age faster; block fonts + spaced caps cost screen space.
**2 — Modern-polished:** clean/confident (lazygit/k9s/charm-tool vibe) — one or
two restrained accents, refined borders, subtle row highlight, tidy status bar.
Timeless, legible, less "fun".
**3 — Minimal accent:** lightest touch — accent colour + nicer cursor + a little
border polish on today's layout. Low risk, bail-friendly.

### Decision
**Direction 1 — Retro-arcade (adaptive).** The user wants a real, distinctive
identity, not a safe accent pass. Spectrum soul retained as flavour; executed
with adaptive colours so it respects terminal themes (per the canvas decision).
Confidence: **high** (explicit pick). This sets the bar for every contingent
subtopic: bold and characterful, but still readable — the open risk to manage is
retro tipping into gimmicky/noisy.

**Evolution (post-mockup, 2026-06-17):** seeing all five rendered, the user
gravitated to the *least* retro option (Modern-Vivid) for its restraint, then
asked to graft a few retro touches onto it (Amber-style header wordmark + block
caret + separator rule). So the direction has softened from "bold retro-arcade"
to **restrained-modern with light retro accents**. The retro-arcade label is
retained as lineage, but the working target is Modern-Vivid v2. Not a
contradiction — the mockups were exactly the instrument meant to let taste
correct the abstract pick.

---

## Colour palette (adaptive accents)

### Context
Under retro-arcade we want vivid, characterful colour — adaptive (light/dark)
and disciplined enough to stay readable in a tool opened many times a day.

### Journey
First proposal: **rainbow-as-signature, not rainbow-as-wallpaper** — vivid
multi-hue anchors (logo, separators, loading bar) over a *restrained* working
palette (one primary accent + existing semantic colours: green=attached,
grey=detail), vs a rainbow-everywhere maximalist version (every header a
different hue, cursor strobing colours) carrying readability/fatigue cost.

Before settling that, the user **dropped the rainbow concept entirely** — a
multi-hue rainbow reads too close to the pride flag, an association they
explicitly do not want. Colour stays in play; the *rainbow specifically* is out.
This drifts the identity further from literal ZX Spectrum (whose signature *is*
the rainbow stripe) — Spectrum is now loose inspiration at most.

Decided to stop discussing colour in the abstract and **visualise** instead:
research non-rainbow retro/TUI colour directions, mock ~5 variations in Paper
MCP, and feed the chosen direction back into this discussion.

### Decision (partial)
- **No rainbow / multi-hue spectrum motif** — firm (pride-flag association
  unwanted).
- Colour is still leveraged; positive palette direction **TBD via Paper
  mockups**.
- Confidence: high on the exclusion; open on the positive direction.

### Tooling note — Paper MCP for mockups
Paper renders web/app UI, not terminals — it can produce gradients,
anti-aliased fonts, sub-cell positioning, shadows, none of which a TUI can
render. **Guardrail:** constrain every mockup to terminal fidelity (monospace
grid, block/box-drawing characters only, flat per-cell fg/bg colour). Paper is a
visualisation aid only; all resulting decisions are documented back here.

### Research — non-rainbow retro directions
Web research + domain knowledge surfaced the well-trodden non-rainbow retro
palettes: phosphor monochromes (amber, green), synthwave/outrun (magenta+cyan),
vintage-micro palettes (C64), and the modern-vivid base16 families (Tokyo Night
/ Dracula / Catppuccin) that the best-looking real TUIs (btop, lazygit, k9s) use
with 24-bit truecolour. References below.

### Candidate directions for Paper mockups
Five to mock on the Sessions page (apples-to-apples colour comparison), each
terminal-faithful and `AdaptiveColor`-ready:

1. **Amber CRT** — single warm hue: amber/gold on near-black, hotter amber for
   cursor/selection. Calm, nostalgic, maximally readable. Adaptive: deep
   burnt-orange on cream. Risk: monochrome may not feel "colourful" enough.
2. **Green phosphor** — single cool hue: CRT green on black. Classic
   old-terminal/hacker feel. Risk: can read cliché / "Matrix".
3. **Synthwave / Outrun** — vivid duo: hot magenta + electric cyan over indigo
   structure on near-black. 80s arcade energy, bold, distinctive, zero rainbow
   association. Likely frontrunner for "exciting + retro + colour".
4. **C64 / vintage micro** — light-blue primary on deep blue-purple, cream text,
   one warm accent. Home-computer 8-bit feel; softer than synthwave.
5. **Modern-vivid (Tokyo Night family)** — one signature accent (vivid
   purple/teal) + tasteful semantic colours on a soft dark. Less retro; the
   restrained-colour comparison point — what proven beautiful TUIs actually do.

### Semantic colour roles — DECIDED
Design to **roles, not fixed hex**: a small fixed set — *primary accent* (cursor
/ selection / active title), *detail* (paths / secondary text), *state*
(attached, error/warning). Each direction instantiates the roles. **State is
never carried by hue alone** — always glyph + colour (e.g. a marker glyph for
attached, tinted only if colour is available), which makes the monochrome
directions and the parked `NO_COLOR` path work for free and protects colour-blind
users. **Existing colours are not sacred** — the user is open to a full
restructure of colour/layout/UI (and possibly UX); today's pink cursor /
green=attached / grey detail / blue preview border have no special claim and may
be replaced wholesale. Consistent with the prior `preview-visual-distinction`
decision ("don't rely on colour alone" for the quick-view border). Confidence:
high.

### Mockup approach (revised in session)
The user rejected "five recolours of today's layout" — they want **five
genuinely different designs**, each its own layout + structure + character +
palette. The current layout is only a content reference (what info must appear),
not a constraint.

**Canvas honesty (corrected mid-build):** an early pass painted each design on a
bespoke tinted background; the user caught that it contradicts the no-forced-
canvas decision. Corrected — the mockups now render **foreground-only on a
neutral black terminal** (a real "black terminal" the user would have, not a
Portal-painted tint). Per-element backgrounds (selected-row highlight, status
strip) are permitted — that's focus styling, not canvas ownership, and still
must pass the contrast floor on light *and* dark. Frames sized to a realistic
modern terminal (status bar pinned to the bottom; empty mid-screen is authentic
for few sessions).

**Built — round 1 (Sessions), Paper file "Portal":** five artboards — Amber CRT
· Green Phosphor · Synthwave · C64 Micro · Modern-Vivid. Palettes per the session
brief (Amber single-hue amber; Green single-hue phosphor; Synthwave magenta+cyan
neon; C64 light-blue+cream+gold; Modern violet+cyan+green).

**Finding from building:** directions whose identity depends on a *painted
background* lose richness foreground-only — Modern/Tokyo-Night most of all (it is
literally a background colourscheme), plus Synthwave's indigo and C64's blue. The
**robust** directions read as a pure *foreground* palette on the user's own
terminal: Amber, Green phosphor, and Synthwave's neon survive best. This is real
selection signal, surfaced only by building.

**Next:** user reactions → narrow; then five loading-page mockups.

### Round 2 — refined direction (Modern Vivid v2 + help modal)
User reactions narrowed to **Modern-Vivid as the base** (restrained
violet/cyan/green foreground palette), with grafted refinements:
- **Header:** Amber-style — uppercase `PORTAL` wordmark + block caret (`▌`) + a
  **separator rule** under it (the element Modern-Vivid lacked).
- **Cursor & selection — DECIDED:** a **thick violet left bar** (`▌`) at the
  far-left of the highlighted row (C64's chunkier block, not the thin `▍`), over
  a subtle row-tint highlight.
- **Footer & keybindings — converging:** the footer was running out of room for
  all binds. Decision pattern: footer shows only the **core** keys
  (navigate / open / filter / preview) + `? help`; the **full** keybinding set
  lives in a **`?` modal overlay**. Standard TUI idiom; solves the footer-space
  problem; the `?` modal was mocked (key/action two-column list, `x` in red,
  "esc to close").
- Built artboards: `Sessions — Modern Vivid v2`, `Sessions — Help Modal (?)`.

Still front-runner, not locked: loading-page mockups + in-terminal validation
remain before the colour direction is final.

### Round 2b — reality alignment (validated vs code)
The user showed the real Sessions screen. Validated against code (`tmux.Session`
= Name/Windows/Attached/Dir; `GenerateSessionName` = `{project}-{nanoid}`;
`internal/tui/session_item.go` delegate):
- **Names** are `{project}-{nanoid}` (6-char code) and freely **renameable**
  (arbitrary, may contain spaces) — highly variable length/format. Mock names
  corrected to realistic ones.
- **Directory:** every session carries `@portal-dir` (git-root at *creation*) →
  maps to a Project (name + tags). It is the session's **origin/project**, set
  once — NOT a live cwd, and a session may span multiple pane dirs. The flat list
  **deliberately omits it**; the project/tag dimension is surfaced via the `s`
  grouping modes (By Project / By Tag). The earlier "path column" was both
  fabricated and redundant — **removed**.
- **Row content (matches code):** name · window count · `● attached`.
- **Pagination:** `bubbles/list`'s built-in paginator (height-driven dots). Mock
  now shows **centred dots** above the footer; list lengthened so paging is
  visible.
- **Principle reinforced:** validate against how Portal actually works before
  designing; code *can* change for good UI, but "the juice must be worth the
  squeeze" — no gratuitous restructure.

**Judging & bail gate** (folds review-001 F6/F9/F12):
1. **Objective** — each direction must clear the contrast floor or it is out.
2. **Taste** — the user judges whether any survivor is genuinely "more exciting /
   nicer to use" enough to ship. If none clears that bar → **bail** (explicit
   anti-sunk-cost gate; "better" = passes the floor AND beats today on the user's
   subjective read).
3. **Validation** — the chosen direction is a *hypothesis* until **prototyped in
   a real terminal** (Lipgloss output, inside tmux); only then is it locked.

---

## Terminal-environment robustness

### Context
The canvas decision (no forced background) means Portal's appearance is now a
function of an **unknown, user-controlled environment**: background colour,
colour depth, terminal size, font, and `NO_COLOR`. The redesign must survive
that whole space, not just look good on one mock. Raised as a cluster by
review-001 and promoted to its own subtopic — it is the largest untouched risk
area created by the canvas decision.

### Contrast floor — DECIDED
Every candidate direction must clear a hard contrast gate **before taste is
judged**. Functional foreground (session names, paths, footer, status text) must
meet WCAG AA — **4.5:1** normal text, **3:1** large/bold text and UI accents
(cursor, border, selection highlight) — against **both** a canonical light
background (≈ white) and a canonical dark background (≈ black). With
`AdaptiveColor` that means: the light-variant tested on white AND the
dark-variant tested on black, each ≥ the ratio. Purely decorative glyphs (logo)
are exempt from the text ratio but must stay visible. Arbitrary mid-tone custom
backgrounds are **out of scope** — we target the standard light/dark cases
`AdaptiveColor` flips between; we can't guarantee every exotic user background.
A direction that can't hit the floor on both extremes is disqualified before we
judge looks.
- **Rationale:** turns "is it readable?" from hope into pass/fail; stops a
  mock-approved direction failing on a real user's theme. Confidence: high.

### Colour-capability ladder (truecolor / 256 / 16) — DECIDED
**Impose our own exact hues via truecolor `AdaptiveColor`**, not inherit the 16
ANSI colours.
- **Rationale:** a recognisable identity needs consistent hues across machines;
  inheriting the user's palette means no identity and possible clashes.
  Respecting the *background* (decided) plus honouring `NO_COLOR` (parked)
  already covers "don't fight the user" — imposing *hues* doesn't conflict with
  that distinction. Lipgloss/termenv auto-downsamples to 256/16 on weaker
  terminals; we accept graceful degradation (a hue may approximate, but the
  contrast floor still governs legibility). Matches existing repo practice
  (`previewBorderColor`). Confidence: high.

### Narrow / short terminal behaviour — pending
Chunky chrome (block logo, framing, spaced headers, status bar) competes for rows
and columns that may not exist in a small tmux split. Needs a minimum supported
size and a degrade strategy (e.g. drop the logo below N columns). Layout concern
— does NOT block the colour mockups; take it with the chrome subtopics.

### NO_COLOR / monochrome — pending
A colour-led identity needs defined behaviour when colour is suppressed
(`NO_COLOR` convention), unavailable (monochrome terminal), or piped/redirected,
and how state (e.g. attached) is still conveyed without colour. Degradation
concern — does NOT block the colour mockups; settle later.

---

## Summary

### Key Insights
1. The Spectrum proposal separates cleanly into a **colour scheme** (the only
   preference-fighting part — dropped) and **structure/typography** (logo,
   borders, headers, status bar, cursor, loading — theme-agnostic, kept). You
   get most of the "exciting" without owning the canvas.
2. Identity has drifted from literal "ZX Spectrum" to "colourful, characterful
   retro-ish TUI." Two signature ZX motifs are now explicitly OUT: forced black
   canvas, and the rainbow. Spectrum is loose inspiration, not a spec.
3. Colour direction is hard to settle verbally — moving to concrete Paper
   mockups to decide.
4. The canvas decision's hidden cost: appearance now depends on an unknown
   user environment (bg, colour depth, size, NO_COLOR). "Terminal-environment
   robustness" captures that; a **contrast floor** is the first gate, and the
   mockups must clear it before taste is judged.
5. Nothing in the current UI is sacred — the user is open to a full restructure
   (colour/layout/UI, possibly UX). Mockups may propose a *new* baseline layout,
   not just recolour today's. Colour decided by role (state glyph-backed), not
   fixed hex.

### Open Threads
- Bail is explicitly acceptable if the redesign doesn't earn its place (now a
  concrete gate — see Mockup plan).
- Animated cycling-colour border noted in seed as possible-but-likely-overkill.
- **(review-001 → chrome stage) Pagination invariant:** new framed border /
  status bar must recompute the list viewport height so "one row = one delegate
  line" still holds; `HeaderItem` stays one line and non-selectable. (F4)
- **(review-001 → chrome stage) Logo fidelity:** block-glyph logo is
  font-dependent; need a plain-text wordmark fallback for fonts lacking the
  glyphs. (F7)
- **(review-001 → chrome stage) Animation cost:** idle CPU of a strobing
  cursor/border in an always-open tool; non-TTY / CI / unfocused behaviour. (F5)
- **(review-001 → scope) Page coverage:** decide whether the retro chrome applies
  to all four pages or selectively. (F10)
- **Surface project/tags in the flat session row?** Useful for renamed sessions
  whose name hides the project; but grouping (By Project / By Tag) already serves
  it. Leaning **keep flat = name only**; revisit if wanted.

### Current State
- **Decided:** respect terminal theme / no forced canvas / adaptive colours;
  retro-arcade direction; no rainbow motif; contrast floor (WCAG AA both
  extremes) as a hard mockup gate; truecolor adaptive hues (impose, don't
  inherit), graceful downsample.
  Also decided: semantic colour roles (state always glyph-backed); existing
  colours/layout not sacred (restructure on the table); cursor/selection = thick
  violet left bar.
- **Front-runner:** Modern-Vivid v2 (restrained violet/cyan/green foreground;
  Amber-style header + separator; thick left-bar selector; condensed footer +
  `?` help modal). Direction softened from bold-retro to restrained-modern.
- **Exploring/converging:** header/footer/keybindings detail; terminal-
  environment robustness (narrow-terminal, NO_COLOR still open); loading-page
  designs next; in-terminal validation before lock.

## Triage

(none)
