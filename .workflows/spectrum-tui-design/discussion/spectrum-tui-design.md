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

  Discussion Map — ZX Spectrum TUI (28 subtopics — 27 decided · 1 converging)

  ┌─ ✓ Terminal theming & canvas ownership [decided]
  ├─ ✓ Direction & ambition (evolved → restrained-modern) [decided]
  ├─ ✓ Colour palette — Modern Vivid [decided]
  │  └─ ✓ Semantic colour roles [decided]
  ├─ ✓ Terminal-environment robustness [decided]
  │  ├─ ✓ Contrast floor [decided]
  │  ├─ ✓ Colour-capability ladder (truecolor/256/16) [decided]
  │  ├─ ✓ Narrow / short terminal behaviour [decided]
  │  ├─ ✓ NO_COLOR / monochrome degradation [decided]
  │  └─ ✓ AdaptiveColor binary light/dark — mid-tone & detect-fail [decided]
  ├─ ✓ PORTAL logo & header (wordmark + caret + separator) [decided]
  ├─ ✓ Spaced uppercase header treatment [decided]
  ├─ ✓ Cursor & selection (thick violet left bar) [decided]
  ├─ ✓ Status / footer & keybindings (? help modal) [decided]
  ├─ ✓ Borders & framing (no full frame; separators + per-element) [decided]
  ├─ ✓ Loading interstitial (combined: header + thick bar + tick-list) [decided]
  ├─ ✓ Startup flip (cold-path concurrent bootstrap + live progress) [decided]
  ├─ ✓ Grouped views (by project / by tag) & Projects page [decided]
  ├─ ✓ Modals — edit / kill / rename (MV) [decided]
  │  └─ ✓ Edit-modal interaction model (two-mode; immediate persist) [decided]
  ├─ ✓ Preview page (MV cyan chrome) [decided]
  ├─ ✓ Theming (tokenise in-scope · user-override deferred/logged) [decided]
  ├─ → Filtering (`/` live, orange) — two-mode boundary to lock [converging]
  ├─ ✓ Interaction conventions (focus/edit · per-page help · modals on blank) [decided]
  ├─ ✓ Animation & performance (minimal; idle-zero tick) [decided]
  ├─ ✓ Scope boundary (v1 vs deferred) [decided]
  ├─ ✓ Design reference & visual verification (Paper map + vhs harness) [decided]
  └─ ✓ Remaining UX states (empty · flash · signpost · command-pending) [decided]

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
- **Row content & layout (matches code):** name takes the **full-width left
  column** (flex); window-count and `● attached` are **fixed-width trailing
  slots pinned right**, each left-aligned so the counts line up and the lime
  bullets line up vertically down the list. (User preferred this over inline
  metadata.) Flat row = name only re: project — confirmed, no project column.
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

### AdaptiveColor binary classification — pending (review-002 F2/F6)
`AdaptiveColor` makes a **binary** light/dark choice from terminal-bg detection;
the real world is continuous. Two genuine risks:
- **Mid-tone backgrounds** (Solarized base03, Gruvbox soft-dark — *mainstream*,
  not exotic) are classified to an extreme they're not on, so a variant tuned for
  near-white/near-black may dip **below the contrast floor** on their actual bg.
- **Detection failure** (no OSC response over SSH / tmux passthrough; `COLORFGBG`
  unset) → termenv defaults (often dark), so a *light*-terminal user can be served
  the *dark* variant on a light bg — a cross-pairing the floor never tests.
  Acute because Portal runs **inside tmux**, where bg-detection passthrough is
  unreliable.
Mitigations to weigh in spec/planning: choose variants that also survive mid-tone;
a manual `--theme` / light-dark override; detect-and-degrade. Open.

### Review-002 dispositions (for the record)
F3 in-terminal validation → folded into the Judging & bail gate (step 3).
F4 monochrome role-separation → moot (chose multi-hue Modern-Vivid, not a
single-hue direction). F5 retro-vs-modern shortlist → resolved (user consciously
chose the restrained option; direction evolved, documented). F8 baseline owner /
F9 narrowing cardinality → resolved (user picks; narrowed to one front-runner).

---

## Other surfaces — grouped views & Projects (MV applied)

### Context
Beyond the flat Sessions list, the picker has two grouping modes (`s` cycles
Flat → by Project → by Tag) and a separate Projects page (`p`/`x`), all paginated
with their own keymaps. The Modern-Vivid identity must apply to all. Mocked for
sign-off (closes review-002 F7).

### By Project / By Tag (grouped Sessions)
Same chrome as flat Sessions (PORTAL header, separator, pagination, condensed
footer); the mode shows in the section header ("Sessions — by project" /
"— by tag"). Group headers are **dim, non-selectable** rows: `heading ··· N`
(dim heading + dimmer "··· count"). Session rows **indent** under their header;
same name(flex) + trailing window/attached layout + thick-violet-bar selection as
flat. By Tag groups by tag (a multi-tag session repeats under each — Pattern B)
with a pinned **Untagged** catch-all; By Project groups by project dir with an
**Unknown** catch-all. Mirrors the existing `HeaderItem` model (every row exactly
one delegate line — the load-bearing pagination invariant). Artboards:
`Sessions — by Project (MV)`, `Sessions — by Tag (MV)`.

### Projects page
Same chrome; section header "Projects N". **Two-line rows:** project name (bold
fg) over its path (dim). Selection = full-height violet left bar + row tint +
white name. Distinct footer (project keymap): `⏎ new session · s sessions ·
e edit · / filter · ? help` — full set (`d delete`, `n new in cwd`, nav) lives in
the `?` modal. Artboard: `Projects (MV)`.

### Notes
- The `?` help modal is **per-page** (Sessions vs Projects keymaps differ); only
  the Sessions variant is mocked so far.
- Project-row left bar is a 4px violet **bg column** (terminal-faithful — a column
  of coloured cells), spanning both text lines.

## Modals & preview (MV)

Mocked the remaining surfaces the user flagged (edit modal "really poor", kill
"not user-friendly", rename "okay but could be better", plus the preview screen).
All centred panels on a dimmed page (mocked on plain dark); `?` help modal is
per-page.

- **Edit project modal — full rethink.** Original was cramped with a confusing
  inline "(none) / Add:". New: a bordered panel with **labelled fields** (NAME /
  ALIASES / TAGS); the focused field gets a violet input border + caret;
  aliases/tags render as **removable chips** (`label ✕`; tags tinted green to
  match the attached/semantic green); a dim `+ add` affordance; footer
  `↵ save · tab next field · ↵ add chip · ✕ remove`. Honours the live-tag model
  (Tab cycles fields; Enter adds; ✕/x removes). Artboard: `Edit Project Modal (MV)`.
- **Kill confirm — destructive red.** `▲ Kill session?` with the session name in
  **red** (`#F7768E`), `· N window(s)`, a consequence line ("Ends the tmux
  session and all its panes. Can't be undone."), footer `y kill · n cancel ·
  esc`. Red is reserved for destructive actions. Artboard: `Kill Confirm Modal (MV)`.
- **Rename — cleaner.** Labelled `NEW NAME` input (focused violet), a `was:
  <old name>` context line, footer `↵ rename · esc cancel`. Artboard:
  `Rename Modal (MV)`.
- **Preview screen — cyan mode-chrome.** Read-only scrollback in a **cyan-framed**
  chrome (cyan = "peek mode", deliberately distinct from the violet main UI —
  preserves the `preview-visual-distinction` mode-signal intent in the new
  palette). Top bar: `⊙ preview <session> · Window x/y · Pane x/y` + nav hints
  (`↹ pane · ⏎ attach · ␣ back`); captured pane content rendered dim (read-only).
  Artboard: `Preview Screen (MV)`. (This restyles the existing chrome; the
  captured content itself is the real pane output, untouched.)

### Edit modal — interaction model (proposed, mocked)
The original modal's focus/keymap was ambiguous; worked it through:
- **Field traversal:** `Tab`/`Shift+Tab` cycle the three fields Name → Aliases →
  Tags → Name. **`←/→`** move *within* a chip field (across chips and onto the
  trailing `+ add` slot).
- **Tab into Aliases/Tags lands on the `+ add` slot** (input ready) — adding is
  the common action; `←` reaches the chips. So `Tab` from Aliases → Tags (next
  field); `→` is what reaches the next chip (e.g. fapi → v1).
- **Chip focused → `x` removes.** **No in-place edit** — change a chip by
  remove + re-add (short tokens; an edit sub-mode isn't worth it). No
  cursor-in-chip, no nested modal.
- **`+ add` is an inline input slot** (not a button / popup / pre-spawned empty
  chip): type, `↵` materialises the typed text as a chip and clears for the next.
- **`↵`** = commit the in-progress chip if an add-slot is being typed, else
  **save & close**. **`Esc`** = cancel & close.
- **Contextual footer** (the key fix): Name → `↵ save · ⇥ next field · esc`;
  chip → `✕ remove · ←→ move · ⇥ next field · esc`; add-slot → `↵ add · ←→ move ·
  ⇥ next field · esc`.
- **Persistence — PROPOSED CHANGE:** make all three fields **batch** (Enter saves
  all, Esc discards all) — the standard predictable modal contract — replacing
  today's asymmetric "tags persist live" behaviour (CLAUDE.md). Flagged for
  explicit confirm.
- Mocked states: `Edit Project Modal (MV)` (Name focused), `Edit Modal — chip
  focused`, `Edit Modal — adding tag`. Same contextual-footer principle applies to
  the other modals.

### Edit modal — model UPDATE (decided): in-place edit + immediate persistence
Supersedes the two flagged calls above (no-in-place-edit, batch-all). Final model
— **two modes applied uniformly to Name / Aliases / Tags:**
- **Navigate mode (default):** `Tab`/`Shift+Tab` between fields; `←/→` across chips
  + the `+ add` slot. Focused element shows a **focus highlight** (violet outline).
  `x` deletes a focused chip immediately. `Esc` **closes the modal**.
- **Edit mode (one element live):** entered by `Enter`/`e` on a chip, `Enter` on
  Name, or `+ add` → which **spawns a new empty chip already in edit mode** (looks
  like a normal chip + an **edit highlight** + live cursor). Type; `←/→` move the
  **text cursor within the value**. `Enter` commits & persists → back to navigate
  (focus highlight); `Esc` **discards that element's edit** (a brand-new empty chip
  vanishes) → back to navigate.

**Persistence = IMMEDIATE per item** (not batch). Each element persists on exit-
edit (`Enter`). Why this wins: it **dissolves the dirty-state + save-key problem**
— there's never an unsaved batch, so no dirty indicator and no save-key question.
Extends the codebase's existing tags-persist-live to Name + Aliases (consistent,
not a reversal).

Falling-out rules:
- **Empty-on-commit = delete** (new or existing chip); deleting a focused chip is
  immediate. Empty **Name** can't persist → reverts.
- **`Esc` backs out one level:** edit mode → discard element edit; navigate mode →
  close modal (all already saved).
- **Three chip visual states:** normal (subtle) · focused (violet outline) ·
  editing (edit highlight + cursor) — mode always legible.
- **Bundle, not split:** one modal for Name+Aliases+Tags is fine under this model
  (user weighed splitting; chose bundle).

**Mocked (rebuilt to the two-mode model; footers fixed — were squished):**
`Edit Modal — navigate (name)` (Name focused, footer `↵ edit · ⇥ next field · esc
close`), `Edit Modal — chip focused` (footer `↵/e edit · x remove · ←→ move ·
⇥ next field · esc close`), `Edit Modal — edit in place` (chip in edit mode:
filled edit-highlight + cursor `Fabric▌`, `◉ EDIT MODE`, footer `↵ save · esc
discard · ←→ cursor · empty on save = delete`). Adding a tag is the **same edit
mode on a new empty chip** — no separate state needed.

## Theming system

### Context
Raised by the user: rather than hardcoding the new colours, delegate them to a
**theme** — layout locked by this redesign, colours pulled from named tokens
(potentially a user-overridable theme file).

### Two levels (proposed split)
- **In scope for this feature — tokenise.** The redesign is *already* built on a
  role-token colour layer (primary / detail / state × light/dark — see Colour
  palette + Implementation feasibility). We structure those as a single named
  built-in **theme** ("Modern Vivid"): every renderer references **tokens**, not
  scattered hex. Locks layout, delegates colour. It's the foundation we need
  anyway — building on tokens makes the app theme-*ready* at near-zero extra cost.
- **Its own topic/initiative — user-overridable themes.** Loading an external
  theme file (e.g. `~/.config/portal/theme.{json,toml}`), merge-over-default,
  **validation** (a user can pick unreadable colours → contrast floor becomes
  advisory + warn, or clamp), multiple built-in themes, a `theme` setting, docs.
  Bigger surface; ships independently after the redesign.

### Recommendation
Build the redesign **token-based** (in scope) so it's theme-ready; **log the
user-overridable theme system as its own topic/idea** to scope separately.
Confidence: high on the split.

## Loading interstitial

### Context
Shown on first launch after a tmux/computer restart while bootstrap restores
sessions (subject to `LoadingMinDuration` = 1.2s). Today it's a plain centred
"Restoring sessions…". Designed in the Modern-Vivid language for consistency
with Sessions.

### Concepts built (Paper, round 1) — five treatments
1. **Block progress** — centred `PORTAL ▌`, violet block progress bar,
   "Restoring sessions · 8 / 12".
2. **Boot checklist** — `PORTAL ▌` + step list: green `✓` done / cyan `◐` active
   / dim `·` pending (Started tmux server → Registered hooks → Launched saver →
   Restoring sessions 8/12 → Replaying scrollback). Most informative.
3. **Minimal line** — `PORTAL ▌` + a thin violet/dim rule + "RESTORING SESSIONS".
   Ultra-restrained.
4. **Spinner** — `PORTAL ▌` + braille spinner + "Restoring sessions… 8 / 12".
   Compact.
5. **Percentage** — `PORTAL ▌` + big "67%" + block bar + "Restoring 12
   sessions…".

All terminal-faithful (block/box/braille glyphs, flat colour), neutral black.

### Feasibility — INVESTIGATED (route-changing)
Traced the bootstrap → loading-page flow (`cmd/root.go` `PersistentPreRunE` →
`cmd/open.go` TUI launch → `internal/tui/model.go` loading lifecycle →
`internal/restore`):

- **Crux:** the full 11-step bootstrap (incl. step 6 Restore) runs **synchronously
  to completion in `PersistentPreRunE`, BEFORE the TUI launches**
  (`cmd/root.go:157`, `cmd/open.go:529`). By the time the loading page renders,
  **restore is already 100% done**. The loading page is a **cosmetic 1.2s pad**
  (`LoadingMinDuration`): `BootstrapCompleteMsg` fires on the first tick, the
  page just waits out `LoadingMinElapsedMsg`. No channel/goroutine streams
  anything from bootstrap to the TUI; warnings are a static post-bootstrap
  snapshot.
- **Consequence:** a determinate bar would either flash to 100% instantly (work
  already done) or be **faked** (a 1.2s animation pretending to be progress —
  dishonest, and we've been holding to honest mocks). Worse: if a restore is
  *slow*, the slow part happens **before** the loading page even appears — so the
  loading page doesn't cover the slow moment at all.
- **Effort verdict:**
  - **(a) indeterminate spinner / line-sweep — SMALL** (~30 lines in
    `viewLoading`; the TUI tick already exists). No bootstrap change.
  - **(b) determinate percent / N-of-M — LARGE** (4–8h): requires decoupling
    bootstrap from `PersistentPreRunE`, running it **concurrently** with the TUI,
    streaming progress via `tea.Msg`/channel, injecting a callback into the
    restore loop (`restore.go:70-81` has the per-session loop but no emitter),
    and handling fatal-error + restore/daemon **race** risks. The synchronous
    design was deliberate (simple error handling; avoids restore/daemon races).
  - **(c) live step checklist — MEDIUM-LARGE** (3–6h): same concurrency
    restructure, per-step `tea.Msg` instead of per-session.
- **Secondary insight:** the concurrency restructure *does* carry a real,
  separate UX benefit — launching the TUI **immediately** (loading page first)
  while bootstrap runs behind it would replace the current "frozen terminal
  during a slow boot" with instant feedback. But that is its own initiative with
  race risk (cf. the prior slow-open/zombie-session incident), not a sub-task of
  a visual redesign.

**Reframe (user, accepted):** a loading screen is **UX**, not just UI — "honest
about what's happening" is its whole job. So the startup flip is legitimately
*in scope* for this redesign, not an unrelated side-quest. Re-costed on that
basis.

### Refined cost — the flip is more tractable than first stated
Two existing seams lower the cost from the blind "LARGE 4–8h":
- **`isTUIPath`** already exists (`cmd/root.go:205` = `open` with zero args) and
  already special-cases the TUI vs CLI paths (warning emission). So we defer
  bootstrap **only** for the TUI path; every CLI/direct-path command keeps the
  current synchronous bootstrap in `PersistentPreRunE`. No need to make *all*
  commands concurrent.
- The loading page **already gates** the Sessions enumeration on
  `BootstrapCompleteMsg`. So the "don't touch tmux until restore is done"
  invariant is already expressed; during loading the TUI is **inert** (animation
  only), which **contains the race surface** — the daemon/saver/restore steps run
  in the goroutine while the event loop only animates.

**Shape:** for the TUI path, launch Bubble Tea immediately (loading page), run the
orchestrator in a goroutine, stream `tea.Msg` per step (or per restored session),
transition to Sessions on complete, **quit-with-error** on the one fatal step.
Inject a progress callback at the restore loop (`restore.go:70-81`).

**Real costs / risks (not zero):**
- Rework how `serverStarted` + warnings reach the TUI (today via `context` +
  package-level memo sink set in `PersistentPreRunE`).
- Fatal-error-as-`tea.Quit` handling (today a `PersistentPreRunE` error return).
- Careful review of restore/daemon races with the event loop live — *load-bearing
  startup with prior-incident history* (the slow-open / zombie-session episode),
  so treat the estimate as having genuine variance.
- Integration-test updates around startup ordering (several subprocess tests).

**Estimate:** ~**1–1.5 days** incl. tests/race review — the single biggest
engineering item in the redesign; warrants **its own phase/PR** and the
in-terminal validation gate.

**Payoff if done:** unlocks an *honest* determinate loading screen (the **boot
checklist (2)** becomes genuinely meaningful — real steps completing), **and**
fixes "frozen terminal on a slow boot" with instant "Portal is starting"
feedback. Real UX win, consistent with the reframe.

### Cold vs warm — loading only on cold boot (CONFIRMED)
The loading page is gated on the **`serverStarted`** flag, not "every open":
`WithServerStarted(true)` is the *only* thing that sets the initial page to
`PageLoading` (`model.go:556-560`); `serverStarted` comes from `serverWasStarted`
→ the context value set when **EnsureServer actually had to start the tmux
server** (`cmd/open.go:136`). So:
- **Cold** (no tmux server): server started → full bootstrap (restore scrollback,
  register Claude-resume hooks, etc.) → **loading page shown**.
- **Warm** (server already up, sessions in progress, just opening another):
  `serverStarted=false` → bootstrap steps no-op (restore skips already-live
  sessions; hooks already registered) → **straight to the picker, no loading
  page**. This is the common case and it's already instant.

**Flip is therefore scoped to the COLD path only.** A cheap `tmux has-server`
check decides: warm → today's fast synchronous path, untouched; cold → launch the
TUI on the loading page immediately and stream bootstrap progress. The
common/warm case carries **zero new risk** — the refactor only touches the
once-per-reboot cold boot. This materially de-risks the change and fully honours
"don't show the loading screen every time."

### DECIDED — fold the cold-path flip; honest combined loading
The user **folded the startup flip in** (cold-path-only concurrent bootstrap with
live progress). The loading screen becomes genuinely honest/determinate, and it's
its **own phase** within this feature (planning to sequence; gated behind
in-terminal validation of the visual direction).

**Combined loading design (round 2):** centred `PORTAL ▌` header + a progress bar
+ a **tick-list that ticks off** as each boot step completes (`✓` done / `◐`
active / `·` pending) — a real list, *not* an in-place text swap. Friendly steps
(maps to the real bootstrap): Started tmux server → Registered hooks → Restoring
sessions (N/M) → Replaying scrollback → Resuming Claude sessions. Two bar weights
mocked for comparison — **thick block bar** (`Loading 6`) vs **thin line bar**
(`Loading 7`). **Bar weight: DECIDED — thick (`Loading 6`).**

Warm path unchanged: no loading screen, straight to the picker.

### Notes
Awaiting user pick. The checklist (2) maps naturally to the real bootstrap steps
and doubles as a "what's happening" surface if restore is slow.

## Keybindings (audited against code)

Per-screen keymap, verified in `internal/tui/model.go` + `pagepreview.go`:
- **Sessions (flat & grouped):** `↑↓`/`j`/`k` nav · `PgUp`/`PgDn` page · `g`/`Home`
  start · `G`/`End` end · `/` filter · `Enter` attach · `Space` preview · `s`
  cycle grouping (flat→project→tag) · `r` rename · `k` kill · `n` new-in-cwd ·
  `p`/`x` → Projects · `q` quit · `Esc` clear-filter/quit. Grouping adds no keys.
- **Projects:** nav/page/start/end · `/` filter · `Enter` new-session-from-project
  · `s`/`x` → Sessions · `e` edit · `d` delete · `n` new-in-cwd · `q` quit · `Esc`.
- **Preview:** `↑↓`/`PgUp`/`PgDn`/`Ctrl+U`/`Ctrl+D`/`j`/`k` scroll · `Home`/`End`
  top/bottom · `Tab` next pane · `]` next window · `[` prev window · `Enter`
  attach (this pane) · `Space`/`Esc` back.
- **Modals:** kill `y`/`n`/`Esc` · delete-project `y`/`n`/`Esc` · rename
  `Enter`/`Esc` · edit `Tab` cycle / `Enter` add-or-save / `x` remove / `Esc`.

**Key finding:** there is **no `?` help binding today** — `?` is actively
*swallowed* (so bubbles/list doesn't toggle its own help). The redesign's `?` help
modal therefore means **binding `?`** (new behaviour) + per-page help content.

**Mock corrections applied (audit caught my errors):** help modal had `x Kill`
— wrong: `x` is Projects, kill is `k`. Fixed to `k Kill` (red), added `p/x
Switch to Projects`, fixed `n → new session in cwd`. Preview chrome now includes
`[ ] window` nav (was missing).

## Implementation feasibility — "a lot of custom components?" (No)

Audited the render architecture. **The bespoke look is achievable mostly by
restyling existing hand-rendered Lipgloss — not by building widgets** — precisely
because today's TUI is already hand-rendered on top of `bubbles/list` (not an
off-the-shelf component kit).

- **Kept as-is (the engine):** `bubbles/list` provides the list model, pagination
  (the dots), filtering, cursor/selection, nav for Sessions & Projects. The
  CLAUDE.md build constraint holds (no `lipgloss/tree`; grouping stays Lipgloss in
  the delegate).
- **Restyle-existing** (edit current custom code + point at palette tokens): the
  row delegates (`SessionDelegate`/`ProjectDelegate`), the manual 3-column footer
  (→ condensed), the group `HeaderItem`, the kill/rename modals, the preview
  chrome (already hand-composed in `pagepreview.go`), the loading `viewLoading`.
- **New-but-small:** the header/logo + separator block above the list (~Lipgloss
  `JoinVertical`); edit-modal **chips** (restyle the alias/tag field render).
- **New-substantial (only one):** the **`?` help modal** — a new modal type +
  binding `?` (currently swallowed) + per-page help content (~60–80 lines), but it
  extends the existing rounded-border modal overlay primitive.
- **Cross-cutting foundation:** an `AdaptiveColor` **palette / role-token** layer
  (primary / detail / state, each with light+dark variants), contrast-floor
  adherence, and `NO_COLOR` handling. Moderate, touches every style — but it's
  *centralising* colour, not new widgets.
- **Separate engineering item (not a widget):** the **startup flip** (concurrent
  bootstrap + live progress) for the honest loading screen — ~1–1.5 days, its own
  phase.

**Bottom line:** ~80% is restyling already-custom render code; the only genuinely
new UI is the header block + chips (small) and the `?` help modal (moderate). The
real *engineering* chunk is the startup flip, which is plumbing, not components.
No widget framework needed.

## Interaction conventions (cross-cutting)

### Focus vs edit — unified visual language
Two states, identical grammar across the Name field, chips, and any editable
element:
- **Focused** (navigate): **outline only** — a violet ring, no fill change.
- **Editing** (cursor live): **filled violet background + cursor**, plus a
  `◉ EDIT MODE` indicator in the modal header. The **Name field in edit mode also
  turns violet-filled** (yes — the name goes purple, same treatment as chips).
- **So: outline = focused, fill = editing** — unambiguous everywhere.
- **Chips (aliases AND tags) are ONE neutral grey style** — identical to each
  other; **green is reserved for the `attached` state only, never chips.** Normal
  chip = grey, no `✕`. Focused chip = grey + **purple border + a purple `✕`** (the
  ✕ appears to show it's actionable; the `x` key removes it). Editing chip =
  **purple fill + cursor**, no `✕`. (Replaces an earlier green-tags / grey-aliases
  split that wrongly borrowed the attached-green and clashed.)

### Filtering (`/`)
- `/` opens an **inline filter input** in the section-header row (where the
  `/ to filter` hint sits). The query renders in a **bright-orange** accent
  (`#FF9E64` — new "filter/search" role token). The list filters **live as you
  type**; `↵` accepts (stay on filtered results, navigate); `Esc` clears. A
  `N matches` count shows at the right. Mocked: `Sessions — filtering (MV)`.
- The `/ to filter` hint shows top-right **consistently** on every session view
  (Flat / by Project / by Tag) and Projects — filtering works on all of them.
  `s switch view` lives in the **footer only** (removed from the header to avoid
  duplicating the footer).

#### Filter modes — REVISIT (to lock before spec) [circle back]
User correction: filtering has **two mutually-exclusive modes — never both at
once** (the current `Sessions — filtering (MV)` mock wrongly shows an active
cursor AND a selected row):
1. **Input-active** (typing): keystrokes go to the filter query; the **cursor
   sits at the end of the typed text**; the list updates live; **no list row is
   selected/cursored**. `↵` *or* `↓` **commits/locks the filter** → switches to
   list-active. `Esc` clears.
2. **List-active** (browsing the filtered results): the input row stays visible
   (locked query, **no cursor**) — proposed with a **faint orange background** on
   the input row to signal "this list is filtered"; arrows move the selection;
   `↵` **attaches**; `Esc` clears and returns.
States to mock on circle-back: (a) input-active (typing, cursor, no selection);
(b) **over-filtered / no matches** ("No matches" while filtering); (c)
list-active on a filtered list (faint-orange locked input + a selected row).
Why it matters: nailing the mode boundary now prevents implementation
ambiguity / unclean state / bugs.

### Page model — views vs pages
- **Sessions is ONE page with three grouping *views*** (Flat / by Project / by
  Tag), cycled by `s` — the same data pivoted, not separate pages.
- **Projects is a separate *page*** (different data + keymap), reached by `p`/`x`.
- **Preview** is an overlay screen (`Space`); **Loading** is the startup screen.
This taxonomy frames the spec's structure.

### `?` help — per-page contextual
`?` is bound on every page (not modals) and opens a help modal listing **that
page's** keymap (Sessions / Projects / Preview keymaps differ). One overlay
pattern, page-specific content.

### Modals render on a blank screen
When a modal opens, the page behind is **cleared to a blank screen** (modal
centred on black) rather than overlaying the dimmed list — the user finds this
cleaner. Our mocks already reflect this.

## Design reference (Paper)

All visual decisions are mocked in the Paper file **"Portal"** (page "Page 1",
`https://app.paper.design/file/01KVAT8NFHMBDTM4YY6V93R53S`), accessed via the
`paper` MCP (`get_basic_info` lists artboards; `get_screenshot` captures one by
id). **Canonical frames** (the decided Modern-Vivid design — the build targets):

- `Sessions — Modern Vivid v2` — flat sessions list (baseline screen)
- `Sessions — by Project (MV)` · `Sessions — by Tag (MV)` — grouping views
- `Sessions — filtering (MV)` — `/` filter active (orange query)
- `Projects (MV)` — projects page (two-line rows)
- `Loading 6 — Combined (thick bar)` — loading interstitial
- `Sessions — Help Modal (?)` — `?` keybindings overlay
- `Edit Modal — navigate (name)` · `Edit Modal — chip focused` · `Edit Modal —
  edit in place` — the three edit-modal states
- `Kill Confirm Modal (MV)` · `Rename Modal (MV)` — confirm / rename
- `Preview Screen (MV)` — scrollback preview (cyan mode-chrome)

All uniform 860×680, laid out as a 2-row grid (screens row / modals row) below the
exploration artboards. **Exploration frames** (the five colour directions, loading
concepts 1–5/7, Modern-Vivid v1) are kept above for reference only — NOT build
targets. This map is the carrier for spec → planning → implementation/review.

## Visual verification methodology

This redesign is predominantly look-and-feel, so each implementation task needs a
visual check against its Paper frame. Two layers:

- **Per-task review point (manual):** at each task's end the user inspects the
  rendered TUI against the named Paper frame. (Standard review gate; carried via
  the Design-reference map above.)
- **Programmatic capture — feasible, recommended.** A terminal TUI *can* be
  screenshotted headlessly. Best-fit first:
  - **`vhs` (charmbracelet/vhs)** — scripts a headless terminal via a `.tape`
    (set size · send keys · screenshot → PNG/GIF). Natural fit (Portal is a
    Bubble Tea / charm app); runs in CI. Drive Portal to a screen, capture a PNG.
  - **`freeze` (charmbracelet/freeze)** — render a command's output / ANSI to
    PNG/SVG; good for static frame snapshots.
  - **`tmux capture-pane -e -p`** — capture the live pane *with* ANSI colour as
    text (Portal already runs in tmux); cheapest, no image (pipe via `aha` →
    headless Chromium for a PNG if needed).
  - **Ghostty** is a terminal *emulator*, not a headless capturer — not the tool;
    `vhs`/`freeze` are.
  - **Recommendation:** a small **`vhs`-tape harness** (one tape per canonical
    screen) so each task self-captures a PNG that the agent/user compares to the
    Paper frame. Caveat: not pixel-perfect vs Paper (Paper is an HTML
    approximation; the real terminal uses the user's font/colours) — the check
    validates **layout, structure, colour-role intent**, which is the review need.
    Exact harness validated at implementation.

## Remaining UX states (designed)

Previously un-mocked states, now decided + mocked (Modern Vivid):
- **Empty sessions** — centred empty state: a dim block glyph, "No sessions yet",
  hint "Press n to start one in the current directory · p for projects"; footer
  reduces to the still-relevant keys (`n` / `p` / `/` / `?`). Artboard: `Sessions
  — empty (MV)`. **Empty projects** mirrors it ("No projects yet" + open-a-
  directory hint) — same pattern, not separately mocked.
- **Inline flash** — a transient band **directly under the title separator, above
  the section header** (list shifts down): **amber left-bar** + `⚠` + message
  (e.g. "folio-Jiz4el closed externally — list updated"); auto-clears. Success
  variant uses green. Artboard: `Sessions — inline flash (MV)`.
- **"No tags yet" signpost** — by-tag with zero tags: a **violet left-bar**
  signpost ("No tags yet — add tags in the project editor (e)…") over the flat
  list (degrade-with-message, not silent flatten). Artboard: `Sessions — no tags
  signpost (MV)`.
- **Command-pending banner** — Projects invoked to run a command: a **violet
  left-bar** banner ("Pick a project to run") with the command in an **orange
  chip**; footer becomes `⏎ run here · n run in cwd · esc cancel`. Artboard:
  `Projects — command pending (MV)`.

**Shared convention:** a **left-bar accent line** for inline notices — **amber**
= transient / warning, **violet** = mode / info. **Placement:** the band sits
**directly under the title separator, above the section header** (full-width
across the top); the section header + list shift down. The **command-pending**
screen keeps the **full Projects chrome** (green `Projects` header + `/ to
filter`) with the banner on top — not a stripped page. Consistent and
terminal-cheap. (Lists in these mocks use realistic Portal session names per the
naming convention.)

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
