# Specification: Spectrum TUI Design

## Specification

## 1. Overview & Design Direction

### Goal
Portal's TUI is functional but personality-free. This redesign gives it a colourful, characterful visual identity that makes Portal nicer and more exciting to use **without overriding the user's terminal preferences**. The shipping bar is concrete: the result must be a genuine improvement over today's UI — both *objectively* (clears the contrast floor, §2) and on the user's *subjective* read. Bailing is a legitimate outcome if no direction clears that bar; this is an explicit anti-sunk-cost gate.

### Locked direction — Modern Vivid
The visual language is **Modern Vivid (MV)**: a restrained, modern palette (violet / cyan / green accents, plus an orange filter accent) with light retro touches grafted on (wordmark + caret + separator rule). It descends from a "ZX Spectrum" inspiration but is explicitly **not** a literal Spectrum reproduction. Two signature Spectrum motifs are **out**:

- **No forced black canvas** — Portal does not paint its own background (see canvas ownership below).
- **No rainbow / multi-hue spectrum motif** — the multi-hue rainbow is firmly excluded (unwanted pride-flag association). Colour is still used heavily; it is just never a rainbow.

Spectrum is loose inspiration only. The redesign keeps the structural/typographic ideas (wordmark, separators, spaced headers, chunky selector, honest loading screen) — which are theme-agnostic — and drops the literal colour scheme.

### Canvas ownership — respect the terminal background
Portal renders **foreground-only on the user's existing terminal background**, using adaptive (light/dark) accent colours so the redesign works on any terminal theme. Per-element backgrounds (selected-row tint, status strips, modal panels) are permitted — that is focus styling, not canvas ownership — but each must clear the contrast floor (§2) on **both** light and dark backgrounds. Portal never fills the full alt-screen with a bespoke colour.

### Nothing is sacred
The current UI carries no special claim. Today's pink cursor (`212`), green=attached (`76`), grey detail text (`#777777`), and blue preview border may all be replaced wholesale. The redesign may restructure colour, layout, and UI — and, where justified, UX — but only where "the juice is worth the squeeze." Code may change in service of good UI; gratuitous restructure is avoided. Every design decision is validated against how Portal actually works before being adopted.

### Reskin, not rebuild (applies throughout)
This feature is a **visual reskin** of the existing TUI, **not** a reimplementation. The specification describes the **styling target** (palette tokens, glyphs, layout treatment) and the **behaviour it must preserve** — it is not a rebuild brief. The existing list model, grouping logic, directory resolution, persistence, resolution chain, and keybinding plumbing are **already implemented and must stay behaviourally identical**; implementation **restyles existing render code** (per the feasibility audit §14) rather than re-deriving the machinery.

**Changing existing code is explicitly permitted** where a restyle genuinely requires it — reworking a row delegate, centralising colour into tokens, adjusting a layout/measurement calc are all in-bounds; a reskin is not limited to additive styling. The bar is **like-for-like behaviour**: whenever existing code is touched, verify the new behaviour against the current implementation (read it, trace every path, diff the logic) so the change is **provably cosmetic**. Behaviour parity — not "did we avoid touching the file" — is the acceptance test.

Where a section documents existing behaviour, treat it as a **constraint to preserve**, not a task to build. Genuinely **new** work is flagged explicitly and is limited to: the `?` help modal, the header/wordmark + separator block, edit-modal chips, the `AdaptiveColor` token layer, and the cold-path startup flip (§10). **If implementing a section would mean re-deriving logic that already exists, restyle in place and verify parity instead of rebuilding** — that is the anti-bug guardrail.

---

## 2. Colour System & Terminal Robustness

### 2.1 Design to roles, not fixed hex
Every renderer references a small fixed set of **semantic role tokens**, never scattered literal hex. The redesign is built on this role-token colour layer; concrete values are pinned in §2.9 (MV token table).

Roles:
- **primary accent** — cursor / selection / active title / header caret. *(MV: violet.)*
- **key-hint** — footer / modal key glyphs. *(MV: blue.)*
- **detail** — paths / secondary text / metadata / counts, across a **tonal ramp** (bright → faint; pinned in §2.9). *(MV: blue-greys.)*
- **state — attached** — the live/attached marker. *(MV: green; reserved for the attached state only — never reused for chips or decoration.)*
- **state — destructive** — kill/delete confirmation emphasis. *(MV: red; reserved for destructive actions only.)*
- **filter / search & warning / transient** — filter query and inline flash share one warm token. *(MV: orange.)*
- **preview mode-chrome** — the read-only preview frame, deliberately distinct from the primary violet to signal "peek mode." *(MV: cyan.)*

Each role has **light and dark variants** (`AdaptiveColor`). A direction instantiates the roles; the roles are the stable interface the rest of the spec refers to.

### 2.2 State is never carried by hue alone
Every state is conveyed by **glyph + colour**, never colour alone: `●` attached, `▌` selector bar, `✕` removable/destructive, spaced uppercase headers, `⚠` warning. Colour, where available, only *reinforces* the glyph. This makes the monochrome / `NO_COLOR` path work for free and protects colour-blind users. (Consistent with the prior `preview-visual-distinction` rule: don't rely on colour alone.)

### 2.3 Contrast floor (hard gate)
Every direction must clear a contrast gate **before taste is judged**. Functional foreground — session names, paths, footer, status text — must meet **WCAG AA**:
- **4.5:1** for normal text,
- **3:1** for large/bold text and UI accents (cursor, border, selection highlight),

measured against **both** a canonical light background (≈ white) and a canonical dark background (≈ black). With `AdaptiveColor` this means the **light variant tested on white** AND the **dark variant tested on black**, each ≥ its ratio. Purely decorative glyphs (the wordmark) are exempt from the text ratio but must stay visible. Arbitrary mid-tone custom backgrounds are out of scope for the hard gate — we target the standard light/dark cases `AdaptiveColor` flips between. A direction that can't hit the floor on both extremes is **disqualified** before looks are judged.

### 2.4 Colour-capability ladder (truecolor / 256 / 16)
Portal **imposes its own exact hues via truecolor `AdaptiveColor`** — it does **not** inherit the terminal's 16 ANSI colours. Rationale: a recognisable identity needs consistent hues across machines; inheriting the user's palette means no identity and possible clashes. Respecting the *background* (§1) plus honouring `NO_COLOR` (§2.5) already covers "don't fight the user"; imposing *hues* does not conflict with that distinction. Lipgloss/termenv **auto-downsamples** to 256/16 on weaker terminals — accepted as graceful degradation (a hue may approximate, but the contrast floor still governs legibility). Matches existing repo practice (`previewBorderColor`).

### 2.5 NO_COLOR / monochrome
Portal **honours `NO_COLOR`** and monochrome terminals: it renders colourless, leaning on the glyph-backed state (§2.2) plus bold/dim attributes. Because state is never colour-only, the UI stays fully usable without colour.

### 2.6 AdaptiveColor binary classification — best-effort + override
`AdaptiveColor` makes a **binary** light/dark choice from terminal-background detection; the real world is continuous. Two accepted-best-effort risks:
- **Mid-tone backgrounds** (e.g. Solarized base03, Gruvbox soft-dark — mainstream, not exotic) get classified to an extreme they aren't on, so a variant tuned for near-white/near-black may dip below the floor on their actual bg.
- **Detection failure** (no OSC response over SSH / tmux passthrough; `COLORFGBG` unset) → termenv defaults (often dark), so a *light*-terminal user can be served the *dark* variant — a cross-pairing the floor never tests. Acute because Portal runs **inside tmux**, where bg-detection passthrough is unreliable.

**Decision:** choose token variants that also hold up on mid-tone backgrounds **where possible**; the eventual manual theme / light-dark override (the deferred user-theme initiative, §16) is the ultimate escape hatch; exotic backgrounds and detection-miss cross-pairing are **accepted as best-effort**, mitigated by the override.

### 2.7 Narrow / short terminal — degrade, never break
Define a **minimum supported terminal size**; below it the UI **degrades** rather than breaks: drop the wordmark → compact wordmark, drop the right-side header hint, truncate names with `…`, and let height drive pagination. It must **never overflow** — the one-row-per-delegate pagination invariant always holds (every list row, header or session, is exactly one delegate line). Exact thresholds are pinned as an implementation detail.

### 2.8 Theming — tokenise now, user-override deferred
- **In scope:** structure the role-token colour layer as a single named built-in theme ("Modern Vivid"). Every renderer references **tokens**, not scattered hex. This locks layout and delegates colour, making the app theme-*ready* at near-zero extra cost.
- **Deferred to its own initiative:** a **user-overridable theme system** (external theme file e.g. `~/.config/portal/theme.{json,toml}`, merge-over-default, validation when a user picks unreadable colours — contrast floor becomes advisory + warn/clamp — multiple built-in themes, a `theme` setting, docs). Bigger surface; ships independently after the redesign. (See §16.)

### 2.9 MV token table (closed vocabulary — pinned values)

Modern Vivid is a **closed set of ~20 named tokens** (Tokyo Night family). Every renderer references a token — **no literal hex at call sites** (this is what makes §2.8 theming work). **Dark variants are pinned exactly** (extracted from the canonical Paper frames, then reconciled to clear the contrast floor). **Light variants** are derived from the Tokyo Night Day siblings, contrast-verified against white, and **locked at the in-terminal validation gate (§15)**.

**Greys / text ramp**

| Token | Role | Dark (on black) | Light (on white) | Floor |
|---|---|---|---|---|
| `text.primary` | names, wordmark, active labels, modal titles, chip text | `#C0CAF5` · 13.0 | `#2E3C64` · 10.8 | 4.5 |
| `text.strong` | selected-row meta, help actions, banner/signpost | `#A9B1D6` · 9.9 | `#3F4760` · 9.2 | 4.5 |
| `text.muted-bright` | done-tick labels, selected-row path | `#828BB8` · 6.3 | `#515A80` · 6.7 | 4.5 |
| `text.detail` | paths, counts, footer labels, subtitles, group headings | `#737AA2` · 5.0 | `#5A6296` · 5.8 | 4.5 |
| `text.dim` | group `··· N` counts, pending loading steps | `#535C86` · 3.2 | `#7C84AA` · 3.7 | 3.0¹ |
| `text.faint` | decorative only — inactive dots, `+ add`, mode indicator, hints | `#3B4261` | `#AEB2C6` | exempt² |
| `text.on-selection` | name on the selected row | `#FFFFFF` | `#1A1B2E` | 4.5 |

¹ Held to the 3:1 large/UI floor — deliberately de-emphasised but legible. ² Decorative-only; **must never carry functional text** (2.1:1, exempt from the floor).

**Accents**

| Token | Role | Dark | Light | Floor |
|---|---|---|---|---|
| `accent.violet` | cursor, selector `▌`, active dot, `?` key, focused field label, EDIT MODE, mode bar, loading bar | `#BB9AF7` · 9.1 | `#8A3FD1` · 5.7 | 3.0 |
| `accent.blue` | footer / modal **key-hint glyphs** | `#7AA2F7` · 8.3 | `#2E5FD0` · 5.7 | 4.5 |
| `accent.cyan` | Sessions header, Preview chrome, active tick `◐` | `#7DCFFF` · 12.2 | `#0E7490` · 5.4 | 4.5 |
| `state.green` | `● attached`, `✓` done, success flash | `#9ECE6A` · 11.5 | `#4C7A1F` · 5.1 | 4.5 |
| `state.red` | kill/delete emphasis, `▲` | `#F7768E` · 7.9 | `#C32647` · 5.7 | 4.5 |
| `accent.orange` | filter query / `/` / `type`, warning flash `⚠` | `#FF9E64` · 10.3 | `#9A5200` · 5.9 | 4.5 |

**Surfaces (tints / borders — light values finalised at validation)**

| Token | Role | Dark | Light |
|---|---|---|---|
| `canvas` | terminal background (never painted) | terminal bg | terminal bg |
| `bg.selection` | selected-row tint | `#1A1726` | light violet |
| `bg.warning` | warning-flash band | `#241B10` | light amber |
| `bg.track` | loading-bar empty track | `#26283A` | light grey |
| `border.separator` | title rule (2px) | `#292E42` | light blue-grey |
| `border.footer` | footer rule (1px) | `#20232E` | light blue-grey |
| `text.on-warning` | warning-flash message | `#E8C9A0` · 13.3 | `#7A4B12` · 7.4 |

**Rules**
- **Closed vocabulary** — every rendered colour is one of these tokens; no literal hex outside the token layer (enforces §2.8 theme-readiness).
- `state.green` is **attached-only** (+ success flash); `state.red` is **destructive-only**; chips are `text.primary` on a tint, never green.
- **One documented exception:** the **Preview scrollback capture** renders the pane's **real ANSI output**, not theme tokens — intentionally outside the palette. Only its *chrome* (frame, top bar) is themed (`accent.cyan` + `text.detail`).
- All values are a **hypothesis until prototyped in a real terminal (§15)**; the table is the build target, validation is the lock.

---

## 3. Visual Identity (shared chrome)

These elements form the shared frame around every page (Sessions, Projects, Preview, Loading); per-page specifics are in §4–§10. Measurements are the Paper-frame reference values — exact cell mapping is finalised at implementation (terminal cells, not web px).

> **Reference (Paper):** the shared chrome is exemplified by `Sessions — Modern Vivid v2` (dark) and `Sessions — Modern Vivid (Light)`.

### 3.1 Header — wordmark + caret + subtitle + rule
- **Wordmark:** `PORTAL` in **uppercase, letter-spaced** (≈0.26em), heavy weight, `text.primary`. Decorative — exempt from the text-contrast ratio but must stay visible.
- **Caret:** a solid block `▌` in `accent.violet`, immediately right of the wordmark — the one retained retro flourish.
- **Subtitle:** right-aligned `session manager` in `text.detail`, small + letter-spaced.
- **Separator rule:** a full-width **2px** rule (`border.separator`) under the header, dividing it from the body.
- **Narrow degrade:** below the minimum width the wordmark collapses to a compact form and the subtitle drops (per §2.7).

### 3.2 Section header
Directly under the rule: a **page/mode label** + **count** on the left, an optional hint on the right.
- Label in `accent.cyan` (Sessions) or `state.green` (Projects); a mode suffix (`— by project` / `— by tag`) in `text.detail`.
- The count renders at the **same font size** as the label, distinguished by **dim colour** not by being smaller (shares the baseline/cap-height): `state.green` for the Sessions count, `text.detail` for the Projects count.
- Right side carries the persistent `/ to filter` hint (`text.detail`) on every filterable view; `s switch view` lives in the footer only (never duplicated here).

### 3.3 Selection — thick violet left-bar
The selected row is marked by a **thick block `▌` in `accent.violet`** pinned at the far-left (a full 2-cell column), over a subtle **`bg.selection`** row tint; the selected name renders in `text.on-selection`. Unselected rows have no bar and no tint. This is the single, consistent selection signal across Sessions, grouped views, and Projects (Projects uses a full-height bar spanning its two-line row — §6).

### 3.4 Footer — condensed keymap + `?` help
A single bottom row above a **1px** top rule (`border.footer`):
- Shows only the **core** keys for the page (e.g. Sessions: `↑↓ navigate · ⏎ attach · / filter · ␣ preview`) plus a right-aligned `? help`.
- **Key glyphs** render in `accent.blue`, their **labels** in `text.detail`, the `?` glyph in `accent.violet`.
- The **full** keymap lives in the `?` help modal (§8), per page. This solves the footer-space problem (the old three-column footer couldn't fit every bind).

### 3.5 Pagination
`bubbles/list`'s built-in height-driven paginator renders as **centred dots** above the footer: the active page dot in `accent.violet`, inactive dots in `text.faint`.

### 3.6 Borders & framing
**No full-screen frame.** Structure is carried by the two horizontal rules (header separator, footer rule) plus per-element treatments (selection tint, modal panels, preview chrome) — never a box around the whole UI. This keeps the foreground-only canvas honest (§1) and avoids full-bleed background fills.

---

## 4. Sessions — Flat list

The default Sessions view (mode **Flat**) and the baseline every other view derives from. `s` cycles Flat → by Project → by Tag (§5); the active mode shows in the section header.

> **Reference (Paper):** `Sessions — Modern Vivid v2`.

### 4.1 Row anatomy
Each session is **one delegate line** — the load-bearing pagination invariant (every list row is exactly one line). Layout:
- **Name** — full-width **left column (flex)**, `text.primary` (selected: `text.on-selection` over the `bg.selection` tint + violet bar). Names are `{project}-{nanoid}` or arbitrary renamed strings (variable length, may contain spaces); over-long names **truncate with `…`** (§2.7). The flat row shows **name only** — no project/path column (that dimension is served by the grouping modes, §5).
- **Window count** — a **fixed-width trailing slot**, left-aligned, `text.detail` (selected row: `text.strong`). Reads `N window` / `N windows`.
- **Attached marker** — a **fixed-width trailing slot** right of the count: `● attached` in `state.green` when attached; an **empty slot of the same width** when not — so the bullets line up vertically down the list and the counts stay column-aligned.

Trailing slots are fixed-width and right-pinned; the name flexes to fill the remainder. This keeps the `● attached` bullets and the window counts each vertically aligned regardless of name length.

### 4.2 Section header & count
`Sessions` (`accent.cyan`) + count (`state.green`) on the left; the `/ to filter` hint on the right (§3.2). An empty list shows the empty state (§11.1).

### 4.3 Selection & navigation
A single violet left-bar + tint marks the cursor row (§3.3). The cursor never lands on a header/non-row; navigation is arrows + `Ctrl+↑/↓` page (§12). Selection feeds `⏎ attach` and `␣ preview`.

---

## 5. Sessions — Grouped views (by Project / by Tag)

> **Existing behaviour — preserved (reskin).** The grouping *machinery* — the `s` cycle, the `HeaderItem` model, Pattern A/B, the catch-alls, cursor-skip, directory resolution, tag anchoring, and mode persistence — is **already implemented**. The only change here is the **MV visual treatment** of headings and rows (§5.1); §5.2–§5.5 record the existing logic the styling consumes, as constraints to preserve — not to rebuild.
>
> **Reference (Paper):** `Sessions — by Project (MV)` · `Sessions — by Tag (MV)`.

`s` cycles the Sessions page through three **views of the same data**: Flat → **by Project** → **by Tag** → Flat. The cycle is **unconditional** (always includes by Tag, even with zero tags or zero sessions). The active view shows in the section header (`Sessions — by project` / `— by tag`); the footer adds an `s switch view` hint. Pressing `s` resets the paginator to page 1. The last-used view **persists** in `prefs.json` (best-effort; failure non-fatal). While the `/` filter is focused, `s` is a literal filter character.

### 5.1 Render-layer grouping (the key invariant)
Group headings are **real, non-selectable rows** (`HeaderItem`) interleaved before each group's session rows — **every list row, header or session, is exactly one delegate line**, so `bubbles/list` pagination stays exact (a page can never draw more lines than the viewport). Grouping is pure Lipgloss styling in the existing delegate — **not** routed through `lipgloss/tree`.
- **Heading row:** `heading ··· N` — the heading in `text.detail`, the `··· N` count in `text.dim` (dimmer). Non-selectable: its filter value is empty, so headings **vanish the instant a filter query is typed** (flatten-on-filter for free).
- **Session rows** nest **one indent level further** than flat (cursor at col 2, name at col 4); flat rows sit flush at col 2.
- The **cursor skips header rows** on initial selection and on every navigation (arrows, paging, and crossing a group boundary) — it only ever lands on session rows.

### 5.2 By Project (Pattern A)
**One row per session**, grouped under its **project**. Key = the session's directory reduced to a canonical path. Sessions whose directory resolves to no known project collect under a pinned **Unknown** catch-all (counted, empty-suppressed, with its own heading).

### 5.3 By Tag (Pattern B)
**One row per `(session, tag)` pair** — a session with multiple tags **repeats** under each tag's heading. Untagged sessions collect under a pinned **Untagged** catch-all. If **no project anywhere has tags**, the view shows the **"No tags yet" signpost** over the flat list instead (degrade-with-message, not silent flatten — §11.3).

### 5.4 Session → directory resolution
Each live session maps to a directory via the **`@portal-dir`** session user-option, stamped at creation from the git-root (by both normal creation and QuickStart), so a session stays anchored to its origin project even after its panes `cd` away.
- **Legacy fallback:** for sessions created before the stamp shipped (no `@portal-dir`), the grouped render derives the directory from the **active pane's `current_path` → git-root**, uses it for this render, and **caches the guess in-memory only** (later rebuilds in the same picker skip the pane read). It is **never stamped back to tmux** (a pane's cwd can drift; freezing it would mis-group the session permanently) — it re-derives next launch, so a session that has `cd`'d back self-corrects.
- The lookup key and the stored project path are both reduced via the same canonical-path function.
- **Pane reads are gated to grouped modes only** — Flat and the zero-tags signpost perform **zero** pane reads.

### 5.5 Tags are directory-anchored (v1)
Tags live on the **project record** (not per session). A session's effective tags = its directory's tags, looked up **live at grouped-render time**. Tags are managed only in the projects edit modal (§8) — never per session, no CLI. (Deferred: per-session tags, live-grouped filtering, tag exclusion — §16.)

---

## 6. Projects page

> **Existing behaviour — preserved (reskin).** The Projects page, its keymap, and project CRUD already exist; this section restyles them in MV. Behaviour stays identical.
>
> **Reference (Paper):** `Projects (MV)`.

A **separate page** (different data + keymap), reached by `p`/`x` from Sessions; `s`/`x` returns. Same shared chrome (§3): PORTAL header + separator, pagination, condensed footer.

### 6.1 Section header
`Projects` (`state.green`) + count (`text.detail`) on the left; the `/ to filter` hint on the right.

### 6.2 Two-line rows
Each project is a **two-line row** (uniform height, so `bubbles/list` height-driven pagination stays exact):
- **Line 1 — name** in `text.primary` (heavy).
- **Line 2 — path** in `text.detail` (dim).
- **Selected:** a **full-height `accent.violet` left bar** (a column of coloured cells spanning **both** lines) + `bg.selection` tint; the name becomes `text.on-selection`, the path `text.muted-bright`.

An empty list shows the empty projects state (§11.1).

### 6.3 Footer (project keymap)
Condensed: `⏎ new session · s sessions · e edit · / filter · ? help`. The **full** set — `d delete`, `n new in cwd`, navigation — lives in the per-page `?` help modal (§8).

---

## Working Notes

[Optional - capture in-progress discussion if needed]
