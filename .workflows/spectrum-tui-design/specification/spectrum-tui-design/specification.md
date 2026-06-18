# Specification: Spectrum TUI Design

## Specification

> **⚠ Verification mandate — applies to every task (read before planning).**
> This is a visual reskin, so correctness is *visual*. **Every plan task MUST carry explicit visual-verification steps in its acceptance criteria** — not just "tests pass". For each task:
> 1. Add or extend the task's **`vhs` tape** to drive the TUI to the screen/state it changes and write a PNG (`vhs <tape>` — harness + setup in §15).
> 2. Compare the captured PNG against the task's **named Paper frame** (§15 frame map) for **layout, structure, and colour-role match**.
> 3. Confirm **behaviour parity** with the pre-reskin implementation (§1 "Reskin, not rebuild").
>
> A task is **not done** until its `vhs` capture is produced and checked against its frame. **Planning MUST propagate this into every task it authors.** §15 defines the `vhs` harness, its setup, and the frame map.

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

## 7. Filtering (`/`)

> **Existing behaviour — preserved (reskin).** Live filtering is `bubbles/list`'s built-in filter; this section restyles the query in MV and **pins the two-mode boundary** so the state machine is unambiguous. The styling + boundary clarity is the change — the filter engine is unchanged.
>
> **Reference (Paper):** `Filtering — input active (MV)` · `Filtering — list-active (MV)` · `Filtering — no matches (MV)`.

`/` opens an **inline filter input** in the section-header row (where the `/ to filter` hint sits). The query renders in **`accent.orange`** (with an `accent.orange` `/` prefix); the list filters **live as you type**. The `/ to filter` hint shows top-right **consistently** on every session view (Flat / by Project / by Tag) and Projects. **No match-count is shown** — the visible results suffice. Typing a query flattens grouped views (headings vanish — §5.1).

### 7.1 Two mutually-exclusive modes
Filtering is **never both at once** — there is never an active input cursor *and* a selected row simultaneously:

1. **Input-active (typing).** Keystrokes go to the query; the **cursor sits at the end of the typed text**; the list updates live; **no list row is selected**. Filter bar reads `/ <query>▌`; footer: `type to filter · ↵/↓ browse results · esc clear`.
2. **List-active (browsing results).** The input row stays visible — the **locked `accent.orange` query (no cursor)** is what signals the list is filtered; **arrows move the selection**; `↵` attaches; `Esc` clears and returns. No background tint. Footer: `↵ attach · ↑↓ navigate · esc clear filter`.

### 7.2 Boundary
- **`↵` or `↓`** commits input-active → list-active.
- **`Esc`** clears the filter from either mode (returns to the unfiltered list).

### 7.3 Over-filtered (no matches)
When the query matches nothing: a centred empty state — a dim `⌀` glyph (`text.faint`), `No sessions match "<query>"` (`text.primary`), hint `⌫ to widen the search · esc to clear the filter` (`text.detail`). Footer stays in input-active form.

---

## 8. Modals (edit · kill · rename · help)

> **Reference (Paper):** `Edit Modal — navigate (name)` · `Edit Modal — chip focused` · `Edit Modal — edit in place` · `Kill Confirm Modal (MV)` · `Rename Modal (MV)` · `Sessions — Help Modal (?)`.

### 8.1 Modal framing (shared)
- **Modals render on a blank screen (changed behaviour).** When a modal opens, the page behind is **cleared to blank** and the modal is centred on the canvas. **This changes today's behaviour** — existing modals render **as an overlay on top of the page content**. Blank-screen is therefore a **shared modal-layer change**, not a per-modal restyle.
  - **Open implementation question (feasibility-gated, §14):** whether the existing modal render path can be **adapted** (clear/replace the page, then draw the centred modal — likely small) or needs a **modal-system rework** is **not yet determined** — assess against the code at implementation. Either way, the underlying **confirm / input logic of each modal must be preserved** (parity); only the surrounding render shell changes.
  - *(Exception: the Preview screen is a full-screen overlay, not a modal — §9; a `?` help opened from Preview overlays the preview without blanking it.)*
- Centred bordered panel with a **contextual footer** reflecting the modal's current focus/mode.
- **Modals are key-exclusive while open** — an open modal consumes all key input until dismissed; underlying page binds (`s`/`p`/`n`/`e`/`d`/clear-filter/quit, etc.) do **not** fire beneath it. `Esc` resolves against the modal first.
- Reskin status: **kill**, **rename**, and **delete-project** keep their **confirm/rename logic** (parity) but adopt the new blank-screen rendering + MV restyle; the **edit modal** adopts a **new interaction model** (§8.2); the **`?` help modal** is **new** (§8.5).

### 8.2 Edit Project modal — two-mode, immediate-persist (⚠ behaviour change)
> **New behaviour (not a reskin-preserve).** This replaces the current asymmetric model (tags persist live; Name/Aliases batch) with a **uniform two-mode immediate-persist** model across all three fields. Behaviour parity does **not** apply here — it is a deliberate change; implement as specified.

A bordered panel with labelled fields **NAME / ALIASES / TAGS** and a mode indicator in the header (`Edit Project <name>` + `navigate` / `◉ EDIT MODE`). Two modes apply uniformly to all three fields:

- **Navigate mode (default).** `Tab`/`Shift+Tab` move between fields; `←/→` move across chips and the trailing `+ add` slot within a chip field. **Entering a chip field via `Tab`/`Shift+Tab` lands on the trailing `+ add` slot** (adding is the common action); `←` then reaches the existing chips. The focused element shows a **focus highlight** (`accent.violet` outline, no fill). `x` **deletes** a focused chip immediately. `Esc` **closes the modal**.
- **Edit mode (one element live).** Entered by `Enter`/`e` on a chip, `Enter` on Name, or `Enter`/`+` on a focused `+ add` slot — which **spawns a new empty chip already in edit mode** (edit highlight + live cursor). Landing on `+ add` (via `Tab` or `←/→`) is navigate-mode focus only; it never auto-enters edit mode. Type to edit; `←/→` move the **text cursor within the value**. `Enter` **commits & persists** → back to navigate; `Esc` **discards that element's edit** (a brand-new empty chip vanishes) → back to navigate.

**Persistence is immediate, per item** — each element persists on exit-edit (`Enter`). There is **no dirty state, no save key, no batch**; `Esc` never discards saved work (it only backs out the current edit, or closes the already-saved modal). This extends the codebase's existing tags-persist-live behaviour to Name + Aliases (consistent, not a reversal).

**Falling-out rules:**
- **Empty on commit = delete** (new or existing chip); deleting a focused chip is immediate.
- **Empty Name can't persist → reverts** to the prior value.
- **Duplicate on commit = no-op.** Committing a chip whose value already exists in the same field silently dedupes (the existing chip remains; no duplicate is added, no error shown) — consistent with the project store's existing per-field dedupe (tags are case-sensitive).
- **`Esc` backs out one level:** edit mode → discard the element's edit; navigate mode → close (all already saved).

**Visual states (the focus-vs-edit grammar, §13):**
- **Chips** (aliases AND tags) are **one neutral style** — `text.primary` on a subtle tint; **never green** (green is attached-only). Three states: **normal** (subtle, no `✕`) · **focused** (`accent.violet` outline + a violet `✕` showing it's actionable — `x` removes it) · **editing** (`accent.violet` fill + cursor, no `✕`).
- **Field labels:** the **focused field's** label is `accent.violet`; the others are `text.detail`.
- **`+ add`** is an inline input slot (not a button/popup) in `text.faint`; the **mode indicator** reads `◉ EDIT MODE` (`accent.violet`) in edit mode, dim in navigate.

**Contextual footer** (matches focus/mode):
- Name focused (navigate): `↵ edit · ⇥ next field · esc close`.
- Chip focused (navigate): `↵/e edit · x remove · ←→ move · ⇥ next field · esc close`.
- Editing in place: `↵ save · esc discard · ←→ cursor · empty on save = delete`.

The modal stays a **single bundle** for Name + Aliases + Tags (not split).

### 8.3 Kill confirm modal
> **Logic preserved; rendering changed.** The confirm flow (`y`/`n`/`Esc`) is unchanged (parity); it inherits the new blank-screen rendering (§8.1) and the MV restyle.

A centred panel: `▲ Kill session?` (`state.red` triangle), the **session name in `state.red`**, `· N window(s)` (`text.detail`), a consequence line "Ends the tmux session and all its panes. Can't be undone." (`text.detail`), footer `y kill · n cancel · esc to cancel`. **`state.red` is reserved for destructive actions.** Keys: `y`/`n`/`Esc`.

### 8.4 Rename modal
> **Logic preserved; rendering changed.** The rename flow is unchanged (parity); it inherits the new blank-screen rendering (§8.1) and the MV restyle.

A labelled `NEW NAME` input (focused label `accent.violet`, value `text.primary` + violet `▌` cursor), a `was: <old name>` context line (`text.detail`), footer `↵ rename · esc cancel`. Keys: `Enter`/`Esc`.

### 8.5 `?` help modal (new) — per-page
> **New behaviour.** There is **no `?` binding today** (`?` is actively swallowed so `bubbles/list` doesn't toggle its own help). This adds: **bind `?`** on every page + a help-modal type + **per-page content**.

A centred panel listing **the current page's** keymap (two columns: key-hint glyph in `accent.blue` / action label in `text.strong`), header `? Keybindings` (`text.primary`), right-aligned `esc to close` (`text.detail`). Content differs per page (Sessions / Projects / Preview keymaps — §12); only Sessions help is mocked, the others follow their audited keymaps at implementation. Opened from Preview, it **overlays** the preview (doesn't blank it — §9). The help modal closes on `?` (toggle) or `Esc`; while open it is key-exclusive (§8.1), so `Esc` dismisses it and does **not** fall through to the page's clear-filter / quit.

### 8.6 Delete project confirm modal
> **Logic preserved; rendering changed.** The confirm flow (`y`/`n`/`Esc`) is unchanged (parity); it inherits the blank-screen rendering (§8.1) + MV restyle. *(Mocked at implementation, mirroring `Kill Confirm Modal (MV)`.)*

A centred panel mirroring the kill modal's destructive treatment: `▲ Delete project?` (`state.red` triangle), the **project name in `state.red`**, its path (`text.detail`), and a consequence line that disambiguates it from killing a session — it removes only the **project record**: "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched." (`text.detail`). Footer `y delete · n cancel · esc to cancel`. Keys: `y`/`n`/`Esc`.

---

## 9. Preview screen

> **Existing behaviour — preserved (reskin).** The read-only scrollback preview already exists (`pagepreview.go`, hand-composed chrome); this restyles its chrome to the MV cyan "peek mode". The captured content and scroll/nav behaviour are unchanged.
>
> **Reference (Paper):** `Preview Screen (MV)`.

A **full-screen overlay** (not a modal — the blank-screen rule of §8.1 does not apply), reached by `Space` on a session. Its chrome is **`accent.cyan`-framed** to signal **"peek mode"** — deliberately distinct from the violet main UI, preserving the `preview-visual-distinction` mode-signal in the MV palette.

### 9.1 Chrome
- **Top bar:** `⊙ preview` (`accent.cyan`) + `<session>` (`text.primary`) + `Window x/y · Pane x/y` (`text.detail`), with right-aligned nav hints `[ ] window · ↹ pane · ⏎ attach · ␣ back` (`text.detail`).
- A **cyan border** (`accent.cyan`) frames the read-only content area.

### 9.2 Captured content (out-of-theme)
The pane content is the **real captured ANSI output**, rendered read-only — **not** theme tokens (the documented palette exception, §2.9/§15.1). Only the chrome is themed; the content is whatever the pane actually printed.

### 9.3 Keys & overlays
Scroll `↑↓` + `Ctrl+↑/↓`; `Tab` next pane; `]`/`[` window; `⏎` attach (this pane); `Space`/`Esc` back (§12). A `?` help opened here **overlays** the preview (doesn't blank it — §8.1).

---

## 10. Loading interstitial & cold-path startup flip

> **New engineering — the single biggest item in the redesign (its own phase/PR).** Making the loading screen honest/determinate requires restructuring cold-boot bootstrap to run concurrently with the TUI. Gated behind in-terminal validation of the visual direction (§15). Estimated **~1–1.5 days** incl. tests + race review — treat the estimate as having genuine variance given the load-bearing startup path and its prior-incident history (the slow-open / zombie-session episode).
>
> **Reference (Paper):** `Loading 6 — Combined (thick bar)`. *(The loading-page error frame is mocked at implementation — §10.5.)*

### 10.1 Cold vs warm — when the loading screen shows
The loading page is gated on **`serverStarted`** (set only when `EnsureServer` actually had to start the tmux server):
- **Cold boot** (no tmux server): server started → full bootstrap → **loading page shown**.
- **Warm** (server already up, just opening another picker): `serverStarted=false` → bootstrap steps no-op → **straight to the picker, no loading page**. The common case — instant and **untouched**.

**The flip is scoped to the COLD path only.** A cheap `tmux has-server` check decides; warm keeps today's fast synchronous path, carrying **zero new risk**.

### 10.2 The startup flip (concurrent cold-boot bootstrap)
**Today:** the full 11-step bootstrap runs **synchronously in `PersistentPreRunE` before the TUI launches** — by the time the loading page renders, restore is already 100 % done, so the page is a cosmetic 1.2 s pad. A slow restore happens *before* the page appears (frozen terminal).

**Flip:** for the **cold + TUI path only** (scoped via the existing `isTUIPath`; CLI/direct-path keeps the synchronous bootstrap), launch Bubble Tea **immediately** on the loading page, run the orchestrator in a **goroutine**, stream a `tea.Msg` per real step (and per restored session), transition to Sessions on complete, **quit-with-error** on the one fatal step. A progress callback is injected at the restore per-session loop.
- The loading page already gates Sessions enumeration on `BootstrapCompleteMsg`, and the TUI is **inert during loading** (animation only) — this **contains the race surface**.
- **A progress channel carries `serverStarted` + per-step progress to the TUI** on the cold/TUI path, replacing today's `context` + package-memo delivery.

**Real costs / risks (not zero):** reworking `serverStarted`/warnings delivery; fatal-error-as-`tea.Quit` (today a `PersistentPreRunE` error return); careful restore/daemon race review against the live event loop (prior-incident history); integration-test updates around startup ordering.

**Payoff:** an *honest* determinate loading screen **and** elimination of "frozen terminal on a slow boot" (instant "Portal is starting" feedback).

### 10.3 Loading screen design (combined, honest)
Centred **`PORTAL ▌`** (wordmark `text.primary` + caret `accent.violet`) over a **thick block progress bar** (filled `accent.violet`, track `bg.track`) and a **tick-list that ticks off** as each boot step completes — a **real list**, not an in-place text swap:
- `✓` done — glyph `state.green`, label `text.muted-bright`
- `◐` active — glyph `accent.cyan`, label `text.primary`
- `·` pending — glyph `text.faint`, label `text.dim`

Bar weight is **thick** (decided). Warm path shows no loading screen.

### 10.4 Step mapping (11 real steps → 5 friendly labels)
The bar advances on **every real bootstrap step**; the **active label** is the friendly group the current step falls in (each label spans ≥1 real step). Proposed grouping (cleanup steps 8–11 are near-instant and fold under the final label; implementation may adjust which fast step sits under which label):

| Friendly label | Real bootstrap step(s) |
|---|---|
| `Started tmux server` | 1 EnsureServer |
| `Registered hooks` | 2 RegisterPortalHooks · 3 set `@portal-restoring` · 4 SweepOrphanDaemons · 5 EnsureSaver |
| `Restoring sessions (N/M)` | 6 Restore — skeleton phase (the per-session loop; `N/M` is its real counter) |
| `Replaying scrollback` | 6 Restore — geometry + scrollback replay · 7 EagerSignalHydrate |
| `Resuming Claude sessions` | hydrate helpers firing on-resume hooks · 8 clear `@portal-restoring` · 9–11 marker/FIFO/stale cleanup |

Only `Restoring sessions` carries an `N/M` counter (the restore loop is the one real per-item progress source); other labels tick once.

### 10.5 Error & warning contract (cold-path)
- **Fatal cold-boot step failure** → an **in-TUI error state on the loading page**: the failed step gets a **`state.red` marker + a one-line message**; `q`/`Esc` quits with a **non-zero exit** — rather than dropping into a half-restored picker. The loading-page **error frame** is mocked at implementation.
- **Soft warnings** ride the **progress channel** and surface as a **post-load notice** (after the picker appears).

---

## 11. Edge / UX states

> **Reference (Paper):** `Sessions — empty (MV)` · `Sessions — inline flash (MV)` · `Sessions — no tags signpost (MV)` · `Projects — command pending (MV)`.

**Shared convention — left-bar accent notices.** Inline notices use a **left-bar accent line**: **`accent.orange`** = transient / warning, **`accent.violet`** = mode / info. **Placement:** the band sits **directly under the title separator, above the section header** (full-width); the section header + list **shift down**.

**Single-slot rule.** The notice slot holds **at most one band**. Persistent mode notices (no-tags signpost §11.3, command-pending banner §11.4) own the slot while their mode is active; a transient flash (§11.2) **takes the slot temporarily**, replacing any persistent band for its duration, then the persistent band returns. The flash **auto-clears on the next keypress or after a short timeout**. Orange (warning) and violet (info) never display at once — the transient flash wins while shown.

### 11.1 Empty states (reskin)
- **Empty sessions** — centred: a dim block glyph `▌ ▌ ▌` (`text.faint`), `No sessions yet` (`text.primary`), hint `Press n to start one in the current directory · p for projects` (`text.detail`); the footer reduces to the still-relevant keys (`n` / `p` / `/` / `?`).
- **Empty projects** mirrors it — `No projects yet` + an open-a-directory hint (same pattern; not separately mocked).

### 11.2 Inline flash (chrome band)
A **transient band** under the title separator: an **`accent.orange` left-bar** + `⚠` + message (e.g. `folio-Jiz4el closed externally — list updated`), on a `bg.warning` tint with `text.on-warning` message text; **auto-clears**. The **success variant** uses `state.green`.
- **F10 — flash vs pagination:** the flash band is **chrome** — when it appears/clears, the list **viewport height is recomputed** (the same recompute the one-row-per-delegate invariant already mandates), so the list never overflows or miscounts rows.

### 11.3 "No tags yet" signpost (reskin)
By-Tag with **zero tags anywhere**: an **`accent.violet` left-bar** signpost (`No tags yet — add tags in the project editor (e) …`, `text.strong`) over the **flat list** — degrade-with-message, not a silent flatten (§5.3).

### 11.4 Command-pending banner (reskin)
When Projects is invoked to **run a command**: an **`accent.violet` left-bar** banner (`Pick a project to run`) with the command in an **`accent.orange` chip**; the footer becomes `⏎ run here · n run in cwd · esc cancel`. The screen keeps the **full Projects chrome** (green `Projects` header + `/ to filter`) — not a stripped page; the banner sits on top.

---

## 12. Keybindings (audited against code)

> **Mixed: mostly existing bindings, with a deliberate keymap revision.** The per-screen keymaps below are audited against the current code. The **changes** are: drop all vim/extra nav aliases, repurpose `k`, and add the `?` binding (§12.2). Unchanged bindings are preserved (parity).

### 12.1 Per-screen keymaps
- **Sessions (flat & grouped):** `↑`/`↓` move · `Ctrl+↑`/`Ctrl+↓` page · `/` filter · `Enter` attach · `Space` preview · `s` cycle grouping (flat→project→tag) · `r` rename · `k` kill · `n` new-in-cwd · `p`/`x` → Projects · `q` quit · `Esc` clear-filter / quit. Grouping adds no keys.
- **Projects:** `↑`/`↓` move · `Ctrl+↑`/`Ctrl+↓` page · `/` filter · `Enter` new-session-from-project · `s`/`x` → Sessions · `e` edit · `d` delete · `n` new-in-cwd · `q` quit · `Esc`.
- **Preview:** `↑`/`↓` + `Ctrl+↑`/`Ctrl+↓` scroll · `Tab` next pane · `]`/`[` window · `Enter` attach (this pane) · `Space`/`Esc` back.
- **Modals:** kill `y`/`n`/`Esc` · delete-project `y`/`n`/`Esc` · rename `Enter`/`Esc` · edit — two-mode (§8.2).

### 12.2 Keymap revision (the changes)
- **Navigation is arrows only.** **Drop all vim aliases (`h`/`j`/`k`/`l`, `g`/`G`) and `PgUp`/`PgDn`/`Home`/`End`** — move is `↑`/`↓`, page is `Ctrl+↑`/`Ctrl+↓`. `/` filter is the fast-find (filtering, not jump-to-extremes, is how you find a session).
- **`k` = kill** — freed by dropping vim-up; the tmux-accurate verb, kept distinct from Projects' `d` = delete (removing a project *record* is a different operation).
- **No uppercase bindings anywhere.**
- **`?` is newly bound** on every page → opens the per-page help modal (§8.5). **Today `?` is actively swallowed** (so `bubbles/list` doesn't toggle its own help); the redesign binds it.

### 12.3 Validation caveat
Confirm `Ctrl+↑`/`Ctrl+↓` isn't swallowed by the terminal/tmux during in-terminal validation (§15); **fall back to another page key if so.**

---

## 13. Interaction conventions (cross-cutting)

These conventions apply across surfaces; per-surface detail lives in the referenced sections.

### 13.1 Focus vs edit — unified visual grammar
Two states, identical grammar everywhere (the Name field, chips, any editable element):
- **Focused** (navigate): **outline only** — an `accent.violet` ring, no fill change.
- **Editing** (cursor live): **`accent.violet` fill + cursor**, plus a `◉ EDIT MODE` indicator in the modal header (the Name field in edit mode also turns violet-filled — same treatment as chips).
- **So: outline = focused, fill = editing** — unambiguous everywhere.
- **Chips** (aliases AND tags) are **one neutral style**; **green is reserved for `attached` only, never chips** (detail in §8.2).

### 13.2 Page model — views vs pages
- **Sessions is ONE page with three grouping *views*** (Flat / by Project / by Tag), cycled by `s` — the same data pivoted, not separate pages (§4–§5).
- **Projects is a separate *page*** (different data + keymap), reached by `p`/`x` (§6).
- **Preview** is an overlay screen (`Space`, §9); **Loading** is the startup screen (§10).

### 13.3 `?` help is per-page contextual
`?` is bound on every page (not modals) and opens a help modal listing **that page's** keymap — one overlay pattern, page-specific content (§8.5, §12).

### 13.4 Filtering & the `/ to filter` hint
The `/ to filter` hint shows top-right on every session view and Projects; `s switch view` lives in the **footer**. Two-mode filtering detail in §7.

### 13.5 Modals on a blank screen
When a (centred) modal opens, the page behind is cleared to blank — not a dimmed overlay (a change from today; §8.1). The Preview overlay is the exception (§9).

### 13.6 Typography — counts beside labels
A count next to a label (`Sessions N`, `Projects N`, group `heading ··· N`) renders at the **same font size** as the label, distinguished by its **dim colour**, not by being smaller — so it shares the baseline and cap-height.

---

## 14. Implementation architecture (feasibility)

> **Context for planning — sizing, not task breakdown.** ~80 % of this reskin is restyling already-custom Lipgloss render code (today's TUI is hand-rendered on top of `bubbles/list`, not an off-the-shelf widget kit). No widget framework is needed.

### 14.1 Kept as-is (the engine)
`bubbles/list` provides the list model, pagination (the dots), filtering, cursor/selection, and nav for Sessions & Projects. The **build constraint holds**: grouping stays pure Lipgloss in the delegate — **no `lipgloss/tree`**.

### 14.2 Restyle existing render code (the bulk)
Edit current custom code and point it at palette tokens: the row delegates (`SessionDelegate` / `ProjectDelegate`), the manual three-column footer (→ condensed), the group `HeaderItem`, the kill / rename modals, the preview chrome (`pagepreview.go`), and the loading `viewLoading`.

### 14.3 New-but-small
- The **header / wordmark + separator block** above the list (≈ Lipgloss `JoinVertical`).
- **Edit-modal chips** (restyle the alias/tag field render into chip elements).

### 14.4 New-substantial (one)
The **`?` help modal** — a new modal type + binding `?` (currently swallowed) + per-page help content (~60–80 lines). Extends the existing rounded-border modal overlay primitive.

### 14.5 Cross-cutting foundation
An **`AdaptiveColor` palette / role-token layer** (the §2.9 tokens, each with light + dark variants), contrast-floor adherence, and `NO_COLOR` handling. Moderate, touches every style — but it is **centralising colour, not adding widgets**.

### 14.6 Open question — modal rendering path
Whether the existing modal render path can be **adapted** for the blank-screen treatment (§8.1) or needs a **modal-system rework** is **not yet determined** — assess against the code at implementation. The underlying confirm/input logic of each modal is preserved either way.

### 14.7 Separate engineering item
The **cold-path startup flip** (§10) — concurrent bootstrap + live progress — is plumbing, not a widget, and is its **own phase** (~1–1.5 days).

---

## 15. Design reference & visual verification

### 15.1 Paper design reference (the frame map)
All visual decisions are mocked in the Paper file **"Portal"** (`https://app.paper.design/file/01KVAT8NFHMBDTM4YY6V93R53S`, page "Page 1"), via the `paper` MCP. The **canonical frames** (build targets, uniform 860×680):

| Surface | Frame(s) | Spec |
|---|---|---|
| Sessions — flat | `Sessions — Modern Vivid v2` · `Sessions — Modern Vivid (Light)` | §4 |
| Sessions — grouped | `Sessions — by Project (MV)` · `Sessions — by Tag (MV)` | §5 |
| Filtering | `Filtering — input active (MV)` · `Filtering — list-active (MV)` · `Filtering — no matches (MV)` | §7 |
| Projects | `Projects (MV)` | §6 |
| Loading | `Loading 6 — Combined (thick bar)` | §10 |
| Help modal | `Sessions — Help Modal (?)` | §8 |
| Edit modal | `Edit Modal — navigate (name)` · `Edit Modal — chip focused` · `Edit Modal — edit in place` | §8 |
| Kill / Rename | `Kill Confirm Modal (MV)` · `Rename Modal (MV)` | §8 |
| Preview | `Preview Screen (MV)` | §9 |
| Edge states | `Sessions — empty (MV)` · `Sessions — inline flash (MV)` · `Sessions — no tags signpost (MV)` · `Projects — command pending (MV)` | §11 |

Exploration frames (the five colour directions, loading concepts, MV v1) are reference-only — **not build targets**. Paper is an HTML approximation: authoritative for **layout, structure, and colour-role**, not pixel-exact rendering (the real terminal uses the user's font + the §2.9 token hexes).

### 15.2 `vhs` capture harness (the prescribed verification tool)
Visual verification uses **`vhs`** (charmbracelet/vhs) — a headless terminal driven by a `.tape` script that sends keys and writes a PNG. Prescribed for this feature (Portal is a Bubble Tea / charm app; `vhs` is the natural fit and runs in CI).

**Setup (one-time):**
1. `brew install vhs` — pulls its `ttyd` + `ffmpeg` dependencies. *(Non-Homebrew: `go install github.com/charmbracelet/vhs@latest`, with `ttyd` + `ffmpeg` installed separately.)*
2. Verify with `vhs --version`.

**Harness structure:**
- **One `.tape` per canonical screen**, committed under a fixed harness dir (e.g. `testdata/vhs/`).
- Each tape sets a fixed terminal size, seeds a **known fixture state** (a fixed set of sessions/projects for deterministic captures — fixture-seeding mechanics are a harness implementation detail), launches Portal, sends keys to reach the target screen, then `Screenshot <name>.png`. Example:
  ```
  Output sessions.png
  Set FontFamily "JetBrains Mono"
  Set Width 1280
  Set Height 800
  Type "portal"
  Enter
  Sleep 800ms
  Screenshot sessions.png
  ```
- Run `vhs <tape>` to produce the PNG.

**Pass criterion:** **layout / structure / colour-role match** to the named Paper frame — **agent/user-judged, NOT a pixel-diff CI gate** (Paper is an approximation; an exact diff would always fail). Tapes are committed and runnable so any reviewer can re-capture.

### 15.3 Per-task manual review
In addition to the `vhs` capture, the user inspects the rendered TUI in a real terminal at each task's end — catching font/colour realities the Paper approximation can't show.

### 15.4 Verification responsibilities in the task loop
Each implementation task runs a fixed loop with an explicit owner for the visual check at every step:

1. **Implementer (sub-agent)** — does the work **and produces the task's `vhs` capture**, comparing it to the named Paper frame to **self-verify before handing off**. The implementer owns the capture so it can check and converge its own work — without this, the implement↔review loop never terminates.
2. **Reviewer (sub-agent)** — reviews the **code** (its primary, essential job) **and** the **visual**: confirms the implementer's capture matches the frame (layout / structure / colour-role) and that **behaviour parity** holds (§1). Only when **both** pass does the task **gate for human review**.
3. **Human gate** — the human opens the task's **latest screenshot** (and inspects the live TUI) before approving.

**Screenshot storage (explicit):** each task's latest `vhs` PNG is **committed in-repo** under the harness dir, **named per frame/task** (e.g. `testdata/vhs/sessions-flat.png`), overwritten in place so "latest" is always current — giving the reviewer and the human a stable, well-labelled image to open without re-running anything.

---

## 16. Scope boundary

### 16.1 In scope (v1)
- The full **Modern Vivid reskin** across **every** surface — Sessions (flat / by-project / by-tag), Projects, Preview, Loading, all modals (edit two-mode, kill, rename, `?` help), filtering (two-mode), and every edge state (empty, inline flash, no-tags signpost, command-pending) — built **token-based** (theme-ready, §2.8).
- The **cold-path startup flip** (§10) — its own phase, gated behind in-terminal validation (§15).

### 16.2 Animation & performance
Animation is **minimal and idle-zero** — no idle CPU tick in an always-open tool. The loading screen animates only while bootstrap runs; the picker does not animate at rest.

### 16.3 Deferred (logged separately)
- **User-overridable theme system** — external theme file, merge-over-default, validation/clamp, multiple built-in themes, a `theme` setting, docs (§2.8). Ships independently after the reskin. *(Logged: `.workflows/.inbox/ideas/2026-06-17--user-overridable-theme-system.md`.)*
- **Tag features (v2):** per-session tags (`@portal-tags` + `--tag=`), live-grouped filtering, tag exclusion (§5.5).

### 16.4 Cut
- The **animated cycling-colour border** — dropped for its idle-CPU cost in an always-open tool (inconsistent with idle-zero animation).

### 16.5 Lock-in gate
The colour direction is a **hypothesis until prototyped in a real terminal** (§15) — the in-terminal validation gate is the final lock before implementation closes; bail remains a legitimate outcome if the direction doesn't clear the bar (§1).

---

## Working Notes

[Optional - capture in-progress discussion if needed]
